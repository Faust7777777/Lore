package cli

import (
	"bytes"
	"errors"
	"strings"
	"testing"
	"time"

	"obsidian-harness/internal/app"
	"obsidian-harness/internal/config/configtest"
	"obsidian-harness/internal/model"
	"obsidian-harness/internal/persona"
	"obsidian-harness/internal/store"
)

func openRuntimeForPersonaCLITest(t *testing.T, workDir string) *app.Runtime {
	t.Helper()
	runtime, err := app.OpenRuntimeWithConfigOptions(workDir, configtest.IsolatedOptions(t))
	if err != nil {
		t.Fatalf("OpenRuntime() error = %v", err)
	}
	t.Cleanup(func() { _ = runtime.Close() })
	return runtime
}

// seedPersonaCandidateCLI creates a persona candidate against the
// given workdir and returns its ID. The runtime is closed before
// return so the CLI under test can open its own fresh handle to the
// sqlite store.
func seedPersonaCandidateCLI(t *testing.T, workDir, field, value, evidence string) string {
	t.Helper()
	runtime, err := app.OpenRuntimeWithConfigOptions(workDir, configtest.IsolatedOptions(t))
	if err != nil {
		t.Fatalf("seed OpenRuntime() error = %v", err)
	}
	if _, err := runtime.Bootstrap(time.Date(2026, 5, 20, 10, 0, 0, 0, time.UTC)); err != nil {
		runtime.Close()
		t.Fatalf("Bootstrap() error = %v", err)
	}
	c := persona.PersonaCandidate{
		Field:           field,
		ProposedValue:   value,
		EvidenceQuote:   evidence,
		Reason:          "test seed",
		Confidence:      persona.ConfidenceHigh,
		SourceKind:      persona.SourceConsole,
		SourceSessionID: "lore-cli-test",
		ObservedAt:      time.Date(2026, 5, 20, 10, 0, 0, 0, time.UTC),
	}
	now := time.Date(2026, 5, 20, 10, 30, 0, 0, time.UTC)
	rec := persona.PersonaCandidateRecord{
		ID:        persona.NewCandidateID(now),
		State:     persona.PersonaCandidateOpen,
		DedupKey:  persona.DedupKey(c),
		Candidate: c,
		CreatedAt: now,
		UpdatedAt: now,
	}
	stored, isNew, err := runtime.RecordPersonaCandidate(rec)
	if err != nil || !isNew {
		runtime.Close()
		t.Fatalf("RecordPersonaCandidate isNew=%v err=%v", isNew, err)
	}
	if err := runtime.Close(); err != nil {
		t.Fatalf("close runtime after seed: %v", err)
	}
	return stored.ID
}

func TestRunPersonaCandidatesList(t *testing.T) {
	configtest.IsolateHome(t)
	workDir := t.TempDir()
	id := seedPersonaCandidateCLI(t, workDir, "major", "economics", "I major in economics")

	var stdout, stderr bytes.Buffer
	exitCode := Run([]string{"persona", "candidates", "list", "--workdir", workDir}, &bytes.Buffer{}, &stdout, &stderr, "test")
	if exitCode != 0 {
		t.Fatalf("exit = %d, stderr=%q", exitCode, stderr.String())
	}
	out := stdout.String()
	for _, want := range []string{"Persona Candidates", "State: open", id, "major", "economics", "high"} {
		if !strings.Contains(out, want) {
			t.Fatalf("output missing %q:\n%s", want, out)
		}
	}
}

func TestRunPersonaCandidatesListEmpty(t *testing.T) {
	configtest.IsolateHome(t)
	workDir := t.TempDir()
	openRuntimeForPersonaCLITest(t, workDir) // pre-create state dir

	var stdout, stderr bytes.Buffer
	exitCode := Run([]string{"persona", "candidates", "list", "--workdir", workDir}, &bytes.Buffer{}, &stdout, &stderr, "test")
	if exitCode != 0 {
		t.Fatalf("exit = %d, stderr=%q", exitCode, stderr.String())
	}
	if !strings.Contains(stdout.String(), "No candidates.") {
		t.Fatalf("output missing friendly empty message:\n%s", stdout.String())
	}
}

