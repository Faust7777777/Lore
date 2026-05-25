package cli

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"strings"
	"time"

	"obsidian-harness/internal/app"
	"obsidian-harness/internal/model"
	"obsidian-harness/internal/persona"
)

func runPersonaCommand(args []string, stdout io.Writer, stderr io.Writer) int {
	if len(args) == 0 {
		fmt.Fprintln(stderr, "usage: lore persona <candidates|errors|summary> [args]")
		fmt.Fprintln(stderr, "  candidates  list/show/dismiss/draft/recover persona memory candidates")
		fmt.Fprintln(stderr, "  errors      tail the persona extraction failure log")
		fmt.Fprintln(stderr, "  summary     one-page dashboard of candidate counts and error counts")
		return 1
	}
	switch args[0] {
	case "candidates":
		// handled below
	case "errors":
		return runPersonaErrorsCommand(args[1:], stdout, stderr)
	case "summary":
		return runPersonaSummaryCommand(args[1:], stdout, stderr)
	default:
		fmt.Fprintf(stderr, "persona: unknown subcommand %q\n", args[0])
		return 1
	}
	sub := args[1:]
	if len(sub) == 0 {
		fmt.Fprintln(stderr, "usage: lore persona candidates <list|show|dismiss|draft|recover> [flags] [id]")
		return 1
	}
	switch sub[0] {
	case "list":
		workDir, stateFilter, limit, asJSON, err := parsePersonaListFlags(sub[1:], stderr)
		if err != nil {
			fmt.Fprintln(stderr, err)
			return 1
		}
		runtime, err := app.OpenRuntime(workDir)
		if err != nil {
			fmt.Fprintf(stderr, "open runtime: %v\n", err)
			return 1
		}
		defer closeRuntime(stderr, runtime, "persona candidates list")
		records, err := runtime.ListPersonaCandidates(stateFilter, limit)
		if err != nil {
			fmt.Fprintf(stderr, "persona candidates list: %v\n", err)
			return 1
		}
		if asJSON {
			if err := emitPersonaCandidateListJSON(stdout, workDir, stateFilter, limit, records); err != nil {
				fmt.Fprintf(stderr, "persona candidates list: emit json: %v\n", err)
				return 1
			}
			return 0
		}
		renderPersonaCandidateList(stdout, stateFilter, records)
		return 0
	case "show", "dismiss":
		workDir, candidateID, asJSON, err := parsePersonaShowDismissFlags("persona candidates "+sub[0], sub[1:], stderr)
		if err != nil {
			fmt.Fprintln(stderr, err)
			return 1
		}
		runtime, err := app.OpenRuntime(workDir)
		if err != nil {
			fmt.Fprintf(stderr, "open runtime: %v\n", err)
			return 1
		}
		defer closeRuntime(stderr, runtime, "persona candidates "+sub[0])

		switch sub[0] {
		case "show":
			record, err := runtime.GetPersonaCandidate(candidateID)
			if err != nil {
				fmt.Fprintf(stderr, "persona candidates show: %v\n", err)
				return 1
			}
			if asJSON {
				if err := emitPersonaCandidateDetailJSON(stdout, record); err != nil {
					fmt.Fprintf(stderr, "persona candidates show: emit json: %v\n", err)
					return 1
				}
				return 0
			}
			renderPersonaCandidateDetail(stdout, record)
		case "dismiss":
			record, err := runtime.DismissPersonaCandidate(candidateID, time.Now())
			if err != nil {
				fmt.Fprintf(stderr, "persona candidates dismiss: %v\n", err)
				if errors.Is(err, app.ErrPersonaCandidateAlreadyDrafted) {
					fmt.Fprintln(stderr, "  hint: drafted candidates are not dismissed directly.")
					fmt.Fprintf(stderr, "        if the candidate is in the partial-orphan shape (DraftID empty), abandon it via:\n")
					fmt.Fprintf(stderr, "          lore persona candidates recover --force-dismiss %s\n", candidateID)
					fmt.Fprintln(stderr, "        otherwise reject the linked draft first via `lore draft reject <draft-id>`.")
				}
				return 1
			}
			renderPersonaCandidateActionResult(stdout, "dismiss", record, "")
		}
		return 0
	case "draft":
		workDir, candidateID, retryRejected, err := parsePersonaDraftFlags(sub[1:], stderr)
		if err != nil {
			fmt.Fprintln(stderr, err)
			return 1
		}
		runtime, err := app.OpenRuntime(workDir)
		if err != nil {
			fmt.Fprintf(stderr, "open runtime: %v\n", err)
			return 1
		}
		defer closeRuntime(stderr, runtime, "persona candidates draft")
		var record persona.PersonaCandidateRecord
		var result model.PersonaUpdateProposalResult
		if retryRejected {
			record, result, err = runtime.RetryRejectedPersonaDraft(candidateID, time.Now())
		} else {
			record, result, err = runtime.CreatePersonaDraftFromCandidate(candidateID, time.Now())
		}
		if err != nil {
			fmt.Fprintf(stderr, "persona candidates draft: %v\n", err)
			renderPersonaDraftErrorHint(stderr, candidateID, retryRejected, err)
			return 1
		}
		action := "draft"
		if retryRejected {
			action = "draft (retry-rejected)"
		}
		renderPersonaCandidateActionResult(stdout, action, record, result.DraftID)
		return 0
	case "recover":
		workDir, candidateID, draftID, forceDismiss, err := parsePersonaRecoverFlags(sub[1:], stderr)
		if err != nil {
			fmt.Fprintln(stderr, err)
			return 1
		}
		runtime, err := app.OpenRuntime(workDir)
		if err != nil {
			fmt.Fprintf(stderr, "open runtime: %v\n", err)
			return 1
		}
		defer closeRuntime(stderr, runtime, "persona candidates recover")
		var record persona.PersonaCandidateRecord
		if forceDismiss {
			record, err = runtime.ForceDismissPartialPersonaCandidate(candidateID, time.Now())
			if err != nil {
				fmt.Fprintf(stderr, "persona candidates recover: %v\n", err)
				renderPersonaRecoverErrorHint(stderr, candidateID, "", true, err)
				return 1
			}
			renderPersonaCandidateActionResult(stdout, "recover --force-dismiss", record, "")
			return 0
		}
		record, err = runtime.RecoverPersonaCandidateLink(candidateID, draftID, time.Now())
		if err != nil {
			fmt.Fprintf(stderr, "persona candidates recover: %v\n", err)
			renderPersonaRecoverErrorHint(stderr, candidateID, draftID, false, err)
			return 1
		}
		renderPersonaCandidateActionResult(stdout, "recover --link", record, record.DraftID)
		return 0
	default:
		fmt.Fprintf(stderr, "persona candidates: unknown subcommand %q\n", sub[0])
		return 1
	}
}

