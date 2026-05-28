package cli

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"sort"
	"time"

	"obsidian-harness/internal/app"
)

// runPersonaSummaryCommand prints a one-page dashboard combining the
// answers `lore persona candidates list --state ...` and `lore
// persona errors` would give if the operator ran each separately.
// Reads only; never mutates store or log file. Intended as the
// fastest "what is the health of this workdir's persona pipeline"
// answer for both the operator's manual test cookbook and
// regression triage.
//
// All aggregation lives in app.Runtime.PersonaSummary; this command
// stays in cli only to glue flag parsing + rendering / JSON emit
// onto the lifted DTO.
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

	view, err := runtime.PersonaSummary()
	if err != nil {
		fmt.Fprintf(stderr, "persona summary: %v\n", err)
		return 1
	}

	if *asJSON {
		if err := emitPersonaSummaryJSON(stdout, view); err != nil {
			fmt.Fprintf(stderr, "persona summary: emit json: %v\n", err)
			return 1
		}
	} else {
		renderPersonaSummary(stdout, view)
	}

	// Health-check exit code. Distinct from 0 (success) and 1
	// (command error) so CI / nagios-style consumers can tell a
	// real failure from an actionable warning. The check runs
	// AFTER the dashboard prints so the operator always sees the
	// numbers even when the exit code signals a problem.
	if *failOnOrphan && view.Candidates.DraftedOrphan > 0 {
		fmt.Fprintln(stderr)
		fmt.Fprintf(stderr, "persona summary: --fail-on-orphan tripped (%d partial-orphan candidate(s) present)\n", view.Candidates.DraftedOrphan)
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
// JSON object. Stable shape so monitoring / alerting consumers
// can rely on it:
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
func emitPersonaSummaryJSON(stdout io.Writer, view app.PersonaSummaryView) error {
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
		Workdir: view.Workdir,
		LogPath: view.LogPath,
		Candidates: map[string]int{
			"open":           view.Candidates.Open,
			"drafted_linked": view.Candidates.DraftedLinked,
			"drafted_orphan": view.Candidates.DraftedOrphan,
			"dismissed":      view.Candidates.Dismissed,
		},
		ExtractLog: extractLog{
			Exists:       view.ExtractLog.Exists,
			TotalEntries: view.ExtractLog.TotalEntries,
			ByStage:      view.ExtractLog.ByStage,
		},
	}
	if out.ExtractLog.ByStage == nil {
		out.ExtractLog.ByStage = map[string]int{}
	}
	if !view.ExtractLog.LastEntry.IsZero() {
		out.ExtractLog.LastEntry = view.ExtractLog.LastEntry.UTC().Format(time.RFC3339Nano)
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
// pure unit test can drive it with a synthetic PersonaSummaryView
// without spinning up a runtime.
func renderPersonaSummary(stdout io.Writer, view app.PersonaSummaryView) {
	fmt.Fprintln(stdout, "Persona Memory Summary")
	fmt.Fprintln(stdout, "======================")
	fmt.Fprintf(stdout, "Workdir: %s\n", view.Workdir)
	fmt.Fprintln(stdout)
	fmt.Fprintln(stdout, "Candidates:")
	fmt.Fprintf(stdout, "  %-12s %d\n", "open", view.Candidates.Open)
	fmt.Fprintf(stdout, "  %-12s %d (linked %d, partial-orphan %d)\n",
		"drafted",
		view.Candidates.DraftedLinked+view.Candidates.DraftedOrphan,
		view.Candidates.DraftedLinked,
		view.Candidates.DraftedOrphan,
	)
	fmt.Fprintf(stdout, "  %-12s %d\n", "dismissed", view.Candidates.Dismissed)

	// Extract log section: counts by stage + freshness signal.
	// Missing log file is rendered as a single placeholder rather
	// than a stat error because a fresh workdir that has never seen
	// a failure is a valid healthy state. The "(no log file yet)"
	// vs. "(empty)" split lets operators distinguish "extraction
	// has never been wired" from "extraction runs, nothing failed".
	fmt.Fprintln(stdout)
	fmt.Fprintln(stdout, "Extract log:")
	fmt.Fprintf(stdout, "  source: %s\n", view.LogPath)
	if !view.ExtractLog.Exists {
		fmt.Fprintln(stdout, "  (no log file yet)")
		return
	}
	if view.ExtractLog.TotalEntries == 0 {
		fmt.Fprintln(stdout, "  (empty)")
		return
	}
	fmt.Fprintf(stdout, "  total entries: %d\n", view.ExtractLog.TotalEntries)
	if len(view.ExtractLog.ByStage) > 0 {
		fmt.Fprintln(stdout, "  by stage:")
		stages := make([]string, 0, len(view.ExtractLog.ByStage))
		for s := range view.ExtractLog.ByStage {
			stages = append(stages, s)
		}
		sort.Strings(stages)
		for _, s := range stages {
			fmt.Fprintf(stdout, "    %-16s %d\n", s, view.ExtractLog.ByStage[s])
		}
	}
	if !view.ExtractLog.LastEntry.IsZero() {
		fmt.Fprintf(stdout, "  last entry: %s\n", view.ExtractLog.LastEntry.UTC().Format(time.RFC3339))
	} else {
		fmt.Fprintln(stdout, "  last entry: —")
	}
}