func TestRunPersonaCandidatesListStateFilter(t *testing.T) {
	configtest.IsolateHome(t)
	workDir := t.TempDir()
	id := seedPersonaCandidateCLI(t, workDir, "major", "economics", "I major in economics")
	// Dismiss the candidate via the CLI itself, then list by
	// dismissed and assert it shows up there but not in open.
	var stdout, stderr bytes.Buffer
	if exit := Run([]string{"persona", "candidates", "dismiss", "--workdir", workDir, id}, &bytes.Buffer{}, &stdout, &stderr, "test"); exit != 0 {
		t.Fatalf("dismiss exit = %d, stderr=%q", exit, stderr.String())
	}

	stdout.Reset()
	stderr.Reset()
	if exit := Run([]string{"persona", "candidates", "list", "--workdir", workDir, "--state", "dismissed"}, &bytes.Buffer{}, &stdout, &stderr, "test"); exit != 0 {
		t.Fatalf("list dismissed exit = %d, stderr=%q", exit, stderr.String())
	}
	if !strings.Contains(stdout.String(), id) || !strings.Contains(stdout.String(), "State: dismissed") {
		t.Fatalf("dismissed list missing seeded id; got:\n%s", stdout.String())
	}

	stdout.Reset()
	stderr.Reset()
	if exit := Run([]string{"persona", "candidates", "list", "--workdir", workDir, "--state", "open"}, &bytes.Buffer{}, &stdout, &stderr, "test"); exit != 0 {
		t.Fatalf("list open exit = %d, stderr=%q", exit, stderr.String())
	}
	if strings.Contains(stdout.String(), id) {
		t.Fatalf("open list should NOT contain dismissed id; got:\n%s", stdout.String())
	}
	if !strings.Contains(stdout.String(), "No candidates.") {
		t.Fatalf("open list missing empty marker; got:\n%s", stdout.String())
	}
}

func TestRunPersonaCandidatesListRejectsNegativeLimit(t *testing.T) {
	// Numeric-flag consistency: parsePersonaErrorsFlags --tail and
	// parseUsageFlags --days both reject negative values. --limit
	// previously fell through and was silently treated as "no cap"
	// by the store, surprising the operator. Verify the rejection
	// hits before OpenRuntime so a typo fails fast without touching
	// disk.
	configtest.IsolateHome(t)
	workDir := t.TempDir()

	var stdout, stderr bytes.Buffer
	exitCode := Run([]string{"persona", "candidates", "list", "--workdir", workDir, "--limit", "-1"}, &bytes.Buffer{}, &stdout, &stderr, "test")
	if exitCode == 0 {
		t.Fatalf("expected non-zero exit for --limit -1; stdout=%q", stdout.String())
	}
	if !strings.Contains(stderr.String(), "--limit must be >= 0") {
		t.Fatalf("stderr missing --limit validation message: %q", stderr.String())
	}
}

func TestRunPersonaCandidatesListRejectsInvalidState(t *testing.T) {
	configtest.IsolateHome(t)
	workDir := t.TempDir()
	openRuntimeForPersonaCLITest(t, workDir)

	var stdout, stderr bytes.Buffer
	exitCode := Run([]string{"persona", "candidates", "list", "--workdir", workDir, "--state", "bogus"}, &bytes.Buffer{}, &stdout, &stderr, "test")
	if exitCode == 0 {
		t.Fatalf("expected non-zero exit for invalid state; stdout=%q", stdout.String())
	}
	if !strings.Contains(stderr.String(), "must be one of") {
		t.Fatalf("stderr missing validation message: %q", stderr.String())
	}
}

func TestRunPersonaCandidatesShow(t *testing.T) {
	configtest.IsolateHome(t)
	workDir := t.TempDir()
	id := seedPersonaCandidateCLI(t, workDir, "major", "economics", "I major in economics")

	var stdout, stderr bytes.Buffer
	exitCode := Run([]string{"persona", "candidates", "show", "--workdir", workDir, id}, &bytes.Buffer{}, &stdout, &stderr, "test")
	if exitCode != 0 {
		t.Fatalf("exit = %d, stderr=%q", exitCode, stderr.String())
	}
	for _, want := range []string{
		id,
		"Field:          major",
		"Proposed value: economics",
		"Evidence:       I major in economics",
		"Confidence:     high",
		"Source:         console",
		"Dedup key:",
	} {
		if !strings.Contains(stdout.String(), want) {
			t.Fatalf("output missing %q:\n%s", want, stdout.String())
		}
	}
}

func TestRunPersonaCandidatesShowMissingIDFails(t *testing.T) {
	configtest.IsolateHome(t)
	workDir := t.TempDir()
	openRuntimeForPersonaCLITest(t, workDir)

	var stdout, stderr bytes.Buffer
	exitCode := Run([]string{"persona", "candidates", "show", "--workdir", workDir}, &bytes.Buffer{}, &stdout, &stderr, "test")
	if exitCode == 0 {
		t.Fatalf("expected non-zero exit for missing id; stdout=%q", stdout.String())
	}
	if !strings.Contains(stderr.String(), "persona candidate id is required") {
		t.Fatalf("stderr missing required-id message: %q", stderr.String())
	}
}

func TestRunPersonaCandidatesDismissTransitionsState(t *testing.T) {
	configtest.IsolateHome(t)
	workDir := t.TempDir()
	id := seedPersonaCandidateCLI(t, workDir, "major", "economics", "I major in economics")

	var stdout, stderr bytes.Buffer
	exitCode := Run([]string{"persona", "candidates", "dismiss", "--workdir", workDir, id}, &bytes.Buffer{}, &stdout, &stderr, "test")
	if exitCode != 0 {
		t.Fatalf("exit = %d, stderr=%q", exitCode, stderr.String())
	}
	if !strings.Contains(stdout.String(), "Action: dismiss") || !strings.Contains(stdout.String(), "State:  dismissed") {
		t.Fatalf("output missing dismissed transition:\n%s", stdout.String())
	}

	runtime := openRuntimeForPersonaCLITest(t, workDir)
	got, err := runtime.GetPersonaCandidate(id)
	if err != nil {
		t.Fatalf("post-dismiss GetPersonaCandidate() error = %v", err)
	}
	if got.State != persona.PersonaCandidateDismissed {
		t.Fatalf("state = %q, want dismissed", got.State)
	}
}