func parsePersonaListFlags(args []string, stderr io.Writer) (string, persona.PersonaCandidateState, int, bool, error) {
	flags := flag.NewFlagSet("persona candidates list", flag.ContinueOnError)
	flags.SetOutput(stderr)
	workDir := flags.String("workdir", "", "workdir that contains vault/ and state/")
	limit := flags.Int("limit", 20, "maximum candidates to show (0 = no limit)")
	state := flags.String("state", "open", "candidate state to list: open, drafted, dismissed")
	asJSON := flags.Bool("json", false, "emit candidates as a JSON object instead of the tab-delimited table")
	args = reorderFlagsBeforePositionals(args, flags)
	if err := flags.Parse(args); err != nil {
		return "", "", 0, false, err
	}
	if *limit < 0 {
		return "", "", 0, false, fmt.Errorf("persona candidates list: --limit must be >= 0")
	}
	resolved, err := defaultWorkDir(*workDir)
	if err != nil {
		return "", "", 0, false, err
	}
	wantState := persona.NormalizeCandidateState(persona.PersonaCandidateState(strings.TrimSpace(*state)))
	switch wantState {
	case persona.PersonaCandidateOpen, persona.PersonaCandidateDrafted, persona.PersonaCandidateDismissed:
	default:
		return "", "", 0, false, fmt.Errorf("persona candidates list: --state must be one of open, drafted, dismissed")
	}
	return resolved, wantState, *limit, *asJSON, nil
}

