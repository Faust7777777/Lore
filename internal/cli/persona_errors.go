package cli

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"strings"
	"time"

	"obsidian-harness/internal/app"
)

func runPersonaErrorsCommand(args []string, stdout io.Writer, stderr io.Writer) int {
	workDir, tail, stageFilter, since, asJSON, err := parsePersonaErrorsFlags(args, stderr)
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}
	runtime, err := app.OpenRuntime(workDir)
	if err != nil {
		fmt.Fprintf(stderr, "open runtime: %v\n", err)
		return 1
	}
	defer closeRuntime(stderr, runtime, "persona errors")
	if asJSON {
		return emitPersonaExtractErrorsJSON(stdout, stderr, runtime.PersonaExtractLogPath(), tail, stageFilter, since, time.Now().UTC())
	}
	return renderPersonaExtractErrors(stdout, stderr, runtime.PersonaExtractLogPath(), tail, stageFilter, since, time.Now().UTC())
}

func parsePersonaErrorsFlags(args []string, stderr io.Writer) (workDir string, tail int, stage string, since time.Duration, asJSON bool, err error) {
	flags := flag.NewFlagSet("persona errors", flag.ContinueOnError)
	flags.SetOutput(stderr)
	workDirFlag := flags.String("workdir", "", "workdir that contains vault/ and state/")
	tailFlag := flags.Int("tail", 20, "show only the last N matching lines (0 = no limit)")
	stageFlag := flags.String("stage", "", "filter by stage: extract, store, parse_warning (empty = all)")
	sinceFlag := flags.Duration("since", 0, "only show lines newer than this duration (e.g. 1h, 30m; 0 = no time filter)")
	jsonFlag := flags.Bool("json", false, "emit filtered entries as a JSON object instead of the tab-delimited log lines")
	args = reorderFlagsBeforePositionals(args, flags)
	if parseErr := flags.Parse(args); parseErr != nil {
		err = parseErr
		return
	}
	if *tailFlag < 0 {
		err = fmt.Errorf("persona errors: --tail must be >= 0")
		return
	}
	if s := strings.TrimSpace(*stageFlag); s != "" {
		switch s {
		case "extract", "store", "parse_warning":
		default:
			err = fmt.Errorf("persona errors: --stage must be one of extract, store, parse_warning")
			return
		}
	}
	if *sinceFlag < 0 {
		err = fmt.Errorf("persona errors: --since must be >= 0")
		return
	}
	resolved, resolveErr := defaultWorkDir(*workDirFlag)
	if resolveErr != nil {
		err = resolveErr
		return
	}
	workDir = resolved
	tail = *tailFlag
	stage = strings.TrimSpace(*stageFlag)
	since = *sinceFlag
	asJSON = *jsonFlag
	return
}