func TestRunPersonaCandidatesDraftOnDismissedSurfacesTombstoneHint(t *testing.T) {
	// UX polish: dismissing a candidate parks its DedupKey as a
	// tombstone. A naive operator who then tries `draft <id>` sees
	// only "app: persona candidate is dismissed" -- they need to
	// know the tombstone semantics so they can choose between
	// repeating the utterance or accepting that the fact is closed.
	configtest.IsolateHome(t)
	workDir := t.TempDir()
	candidateID := seedPersonaCandidateCLI(t, workDir, "major", "economics", "I major in economics")
	if exit := Run([]string{"persona", "candidates", "dismiss", "--workdir", workDir, candidateID}, &bytes.Buffer{}, &bytes.Buffer{}, &bytes.Buffer{}, "test"); exit != 0 {
		t.Fatalf("seed dismiss exit = %d", exit)
	}

	var stdout, stderr bytes.Buffer
	exit := Run([]string{"persona", "candidates", "draft", "--workdir", workDir, candidateID}, &bytes.Buffer{}, &stdout, &stderr, "test")
	if exit == 0 {
		t.Fatalf("expected non-zero exit for draft on dismissed candidate; stdout=%q", stdout.String())
	}
	for _, want := range []string{
		"persona candidate is dismissed",
		"DedupKey",
		"tombstone",
	} {
		if !strings.Contains(stderr.String(), want) {
			t.Fatalf("stderr missing hint fragment %q:\n%s", want, stderr.String())
		}
	}
}

func TestRunPersonaCandidatesDraftOnPartialOrphanSurfacesRecoverHint(t *testing.T) {
	// Mirror of the dismiss hint for the draft path. When the
	// candidate is in (Drafted, DraftID=""), a blind `draft <id>`
	// trips ErrPersonaCandidateAlreadyDrafted. The hint should
	// tell the operator the two recover branches with the actual
	// candidate ID baked in.
	configtest.IsolateHome(t)
	workDir := t.TempDir()
	partialID := forcePartialDraftedCandidateCLI(t, workDir, "major", "economics", "I major in economics")

	var stdout, stderr bytes.Buffer
	exit := Run([]string{"persona", "candidates", "draft", "--workdir", workDir, partialID}, &bytes.Buffer{}, &stdout, &stderr, "test")
	if exit == 0 {
		t.Fatalf("expected non-zero exit for draft on partial orphan; stdout=%q", stdout.String())
	}
	for _, want := range []string{
		"already drafted",
		"partial-orphan",
		"recover --link",
		"recover --force-dismiss " + partialID,
	} {
		if !strings.Contains(stderr.String(), want) {
			t.Fatalf("stderr missing hint fragment %q:\n%s", want, stderr.String())
		}
	}
}

func TestRunPersonaCandidatesDraftRetryRejectedOnOpenSurfacesHint(t *testing.T) {
	// --retry-rejected on an Open candidate trips
	// ErrPersonaCandidateLinkedStateRequired. Hint should point
	// the operator at the first-time-promote command (no flag).
	configtest.IsolateHome(t)
	workDir := t.TempDir()
	candidateID := seedPersonaCandidateCLI(t, workDir, "major", "economics", "I major in economics")

	var stdout, stderr bytes.Buffer
	exit := Run([]string{"persona", "candidates", "draft", "--workdir", workDir, "--retry-rejected", candidateID}, &bytes.Buffer{}, &stdout, &stderr, "test")
	if exit == 0 {
		t.Fatalf("expected non-zero exit; stdout=%q", stdout.String())
	}
	for _, want := range []string{
		"--retry-rejected only applies",
		"lore persona candidates draft " + candidateID,
	} {
		if !strings.Contains(stderr.String(), want) {
			t.Fatalf("stderr missing hint fragment %q:\n%s", want, stderr.String())
		}
	}
}