func parsePersonaShowDismissFlags(name string, args []string, stderr io.Writer) (string, string, bool, error) {
	flags := flag.NewFlagSet(name, flag.ContinueOnError)
	flags.SetOutput(stderr)
	workDir := flags.String("workdir", "", "workdir that contains vault/ and state/")
	asJSON := flags.Bool("json", false, "emit the candidate as a JSON object (show only; dismiss ignores this flag)")
	args = reorderFlagsBeforePositionals(args, flags)
	if err := flags.Parse(args); err != nil {
		return "", "", false, err
	}
	remaining := flags.Args()
	if len(remaining) == 0 || strings.TrimSpace(remaining[0]) == "" {
		return "", "", false, fmt.Errorf("%s: persona candidate id is required", name)
	}
	resolved, err := defaultWorkDir(*workDir)
	if err != nil {
		return "", "", false, err
	}
	return resolved, strings.TrimSpace(remaining[0]), *asJSON, nil
}

func parsePersonaDraftFlags(args []string, stderr io.Writer) (string, string, bool, error) {
	flags := flag.NewFlagSet("persona candidates draft", flag.ContinueOnError)
	flags.SetOutput(stderr)
	workDir := flags.String("workdir", "", "workdir that contains vault/ and state/")
	retryRejected := flags.Bool("retry-rejected", false, "retry a candidate whose previous draft is rejected, expired, or superseded")
	args = reorderFlagsBeforePositionals(args, flags)
	if err := flags.Parse(args); err != nil {
		return "", "", false, err
	}
	remaining := flags.Args()
	if len(remaining) == 0 || strings.TrimSpace(remaining[0]) == "" {
		return "", "", false, fmt.Errorf("persona candidates draft: persona candidate id is required")
	}
	resolved, err := defaultWorkDir(*workDir)
	if err != nil {
		return "", "", false, err
	}
	return resolved, strings.TrimSpace(remaining[0]), *retryRejected, nil
}

func parsePersonaRecoverFlags(args []string, stderr io.Writer) (string, string, string, bool, error) {
	flags := flag.NewFlagSet("persona candidates recover", flag.ContinueOnError)
	flags.SetOutput(stderr)
	workDir := flags.String("workdir", "", "workdir that contains vault/ and state/")
	linkDraftID := flags.String("link", "", "link the candidate to an existing orphan persona_update draft")
	forceDismiss := flags.Bool("force-dismiss", false, "abandon a partial drafted candidate by transitioning it to dismissed")
	args = reorderFlagsBeforePositionals(args, flags)
	if err := flags.Parse(args); err != nil {
		return "", "", "", false, err
	}
	remaining := flags.Args()
	if len(remaining) == 0 || strings.TrimSpace(remaining[0]) == "" {
		return "", "", "", false, fmt.Errorf("persona candidates recover: persona candidate id is required")
	}
	linkSet := strings.TrimSpace(*linkDraftID) != ""
	if linkSet && *forceDismiss {
		return "", "", "", false, fmt.Errorf("persona candidates recover: --link and --force-dismiss are mutually exclusive")
	}
	if !linkSet && !*forceDismiss {
		return "", "", "", false, fmt.Errorf("persona candidates recover: exactly one of --link <draft-id> or --force-dismiss is required")
	}
	resolved, err := defaultWorkDir(*workDir)
	if err != nil {
		return "", "", "", false, err
	}
	return resolved, strings.TrimSpace(remaining[0]), strings.TrimSpace(*linkDraftID), *forceDismiss, nil
}

