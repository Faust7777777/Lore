package cli

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"sort"
	"strings"
	"time"

	"obsidian-harness/internal/app"
	"obsidian-harness/internal/persona"
)

// runPersonaSummaryCommand prints a one-page dashboard combining
// the answers `lore persona candidates list --state ...` and `lore
// persona errors` would give if the operator ran each separately.
// Reads only; never mutates store or log file. Intended as the
// fastest "what is the health of this workdir's persona pipeline"
// answer for both the operator's manual test cookbook and
// regression triage.
func runPersonaSummaryCommand(args []string, stdout io.Writer, stderr io.Writer) int {
	flags := flag.NewFlagSet("persona summary", flag.ContinueOnError)
	flags.SetOutput(stderr)
	workDirFlag := flags.String("workdir", "", "workdir that contains vault/ and state/")
	failOnOrphan := flags.Bool("fail-on-orphan", false, "exit with code 2 if any candidate is in the partial-orphan shape (Drafted with empty DraftID)")
	asJSON := flags.Bool("json", false, "emit the dashboard as a single-line JSON object instead of the human-formatted block")
	args = reorderFlagsBeforePositionals(args, flags)
	if err := flags.Parse(args); err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}
	workDir, err := defaultWorkDir(*workDirFlag)
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}
	runtime, err := app.OpenRuntime(workDir)
	if err != nil {
		fmt.Fprintf(stderr, "open runtime: %v\n", err)
		return 1
	}
	defer closeRuntime(stderr, runtime, "persona summary")

	openList, err := runtime.ListPersonaCandidates(persona.PersonaCandidateOpen, 0)
	if err != nil {
		fmt.Fprintf(stderr, "persona summary: list open: %v\n", err)
		return 1
	}
	draftedList, err := runtime.ListPersonaCandidates(persona.PersonaCandidateDrafted, 0)
	if err != nil {
		fmt.Fprintf(stderr, "persona summary: list drafted: %v\n", err)
		return 1
	}
	dismissedList, err := runtime.ListPersonaCandidates(persona.PersonaCandidateDismissed, 0)
	if err != nil {
		fmt.Fprintf(stderr, "persona summary: list dismissed: %v\n", err)
		return 1
	}
	draftedLinked := 0
	draftedOrphan := 0
	for _, r := range draftedList {
		if strings.TrimSpace(r.DraftID) == "" {
			draftedOrphan++
		} else {
			draftedLinked++
		}
	}

	if *asJSON {
		if err := emitPersonaSummaryJSON(stdout, workDir, runtime.PersonaExtractLogPath(),
			len(openList), draftedLinked, draftedOrphan, len(dismissedList)); err != nil {
			fmt.Fprintf(stderr, "persona summary: emit json: %v\n", err)
			return 1
		}
	} else {
		renderPersonaSummary(stdout, workDir, runtime.PersonaExtractLogPath(),
			len(openList), draftedLinked, draftedOrphan, len(dismissedList))
	}

	// Health-check exit code. Distinct from 0 (success) and 1
	// (command error) so CI / nagios-style consumers can tell a
	// real failure from an actionable warning. The check runs
	// AFTER the dashboard prints so the operator always sees the
	// numbers even when the exit code signals a problem.
	if *failOnOrphan && draftedOrphan > 0 {
		fmt.Fprintln(stderr)
		fmt.Fprintf(stderr, "persona summary: --fail-on-orphan tripped (%d partial-orphan candidate(s) present)\n", draftedOrphan)
		fmt.Fprintln(stderr, "  hint: enumerate the orphans via:")
		fmt.Fprintln(stderr, "    lore persona candidates list --workdir <wd> --state drafted")
		fmt.Fprintln(stderr, "  then reconcile each with either:")
		fmt.Fprintln(stderr, "    lore persona candidates recover --link <draft-id> <candidate>")
		fmt.Fprintln(stderr, "    lore persona candidates recover --force-dismiss <candidate>")
		return 2
	}

	return 0
}