func TestRunPersonaCandidatesDraftRetryRejectedOnPendingDraftSurfacesHint(t *testing.T) {
	// --retry-rejected on a candidate whose linked draft is still
	// pending_review trips ErrPersonaDraftNotTerminalForRetry. The
	// hint should walk the operator through reject/review.
	configtest.IsolateHome(t)
	workDir := t.TempDir()
	candidateID := seedPersonaCandidateCLI(t, workDir, "major", "economics", "I major in economics")
	if exit := Run([]string{"persona", "candidates", "draft", "--workdir", workDir, candidateID}, &bytes.Buffer{}, &bytes.Buffer{}, &bytes.Buffer{}, "test"); exit != 0 {
		t.Fatalf("seed draft exit = %d", exit)
	}

	var stdout, stderr bytes.Buffer
	exit := Run([]string{"persona", "candidates", "draft", "--workdir", workDir, "--retry-rejected", candidateID}, &bytes.Buffer{}, &stdout, &stderr, "test")
	if exit == 0 {
		t.Fatalf("expected non-zero exit; stdout=%q", stdout.String())
	}
	for _, want := range []string{
		"linked draft is still in flight",
		"lore draft reject",
		"retry-rejected only fires after",
	} {
		if !strings.Contains(stderr.String(), want) {
			t.Fatalf("stderr missing hint fragment %q:\n%s", want, stderr.String())
		}
	}
}

func TestRunPersonaCandidatesDismissOnDraftedSurfacesRecoverHint(t *testing.T) {
	// UX polish: dismiss deliberately refuses Drafted candidates
	// (the linked draft owns the review path). The CLI should not
	// just print the raw error; it should tell the operator which
	// follow-up command to use -- recover --force-dismiss for the
	// partial-orphan shape, or `lore draft reject` for a linked
	// draft. Otherwise the operator has to dig through docs.
	configtest.IsolateHome(t)
	workDir := t.TempDir()
	candidateID := seedPersonaCandidateCLI(t, workDir, "major", "economics", "I major in economics")

	// Promote the candidate to Drafted (linked) via the normal CLI path.
	if exit := Run([]string{"persona", "candidates", "draft", "--workdir", workDir, candidateID}, &bytes.Buffer{}, &bytes.Buffer{}, &bytes.Buffer{}, "test"); exit != 0 {
		t.Fatalf("seed draft exit = %d", exit)
	}

	var stdout, stderr bytes.Buffer
	exit := Run([]string{"persona", "candidates", "dismiss", "--workdir", workDir, candidateID}, &bytes.Buffer{}, &stdout, &stderr, "test")
	if exit == 0 {
		t.Fatalf("expected non-zero exit when dismissing a drafted candidate; stdout=%q", stdout.String())
	}
	for _, want := range []string{
		"persona candidates dismiss",
		"persona candidate already drafted",
		"recover --force-dismiss",
		"lore draft reject",
		candidateID,
	} {
		if !strings.Contains(stderr.String(), want) {
			t.Fatalf("stderr missing hint fragment %q:\n%s", want, stderr.String())
		}
	}
}

func TestRunPersonaCandidatesDraftCreatesPersonaUpdateDraft(t *testing.T) {
	configtest.IsolateHome(t)
	workDir := t.TempDir()
	id := seedPersonaCandidateCLI(t, workDir, "major", "economics", "I major in economics")

	var stdout, stderr bytes.Buffer
	exitCode := Run([]string{"persona", "candidates", "draft", "--workdir", workDir, id}, &bytes.Buffer{}, &stdout, &stderr, "test")
	if exitCode != 0 {
		t.Fatalf("exit = %d, stderr=%q", exitCode, stderr.String())
	}
	out := stdout.String()
	for _, want := range []string{"Action: draft", "State:  drafted", "Draft:", "pending_review"} {
		if !strings.Contains(out, want) {
			t.Fatalf("output missing %q:\n%s", want, out)
		}
	}

	// Reopen the runtime and confirm:
	//   - candidate state == drafted;
	//   - a persona_update draft exists in pending_review with the
	//     candidate evidence preserved in the summary.
	runtime := openRuntimeForPersonaCLITest(t, workDir)
	candidate, err := runtime.GetPersonaCandidate(id)
	if err != nil {
		t.Fatalf("GetPersonaCandidate() error = %v", err)
	}
	if candidate.State != persona.PersonaCandidateDrafted {
		t.Fatalf("candidate state = %q, want drafted", candidate.State)
	}

	drafts, err := runtime.Store.Drafts().ListDrafts()
	if err != nil {
		t.Fatalf("ListDrafts() error = %v", err)
	}
	var personaDraft *model.Draft
	for i := range drafts {
		if drafts[i].Kind == model.DraftKindPersonaUpdate {
			personaDraft = &drafts[i]
			break
		}
	}
	if personaDraft == nil {
		t.Fatalf("no persona_update draft found; drafts = %+v", drafts)
	}
	if personaDraft.State != model.DraftPendingReview {
		t.Fatalf("draft state = %q, want pending_review (CLI must not approve/apply)", personaDraft.State)
	}
	if !strings.Contains(personaDraft.Summary, "I major in economics") {
		t.Fatalf("draft summary missing evidence:\n%s", personaDraft.Summary)
	}
	if !strings.Contains(personaDraft.Summary, "confidence: high") {
		t.Fatalf("draft summary missing confidence:\n%s", personaDraft.Summary)
	}
}