func renderPersonaDraftErrorHint(stderr io.Writer, candidateID string, retryRejected bool, err error) {
	switch {
	case errors.Is(err, app.ErrPersonaCandidateDismissed):
		fmt.Fprintln(stderr, "  hint: this candidate was dismissed; its DedupKey is a tombstone.")
		fmt.Fprintln(stderr, "        the underlying fact will not be re-extracted from a console turn unless")
		fmt.Fprintln(stderr, "        the user repeats the utterance with materially different evidence.")
	case errors.Is(err, app.ErrPersonaCandidateAlreadyDrafted):
		fmt.Fprintln(stderr, "  hint: the candidate is in the partial-orphan shape (Drafted with empty DraftID).")
		fmt.Fprintln(stderr, "        either link it to the orphan persona_update draft (find via `lore draft list`):")
		fmt.Fprintf(stderr, "          lore persona candidates recover --link <draft-id> %s\n", candidateID)
		fmt.Fprintln(stderr, "        or abandon it (DedupKey kept as tombstone):")
		fmt.Fprintf(stderr, "          lore persona candidates recover --force-dismiss %s\n", candidateID)
	case errors.Is(err, app.ErrPersonaCandidateLinkedStateRequired):
		if retryRejected {
			fmt.Fprintln(stderr, "  hint: --retry-rejected only applies to candidates already promoted once.")
			fmt.Fprintf(stderr, "        for a first-time promote, drop the flag:\n          lore persona candidates draft %s\n", candidateID)
			fmt.Fprintln(stderr, "        for a candidate in the partial-orphan shape (Drafted, no DraftID),")
			fmt.Fprintln(stderr, "        use `recover --link` or `recover --force-dismiss` instead.")
		}
	case errors.Is(err, app.ErrPersonaDraftNotTerminalForRetry):
		fmt.Fprintln(stderr, "  hint: the linked draft is still in flight (pending_review / approved / applied).")
		fmt.Fprintln(stderr, "        review it via:")
		fmt.Fprintln(stderr, "          lore draft list                      # locate the linked draft")
		fmt.Fprintln(stderr, "          lore draft review <draft-id>         # inspect it")
		fmt.Fprintln(stderr, "          lore draft reject <draft-id>         # if you want to retry afterwards")
		fmt.Fprintln(stderr, "        retry-rejected only fires after the linked draft reaches rejected /")
		fmt.Fprintln(stderr, "        expired / superseded.")
	}
}

func renderPersonaRecoverErrorHint(stderr io.Writer, candidateID, draftID string, forceDismiss bool, err error) {
	switch {
	case errors.Is(err, app.ErrPersonaCandidatePartialStateRequired):
		fmt.Fprintln(stderr, "  hint: recover only operates on candidates in the partial-orphan shape (Drafted with empty DraftID).")
		fmt.Fprintln(stderr, "        confirm the candidate's current shape:")
		fmt.Fprintf(stderr, "          lore persona candidates show %s\n", candidateID)
		if forceDismiss {
			fmt.Fprintln(stderr, "        if it is in Open state, use the regular dismiss path:")
			fmt.Fprintf(stderr, "          lore persona candidates dismiss %s\n", candidateID)
		} else {
			fmt.Fprintln(stderr, "        if it is Open, promote via:")
			fmt.Fprintf(stderr, "          lore persona candidates draft %s\n", candidateID)
		}
		fmt.Fprintln(stderr, "        if it is Drafted with a non-empty DraftID, reject the linked draft first:")
		fmt.Fprintln(stderr, "          lore draft reject <draft-id>")
	case errors.Is(err, app.ErrPersonaDraftKindMismatch):
		fmt.Fprintf(stderr, "  hint: --link only accepts a persona_update draft. The draft %q exists but is a different kind.\n", draftID)
		fmt.Fprintln(stderr, "        list drafts and pick a persona_update entry:")
		fmt.Fprintln(stderr, "          lore draft list")
	default:
		if strings.Contains(err.Error(), "not found") {
			fmt.Fprintln(stderr, "  hint: a referenced ID was not found.")
			fmt.Fprintln(stderr, "        confirm the candidate ID via:")
			fmt.Fprintf(stderr, "          lore persona candidates show %s\n", candidateID)
			if !forceDismiss && draftID != "" {
				fmt.Fprintf(stderr, "        confirm the draft ID %q via:\n", draftID)
				fmt.Fprintln(stderr, "          lore draft list")
			}
		}
	}
}