// emitPersonaExtractErrorsJSON is the --json counterpart of
// renderPersonaExtractErrors. Filter semantics (stage, since, tail)
// are identical; the difference is the output shape:
//
//	{
//	  "log_path": "...",
//	  "filter": {"tail": N, "stage": "...", "since_seconds": N},
//	  "summary": {"total_entries": N, "shown": N},
//	  "entries": [
//	    {"timestamp": "RFC3339", "stage": "...", "session": "...", "error": "...", "raw": "..."},
//	    ...
//	  ]
//	}
//
// Lines that fail to parse appear with the canonical fields blank
// and the verbatim raw line preserved so a downstream consumer
// can still see them. Missing log file degrades to a single-shot
// summary block with entries=[] and total_entries=0; exit code
// stays 0.
func emitPersonaExtractErrorsJSON(stdout, stderr io.Writer, logPath string, tail int, stageFilter string, since time.Duration, now time.Time) int {
	type entryOut struct {
		Timestamp string `json:"timestamp"`
		Stage     string `json:"stage"`
		Session   string `json:"session"`
		Error     string `json:"error"`
		Model     string `json:"model,omitempty"`
		BaseURL   string `json:"base_url,omitempty"`
		Raw       string `json:"raw"`
	}
	type filterOut struct {
		Tail         int    `json:"tail"`
		Stage        string `json:"stage"`
		SinceSeconds int64  `json:"since_seconds"`
	}
	type summaryOut struct {
		TotalEntries int `json:"total_entries"`
		Shown        int `json:"shown"`
	}
	type payload struct {
		LogPath string     `json:"log_path"`
		Filter  filterOut  `json:"filter"`
		Summary summaryOut `json:"summary"`
		Entries []entryOut `json:"entries"`
	}

	result, err := app.ReadPersonaExtractErrors(logPath, app.PersonaErrorsFilter{
		Tail: tail, Stage: stageFilter, Since: since, Now: now,
	})
	if err != nil {
		fmt.Fprintf(stderr, "persona errors: read log: %v\n", err)
		return 1
	}

	out := payload{
		LogPath: logPath,
		Filter: filterOut{
			Tail:         tail,
			Stage:        stageFilter,
			SinceSeconds: int64(since.Seconds()),
		},
		Summary: summaryOut{
			TotalEntries: result.TotalEntries,
			Shown:        len(result.Entries),
		},
		Entries: make([]entryOut, 0, len(result.Entries)),
	}
	for _, e := range result.Entries {
		entry := entryOut{Raw: e.Raw}
		if e.Parsed {
			entry.Timestamp = e.Timestamp.UTC().Format(time.RFC3339Nano)
			entry.Stage = e.Stage
			entry.Session = e.Session
			entry.Error = e.Error
			entry.Model = e.Model
			entry.BaseURL = e.BaseURL
		}
		out.Entries = append(out.Entries, entry)
	}

	encoded, err := json.Marshal(out)
	if err != nil {
		fmt.Fprintf(stderr, "persona errors: emit json: %v\n", err)
		return 1
	}
	_, _ = stdout.Write(encoded)
	_, _ = fmt.Fprintln(stdout)
	return 0
}

// renderPersonaExtractErrors reads the persona-extract.log file at
// logPath, applies the requested filters, and prints a tail-style
// report to stdout. now is parameter-injected so tests can pin a
// stable "now" against the file's RFC3339Nano timestamps when
// exercising the --since filter.
func renderPersonaExtractErrors(stdout, stderr io.Writer, logPath string, tail int, stageFilter string, since time.Duration, now time.Time) int {
	header := func() {
		fmt.Fprintln(stdout, "Persona Extraction Errors")
		fmt.Fprintln(stdout, "=========================")
		fmt.Fprintf(stdout, "Source: %s\n", logPath)
	}
	result, err := app.ReadPersonaExtractErrors(logPath, app.PersonaErrorsFilter{
		Tail: tail, Stage: stageFilter, Since: since, Now: now,
	})
	if err != nil {
		fmt.Fprintf(stderr, "persona errors: read log: %v\n", err)
		return 1
	}
	if !result.LogExists {
		header()
		fmt.Fprintln(stdout)
		fmt.Fprintln(stdout, "No log file yet. Either extraction has not produced any failures in this workdir, or the runtime has not been opened here.")
		return 0
	}

	header()
	var filterParts []string
	if tail > 0 {
		filterParts = append(filterParts, fmt.Sprintf("tail=%d", tail))
	}
	if stageFilter != "" {
		filterParts = append(filterParts, fmt.Sprintf("stage=%s", stageFilter))
	}
	if since > 0 {
		filterParts = append(filterParts, fmt.Sprintf("since=%s", since))
	}
	if len(filterParts) > 0 {
		fmt.Fprintf(stdout, "Filter: %s\n", strings.Join(filterParts, ", "))
	}
	fmt.Fprintln(stdout)

	if len(result.Entries) == 0 {
		fmt.Fprintln(stdout, "No matching entries.")
		return 0
	}

	for _, e := range result.Entries {
		fmt.Fprintln(stdout, e.Raw)
	}
	fmt.Fprintln(stdout)
	if result.FilteredCount != result.TotalEntries {
		fmt.Fprintf(stdout, "%d of %d entries shown (filtered from %d total).\n",
			len(result.Entries), result.FilteredCount, result.TotalEntries)
	} else {
		fmt.Fprintf(stdout, "%d of %d entries shown.\n", len(result.Entries), result.TotalEntries)
	}
	return 0
}