func TestRunPersonaCandidatesDraftIsIdempotentOnRetry(t *testing.T) {
	// CLI re-invocation must NOT create a duplicate persona_update
	// draft. The second call returns the same DraftID and the store
	// still has exactly one persona_update draft.
	configtest.IsolateHome(t)
	workDir := t.TempDir()
	id := seedPersonaCandidateCLI(t, workDir, "major", "economics", "I major in economics")

	var firstStdout bytes.Buffer
	if exit := Run([]string{"persona", "candidates", "draft", "--workdir", workDir, id}, &bytes.Buffer{}, &firstStdout, &bytes.Buffer{}, "test"); exit != 0 {
		t.Fatalf("first draft exit = %d", exit)
	}
	firstDraftID := extractDraftIDFromOutput(t, firstStdout.String())

	var secondStdout, secondStderr bytes.Buffer
	exitCode := Run([]string{"persona", "candidates", "draft", "--workdir", workDir, id}, &bytes.Buffer{}, &secondStdout, &secondStderr, "test")
	if exitCode != 0 {
		t.Fatalf("second draft exit = %d (want idempotent success); stderr=%q", exitCode, secondStderr.String())
	}
	secondDraftID := extractDraftIDFromOutput(t, secondStdout.String())
	if secondDraftID != firstDraftID {
		t.Fatalf("second DraftID = %q, want same as first %q (idempotent retry must reuse draft)", secondDraftID, firstDraftID)
	}

	// Underlying store has exactly one persona_update draft.
	runtime := openRuntimeForPersonaCLITest(t, workDir)
	drafts, err := runtime.Store.Drafts().ListDrafts()
	if err != nil {
		t.Fatalf("ListDrafts() error = %v", err)
	}
	count := 0
	for _, d := range drafts {
		if d.Kind == model.DraftKindPersonaUpdate {
			count++
		}
	}
	if count != 1 {
		t.Fatalf("persona_update drafts in store = %d, want 1", count)
	}
}

func TestRunPersonaCandidatesListShowsDraftColumnForLinkedCandidate(t *testing.T) {
	// B-P10: list output gains a DRAFT column so the operator can see
	// each drafted candidate's linked DraftID without running show on
	// every row. Empty DraftID (open / dismissed / partial-orphan)
	// renders "-" so the column schema stays stable -- awk/cut
	// pipelines do not need to branch on state filter.
	configtest.IsolateHome(t)
	workDir := t.TempDir()
	candidateID := seedPersonaCandidateCLI(t, workDir, "major", "economics", "I major in economics")

	var draftStdout bytes.Buffer
	if exit := Run([]string{"persona", "candidates", "draft", "--workdir", workDir, candidateID}, &bytes.Buffer{}, &draftStdout, &bytes.Buffer{}, "test"); exit != 0 {
		t.Fatalf("draft exit = %d", exit)
	}
	draftID := extractDraftIDFromOutput(t, draftStdout.String())

	var listStdout, listStderr bytes.Buffer
	if exit := Run([]string{"persona", "candidates", "list", "--workdir", workDir, "--state", "drafted"}, &bytes.Buffer{}, &listStdout, &listStderr, "test"); exit != 0 {
		t.Fatalf("list exit = %d, stderr=%q", exit, listStderr.String())
	}
	out := listStdout.String()
	// Header must include the new column.
	if !strings.Contains(out, "DRAFT") {
		t.Fatalf("list header missing DRAFT column:\n%s", out)
	}
	// Data row must contain the linked DraftID verbatim.
	if !strings.Contains(out, draftID) {
		t.Fatalf("list row missing linked DraftID %q:\n%s", draftID, out)
	}
}

func TestRunPersonaCandidatesListDraftColumnDashForOpen(t *testing.T) {
	// Stable schema check: open candidates (no DraftID) must render
	// the column as "-" so consumers piping through cut/awk see a
	// constant number of columns. Blank would collapse adjacent tabs
	// and break field offsets.
	configtest.IsolateHome(t)
	workDir := t.TempDir()
	seedPersonaCandidateCLI(t, workDir, "major", "economics", "I major in economics")

	var stdout, stderr bytes.Buffer
	if exit := Run([]string{"persona", "candidates", "list", "--workdir", workDir}, &bytes.Buffer{}, &stdout, &stderr, "test"); exit != 0 {
		t.Fatalf("list exit = %d, stderr=%q", exit, stderr.String())
	}
	out := stdout.String()
	if !strings.Contains(out, "DRAFT") {
		t.Fatalf("list header missing DRAFT column on open state:\n%s", out)
	}
	// Find the data row (after the header) and assert it ends with
	// "\t-\n" so the last column is the explicit dash.
	dataRowSuffix := "\t-\n"
	if !strings.Contains(out, dataRowSuffix) {
		t.Fatalf("open candidate row should end with %q to keep 7-column schema stable:\n%s", dataRowSuffix, out)
	}
}