// personaCandidateOut is the JSON shape for one persona candidate.
// Single source of truth for `lore persona candidates show --json`
// and the `candidates[]` element shape of `lore persona candidates
// list --json`. Keeping it as a named type avoids field drift between
// the two endpoints -- any future column additions land in both
// places automatically.
type personaCandidateOut struct {
	ID              string `json:"id"`
	State           string `json:"state"`
	DraftID         string `json:"draft_id"`
	DedupKey        string `json:"dedup_key"`
	CreatedAt       string `json:"created_at"`
	UpdatedAt       string `json:"updated_at"`
	Field           string `json:"field"`
	ProposedValue   string `json:"proposed_value"`
	CurrentValue    string `json:"current_value"`
	EvidenceQuote   string `json:"evidence_quote"`
	Reason          string `json:"reason"`
	Confidence      string `json:"confidence"`
	Conflict        bool   `json:"conflict"`
	SourceKind      string `json:"source_kind"`
	SourceSessionID string `json:"source_session_id"`
	ObservedAt      string `json:"observed_at"`
}

// newPersonaCandidateOut converts a runtime record to its JSON
// projection. Timestamps are normalized to UTC + RFC3339Nano so
// scripted consumers see a stable, sortable string regardless of
// the operator's local timezone.
func newPersonaCandidateOut(r persona.PersonaCandidateRecord) personaCandidateOut {
	return personaCandidateOut{
		ID:              r.ID,
		State:           string(r.State),
		DraftID:         r.DraftID,
		DedupKey:        r.DedupKey,
		CreatedAt:       r.CreatedAt.UTC().Format(time.RFC3339Nano),
		UpdatedAt:       r.UpdatedAt.UTC().Format(time.RFC3339Nano),
		Field:           r.Candidate.Field,
		ProposedValue:   r.Candidate.ProposedValue,
		CurrentValue:    r.Candidate.CurrentValue,
		EvidenceQuote:   r.Candidate.EvidenceQuote,
		Reason:          r.Candidate.Reason,
		Confidence:      string(r.Candidate.Confidence),
		Conflict:        r.Candidate.Conflict,
		SourceKind:      string(r.Candidate.SourceKind),
		SourceSessionID: r.Candidate.SourceSessionID,
		ObservedAt:      r.Candidate.ObservedAt.UTC().Format(time.RFC3339Nano),
	}
}

func emitPersonaCandidateDetailJSON(stdout io.Writer, r persona.PersonaCandidateRecord) error {
	out := newPersonaCandidateOut(r)
	encoded, err := json.Marshal(out)
	if err != nil {
		return err
	}
	if _, err := stdout.Write(encoded); err != nil {
		return err
	}
	_, err = fmt.Fprintln(stdout)
	return err
}

func emitPersonaCandidateListJSON(stdout io.Writer, workDir string, state persona.PersonaCandidateState, limit int, records []persona.PersonaCandidateRecord) error {
	type payload struct {
		Workdir    string                `json:"workdir"`
		State      string                `json:"state"`
		Limit      int                   `json:"limit"`
		Count      int                   `json:"count"`
		Candidates []personaCandidateOut `json:"candidates"`
	}
	out := payload{
		Workdir:    workDir,
		State:      string(state),
		Limit:      limit,
		Count:      len(records),
		Candidates: []personaCandidateOut{},
	}
	for _, r := range records {
		out.Candidates = append(out.Candidates, newPersonaCandidateOut(r))
	}
	encoded, err := json.Marshal(out)
	if err != nil {
		return err
	}
	if _, err := stdout.Write(encoded); err != nil {
		return err
	}
	_, err = fmt.Fprintln(stdout)
	return err
}