// emitPersonaSummaryJSON writes the dashboard as a single-line
// JSON object instead of the human-formatted block. Stable shape
// so monitoring / alerting consumers can rely on it:
//
//	{
//	  "workdir": "...",
//	  "log_path": "...",
//	  "candidates": {"open": N, "drafted_linked": M, "drafted_orphan": K, "dismissed": N},
//	  "extract_log": {
//	    "exists": true,
//	    "total_entries": N,
//	    "by_stage": {"extract": N, "store": N, "parse_warning": N, "malformed": N},
//	    "last_entry": "2026-..."  // empty string if no parsed entries
//	  }
//	}
//
// JSON is emitted to stdout regardless of --fail-on-orphan exit
// signal; the check tripped block still lands on stderr so a
// script can pipe stdout to jq while branching on the exit code.
func emitPersonaSummaryJSON(stdout io.Writer, workDir, logPath string, openN, draftedLinked, draftedOrphan, dismissedN int) error {
	type extractLog struct {
		Exists       bool           `json:"exists"`
		TotalEntries int            `json:"total_entries"`
		ByStage      map[string]int `json:"by_stage"`
		LastEntry    string         `json:"last_entry"`
	}
	type payload struct {
		Workdir    string         `json:"workdir"`
		LogPath    string         `json:"log_path"`
		Candidates map[string]int `json:"candidates"`
		ExtractLog extractLog     `json:"extract_log"`
	}

	out := payload{
		Workdir: workDir,
		LogPath: logPath,
		Candidates: map[string]int{
			"open":           openN,
			"drafted_linked": draftedLinked,
			"drafted_orphan": draftedOrphan,
			"dismissed":      dismissedN,
		},
		ExtractLog: extractLog{ByStage: map[string]int{}},
	}

	all, exists, err := readPersonaLogEntries(logPath)
	if err != nil {
		return err
	}
	out.ExtractLog.Exists = exists
	if exists {
		var latest time.Time
		for _, entry := range all {
			out.ExtractLog.TotalEntries++
			if entry.parsed {
				out.ExtractLog.ByStage[entry.stage]++
				if entry.ts.After(latest) {
					latest = entry.ts
				}
			} else {
				out.ExtractLog.ByStage["malformed"]++
			}
		}
		if !latest.IsZero() {
			out.ExtractLog.LastEntry = latest.UTC().Format(time.RFC3339Nano)
		}
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

// renderPersonaSummary writes the dashboard block. Split out so a
// pure unit test can drive it with synthetic counts + a seeded log
// file without OpenRuntime.
func renderPersonaSummary(stdout io.Writer, workDir, logPath string, openN, draftedLinked, draftedOrphan, dismissedN int) {
	fmt.Fprintln(stdout, "Persona Memory Summary")
	fmt.Fprintln(stdout, "======================")
	fmt.Fprintf(stdout, "Workdir: %s\n", workDir)
	fmt.Fprintln(stdout)
	fmt.Fprintln(stdout, "Candidates:")
	fmt.Fprintf(stdout, "  %-12s %d\n", "open", openN)
	fmt.Fprintf(stdout, "  %-12s %d (linked %d, partial-orphan %d)\n", "drafted", draftedLinked+draftedOrphan, draftedLinked, draftedOrphan)
	fmt.Fprintf(stdout, "  %-12s %d\n", "dismissed", dismissedN)

	// Extract log section: read counts by stage, plus the timestamp
	// of the most recent entry as a freshness signal. Missing log
	// file is rendered as a single "—" line rather than a stat error
	// because a fresh workdir that has never seen a failure is a
	// valid healthy state.
	fmt.Fprintln(stdout)
	fmt.Fprintln(stdout, "Extract log:")
	fmt.Fprintf(stdout, "  source: %s\n", logPath)

	all, exists, err := readPersonaLogEntries(logPath)
	if err != nil {
		fmt.Fprintf(stdout, "  (read error: %v)\n", err)
		return
	}
	if !exists {
		fmt.Fprintln(stdout, "  (no log file yet)")
		return
	}
	if len(all) == 0 {
		fmt.Fprintln(stdout, "  (empty)")
		return
	}

	totals := map[string]int{}
	var latest time.Time
	for _, entry := range all {
		if entry.parsed {
			totals[entry.stage]++
			if entry.ts.After(latest) {
				latest = entry.ts
			}
		} else {
			totals["(malformed)"]++
		}
	}
	fmt.Fprintf(stdout, "  total entries: %d\n", len(all))
	if len(totals) > 0 {
		fmt.Fprintln(stdout, "  by stage:")
		stages := make([]string, 0, len(totals))
		for s := range totals {
			stages = append(stages, s)
		}
		sort.Strings(stages)
		for _, s := range stages {
			fmt.Fprintf(stdout, "    %-16s %d\n", s, totals[s])
		}
	}
	if !latest.IsZero() {
		fmt.Fprintf(stdout, "  last entry: %s\n", latest.UTC().Format(time.RFC3339))
	} else {
		fmt.Fprintln(stdout, "  last entry: —")
	}
}