func TestRunPersonaCandidatesShowIncludesLinkedDraftID(t *testing.T) {
	// After `lore persona candidates draft <id>` links a DraftID onto
	// the candidate, `lore persona candidates show <id>` must display
	// that DraftID so operators can trace the candidate -> draft
	// relationship from the candidate view alone.
	configtest.IsolateHome(t)
	workDir := t.TempDir()
	id := seedPersonaCandidateCLI(t, workDir, "major", "economics", "I major in economics")

	var draftStdout bytes.Buffer
	if exit := Run([]string{"persona", "candidates", "draft", "--workdir", workDir, id}, &bytes.Buffer{}, &draftStdout, &bytes.Buffer{}, "test"); exit != 0 {
		t.Fatalf("draft exit = %d", exit)
	}
	draftID := extractDraftIDFromOutput(t, draftStdout.String())

	var showStdout, showStderr bytes.Buffer
	if exit := Run([]string{"persona", "candidates", "show", "--workdir", workDir, id}, &bytes.Buffer{}, &showStdout, &showStderr, "test"); exit != 0 {
		t.Fatalf("show exit = %d, stderr=%q", exit, showStderr.String())
	}
	showOut := showStdout.String()
	if !strings.Contains(showOut, "State:          drafted") {
		t.Fatalf("show output missing drafted state:\n%s", showOut)
	}
	wantLine := "Draft:          " + draftID
	if !strings.Contains(showOut, wantLine) {
		t.Fatalf("show output missing labeled %q line:\n%s", wantLine, showOut)
	}
}

// extractDraftIDFromOutput pulls the "Draft:  <id> ..." line out of
// the renderPersonaCandidateActionResult output so tests can compare
// DraftIDs across CLI invocations without parsing the full record.
func extractDraftIDFromOutput(t *testing.T, output string) string {
	t.Helper()
	for _, line := range strings.Split(output, "\n") {
		trimmed := strings.TrimSpace(line)
		if !strings.HasPrefix(trimmed, "Draft:") {
			continue
		}
		rest := strings.TrimSpace(strings.TrimPrefix(trimmed, "Draft:"))
		if rest == "" {
			continue
		}
		// "Draft:  draft-123 (pending_review; use ...)" -> "draft-123"
		fields := strings.Fields(rest)
		if len(fields) == 0 {
			continue
		}
		return fields[0]
	}
	t.Fatalf("output missing Draft: line:\n%s", output)
	return ""
}

func TestRunPersonaCandidatesShowReturnsNotFoundForMissingID(t *testing.T) {
	configtest.IsolateHome(t)
	workDir := t.TempDir()
	openRuntimeForPersonaCLITest(t, workDir)

	var stdout, stderr bytes.Buffer
	exitCode := Run([]string{"persona", "candidates", "show", "--workdir", workDir, "nonexistent"}, &bytes.Buffer{}, &stdout, &stderr, "test")
	if exitCode == 0 {
		t.Fatalf("expected non-zero exit for missing id; stdout=%q", stdout.String())
	}
	// The store-level error propagates via the CLI error message.
	if !strings.Contains(stderr.String(), "not found") && !errors.Is(errors.New(stderr.String()), store.ErrNotFound) {
		t.Fatalf("stderr missing not-found context: %q", stderr.String())
	}
}

func TestRunPersonaWithoutSubcommandShowsUsage(t *testing.T) {
	configtest.IsolateHome(t)
	var stdout, stderr bytes.Buffer
	exitCode := Run([]string{"persona"}, &bytes.Buffer{}, &stdout, &stderr, "test")
	if exitCode == 0 {
		t.Fatalf("expected non-zero exit for no subcommand")
	}
	// B-P11c reworked the usage line to dispatch between the
	// candidates and errors subgroups; assert both surfaces are
	// mentioned so the operator sees the full menu when typing
	// `lore persona` with no further args.
	for _, want := range []string{"lore persona", "candidates", "errors"} {
		if !strings.Contains(stderr.String(), want) {
			t.Fatalf("stderr missing usage hint %q: %q", want, stderr.String())
		}
	}
}

// forcePartialDraftedCandidateCLI seeds an Open candidate then forces
// it into the partial-failure state (Drafted + empty DraftID) using
// UpdateCandidateState. The runtime is closed before return so the
// CLI under test opens its own handle.
func forcePartialDraftedCandidateCLI(t *testing.T, workDir, field, value, evidence string) string {
	t.Helper()
	candidateID := seedPersonaCandidateCLI(t, workDir, field, value, evidence)
	runtime, err := app.OpenRuntimeWithConfigOptions(workDir, configtest.IsolatedOptions(t))
	if err != nil {
		t.Fatalf("forcePartial OpenRuntime: %v", err)
	}
	if _, err := runtime.Store.PersonaCandidates().UpdateCandidateState(candidateID, persona.PersonaCandidateDrafted, time.Date(2026, 5, 20, 11, 0, 0, 0, time.UTC)); err != nil {
		runtime.Close()
		t.Fatalf("forcePartial UpdateCandidateState: %v", err)
	}
	if err := runtime.Close(); err != nil {
		t.Fatalf("forcePartial close runtime: %v", err)
	}
	return candidateID
}