func renderPersonaCandidateList(stdout io.Writer, state persona.PersonaCandidateState, records []persona.PersonaCandidateRecord) {
	fmt.Fprintln(stdout, "Persona Candidates")
	fmt.Fprintln(stdout, "==================")
	fmt.Fprintf(stdout, "State: %s\n", state)
	fmt.Fprintln(stdout)
	if len(records) == 0 {
		fmt.Fprintln(stdout, "No candidates.")
		return
	}
	fmt.Fprintf(stdout, "%s\t%s\t%s\t%s\t%s\t%s\t%s\n", "ID", "FIELD", "PROPOSED", "CONFIDENCE", "CONFLICT", "SOURCE", "DRAFT")
	for _, record := range records {
		conflict := "no"
		if record.Candidate.Conflict {
			conflict = "yes"
		}
		// DRAFT column carries the linked draft ID when the candidate
		// has been promoted. Empty / partial / open / dismissed rows
		// render "-" so awk/cut pipelines keep a stable 7-column
		// schema regardless of state filter.
		draftID := strings.TrimSpace(record.DraftID)
		if draftID == "" {
			draftID = "-"
		}
		fmt.Fprintf(
			stdout,
			"%s\t%s\t%s\t%s\t%s\t%s\t%s\n",
			record.ID,
			record.Candidate.Field,
			clipOneLine(record.Candidate.ProposedValue, 40),
			record.Candidate.Confidence,
			conflict,
			record.Candidate.SourceKind,
			draftID,
		)
	}
	fmt.Fprintln(stdout)
	fmt.Fprintln(stdout, "Next:")
	fmt.Fprintln(stdout, "  lore persona candidates show <id>")
	fmt.Fprintln(stdout, "  lore persona candidates draft <id>")
	fmt.Fprintln(stdout, "  lore persona candidates dismiss <id>")
}

func renderPersonaCandidateDetail(stdout io.Writer, record persona.PersonaCandidateRecord) {
	fmt.Fprintln(stdout, "Persona Candidate")
	fmt.Fprintln(stdout, "=================")
	fmt.Fprintf(stdout, "ID:             %s\n", record.ID)
	fmt.Fprintf(stdout, "State:          %s\n", record.State)
	if strings.TrimSpace(record.DraftID) != "" {
		fmt.Fprintf(stdout, "Draft:          %s\n", record.DraftID)
	}
	fmt.Fprintf(stdout, "Field:          %s\n", record.Candidate.Field)
	fmt.Fprintf(stdout, "Proposed value: %s\n", record.Candidate.ProposedValue)
	if strings.TrimSpace(record.Candidate.CurrentValue) != "" {
		fmt.Fprintf(stdout, "Current value:  %s\n", record.Candidate.CurrentValue)
	}
	fmt.Fprintf(stdout, "Evidence:       %s\n", record.Candidate.EvidenceQuote)
	if strings.TrimSpace(record.Candidate.Reason) != "" {
		fmt.Fprintf(stdout, "Reason:         %s\n", record.Candidate.Reason)
	}
	fmt.Fprintf(stdout, "Confidence:     %s\n", record.Candidate.Confidence)
	fmt.Fprintf(stdout, "Conflict:       %v\n", record.Candidate.Conflict)
	fmt.Fprintf(stdout, "Source:         %s\n", record.Candidate.SourceKind)
	if strings.TrimSpace(record.Candidate.SourceSessionID) != "" {
		fmt.Fprintf(stdout, "Session:        %s\n", record.Candidate.SourceSessionID)
	}
	fmt.Fprintf(stdout, "Observed at:    %s\n", record.Candidate.ObservedAt.Format(time.RFC3339))
	fmt.Fprintf(stdout, "Created at:     %s\n", record.CreatedAt.Format(time.RFC3339))
	fmt.Fprintf(stdout, "Updated at:     %s\n", record.UpdatedAt.Format(time.RFC3339))
	fmt.Fprintf(stdout, "Dedup key:      %s\n", record.DedupKey)
}

func renderPersonaCandidateActionResult(stdout io.Writer, action string, record persona.PersonaCandidateRecord, draftID string) {
	fmt.Fprintln(stdout, "Persona Candidate Updated")
	fmt.Fprintln(stdout, "=========================")
	fmt.Fprintf(stdout, "Action: %s\n", action)
	fmt.Fprintf(stdout, "ID:     %s\n", record.ID)
	fmt.Fprintf(stdout, "State:  %s\n", record.State)
	fmt.Fprintf(stdout, "Field:  %s\n", record.Candidate.Field)
	if draftID != "" {
		fmt.Fprintf(stdout, "Draft:  %s (pending_review; use `lore draft review %s`)\n", draftID, draftID)
	}
}