// seedLinkedAndRejectedPersonaCandidateCLI runs the normal CLI promote
// path and then rejects the resulting draft so the candidate is in
// State=Drafted with DraftID pointing at a Rejected draft. Returns
// (candidateID, originalDraftID).
func seedLinkedAndRejectedPersonaCandidateCLI(t *testing.T, workDir, field, value, evidence string) (string, string) {
	t.Helper()
	candidateID := seedPersonaCandidateCLI(t, workDir, field, value, evidence)
	var draftStdout bytes.Buffer
	if exit := Run([]string{"persona", "candidates", "draft", "--workdir", workDir, candidateID}, &bytes.Buffer{}, &draftStdout, &bytes.Buffer{}, "test"); exit != 0 {
		t.Fatalf("seed draft exit = %d", exit)
	}
	draftID := extractDraftIDFromOutput(t, draftStdout.String())
	runtime, err := app.OpenRuntimeWithConfigOptions(workDir, configtest.IsolatedOptions(t))
	if err != nil {
		t.Fatalf("seedRejected OpenRuntime: %v", err)
	}
	if _, err := runtime.RejectDraft(draftID); err != nil {
		runtime.Close()
		t.Fatalf("seedRejected RejectDraft: %v", err)
	}
	if err := runtime.Close(); err != nil {
		t.Fatalf("seedRejected close runtime: %v", err)
	}
	return candidateID, draftID
}

func TestRunPersonaCandidatesRecoverForceDismiss(t *testing.T) {
	configtest.IsolateHome(t)
	workDir := t.TempDir()
	partialID := forcePartialDraftedCandidateCLI(t, workDir, "major", "economics", "I major in economics")

	var stdout, stderr bytes.Buffer
	exit := Run([]string{"persona", "candidates", "recover", "--workdir", workDir, "--force-dismiss", partialID}, &bytes.Buffer{}, &stdout, &stderr, "test")
	if exit != 0 {
		t.Fatalf("exit = %d, stderr=%q", exit, stderr.String())
	}
	out := stdout.String()
	if !strings.Contains(out, "Action: recover --force-dismiss") {
		t.Fatalf("output missing action label:\n%s", out)
	}
	if !strings.Contains(out, "State:  dismissed") {
		t.Fatalf("output missing dismissed transition:\n%s", out)
	}

	// Verify in-store state via a fresh runtime handle.
	runtime := openRuntimeForPersonaCLITest(t, workDir)
	got, err := runtime.GetPersonaCandidate(partialID)
	if err != nil {
		t.Fatalf("post-recover GetPersonaCandidate: %v", err)
	}
	if got.State != persona.PersonaCandidateDismissed {
		t.Fatalf("state = %q, want dismissed", got.State)
	}
	if strings.TrimSpace(got.DraftID) != "" {
		t.Fatalf("DraftID = %q, want empty (force-dismiss must not invent a link)", got.DraftID)
	}
}

func TestRunPersonaCandidatesRecoverLinkRepairsPartialState(t *testing.T) {
	configtest.IsolateHome(t)
	workDir := t.TempDir()
	partialID := forcePartialDraftedCandidateCLI(t, workDir, "major", "economics", "I major in economics")

	// Mint an orphan persona_update draft via the harness so the
	// recover --link target exists. Closed before the CLI runs.
	runtime, err := app.OpenRuntimeWithConfigOptions(workDir, configtest.IsolatedOptions(t))
	if err != nil {
		t.Fatalf("orphan OpenRuntime: %v", err)
	}
	orphanResult, err := runtime.Harness.ProposePersonaUpdate(model.PersonaUpdateProposal{
		Field:         "major",
		ProposedValue: "economics",
		Evidence:      "I major in economics",
		Reason:        "orphan for recover test",
		Confidence:    "high",
		Source:        "console",
		ObservedAt:    time.Date(2026, 5, 20, 10, 30, 0, 0, time.UTC),
	}, time.Date(2026, 5, 20, 10, 35, 0, 0, time.UTC))
	if err != nil {
		runtime.Close()
		t.Fatalf("orphan ProposePersonaUpdate: %v", err)
	}
	if err := runtime.Close(); err != nil {
		t.Fatalf("orphan close runtime: %v", err)
	}

	var stdout, stderr bytes.Buffer
	exit := Run([]string{"persona", "candidates", "recover", "--workdir", workDir, "--link", orphanResult.DraftID, partialID}, &bytes.Buffer{}, &stdout, &stderr, "test")
	if exit != 0 {
		t.Fatalf("exit = %d, stderr=%q", exit, stderr.String())
	}
	out := stdout.String()
	if !strings.Contains(out, "Action: recover --link") {
		t.Fatalf("output missing action label:\n%s", out)
	}
	if !strings.Contains(out, "State:  drafted") {
		t.Fatalf("output missing drafted transition:\n%s", out)
	}
	if !strings.Contains(out, orphanResult.DraftID) {
		t.Fatalf("output missing linked DraftID %q:\n%s", orphanResult.DraftID, out)
	}

	check := openRuntimeForPersonaCLITest(t, workDir)
	got, err := check.GetPersonaCandidate(partialID)
	if err != nil {
		t.Fatalf("post-recover GetPersonaCandidate: %v", err)
	}
	if got.DraftID != orphanResult.DraftID {
		t.Fatalf("candidate DraftID = %q, want %q", got.DraftID, orphanResult.DraftID)
	}
}

func TestRunPersonaCandidatesRecoverRejectsWithoutModeFlag(t *testing.T) {
	configtest.IsolateHome(t)
	workDir := t.TempDir()
	openRuntimeForPersonaCLITest(t, workDir)

	var stdout, stderr bytes.Buffer
	exit := Run([]string{"persona", "candidates", "recover", "--workdir", workDir, "pc-fake"}, &bytes.Buffer{}, &stdout, &stderr, "test")
	if exit == 0 {
		t.Fatalf("expected non-zero exit when neither --link nor --force-dismiss supplied; stdout=%q", stdout.String())
	}
	if !strings.Contains(stderr.String(), "--link") || !strings.Contains(stderr.String(), "--force-dismiss") {
		t.Fatalf("stderr should mention both mode flags: %q", stderr.String())
	}
}

func TestRunPersonaCandidatesRecoverRejectsWithBothFlags(t *testing.T) {
	configtest.IsolateHome(t)
	workDir := t.TempDir()
	openRuntimeForPersonaCLITest(t, workDir)

	var stdout, stderr bytes.Buffer
	exit := Run([]string{"persona", "candidates", "recover", "--workdir", workDir, "--link", "draft-x", "--force-dismiss", "pc-fake"}, &bytes.Buffer{}, &stdout, &stderr, "test")
	if exit == 0 {
		t.Fatalf("expected non-zero exit when both mode flags supplied; stdout=%q", stdout.String())
	}
	if !strings.Contains(stderr.String(), "mutually exclusive") {
		t.Fatalf("stderr should explain exclusivity: %q", stderr.String())
	}
}

func TestRunPersonaCandidatesDraftRetryRejected(t *testing.T) {
	configtest.IsolateHome(t)
	workDir := t.TempDir()
	candidateID, originalDraftID := seedLinkedAndRejectedPersonaCandidateCLI(t, workDir, "major", "economics", "I major in economics")

	var stdout, stderr bytes.Buffer
	exit := Run([]string{"persona", "candidates", "draft", "--workdir", workDir, "--retry-rejected", candidateID}, &bytes.Buffer{}, &stdout, &stderr, "test")
	if exit != 0 {
		t.Fatalf("exit = %d, stderr=%q", exit, stderr.String())
	}
	out := stdout.String()
	if !strings.Contains(out, "Action: draft (retry-rejected)") {
		t.Fatalf("output missing retry action label:\n%s", out)
	}
	newDraftID := extractDraftIDFromOutput(t, out)
	if newDraftID == originalDraftID {
		t.Fatalf("retry returned same DraftID %q as the rejected one", newDraftID)
	}

	runtime := openRuntimeForPersonaCLITest(t, workDir)
	got, err := runtime.GetPersonaCandidate(candidateID)
	if err != nil {
		t.Fatalf("post-retry GetPersonaCandidate: %v", err)
	}
	if got.DraftID != newDraftID {
		t.Fatalf("candidate DraftID = %q, want %q", got.DraftID, newDraftID)
	}
	// Original rejected draft is preserved in store.
	origDraft, err := runtime.Store.Drafts().GetDraft(originalDraftID)
	if err != nil {
		t.Fatalf("GetDraft(original): %v", err)
	}
	if origDraft.State != model.DraftRejected {
		t.Fatalf("original draft state = %q, want rejected", origDraft.State)
	}
}

func TestRunPersonaCandidatesDraftRetryRejectedRefusesPendingReview(t *testing.T) {
	configtest.IsolateHome(t)
	workDir := t.TempDir()
	candidateID := seedPersonaCandidateCLI(t, workDir, "major", "economics", "I major in economics")
	// First draft puts candidate in linked+pending_review. Retry must
	// refuse so the CLI never races the reviewer.
	if exit := Run([]string{"persona", "candidates", "draft", "--workdir", workDir, candidateID}, &bytes.Buffer{}, &bytes.Buffer{}, &bytes.Buffer{}, "test"); exit != 0 {
		t.Fatalf("seed draft exit = %d", exit)
	}

	var stdout, stderr bytes.Buffer
	exit := Run([]string{"persona", "candidates", "draft", "--workdir", workDir, "--retry-rejected", candidateID}, &bytes.Buffer{}, &stdout, &stderr, "test")
	if exit == 0 {
		t.Fatalf("expected non-zero exit for retry-rejected against pending_review draft; stdout=%q", stdout.String())
	}
	if !strings.Contains(stderr.String(), "terminal") && !strings.Contains(stderr.String(), "pending_review") {
		t.Fatalf("stderr should explain non-terminal refusal: %q", stderr.String())
	}
}
