package cli

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"strconv"
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

// personaLogEntry is the parsed shape of a single persona-extract.log
// line. Lines that fail to parse (malformed, truncated) are still
// retained with parsed=false so the operator sees the raw line; the
// stage/since filters skip them rather than crashing.
//
// session and error are extracted up-front from the third and fourth
// tab-delimited fields so JSON emit does not have to re-split raw.
// error is unquoted via unquoteSafe because session.go's
// logPersonaExtractError formats it as %q.
//
// model and baseURL are optional fifth/sixth columns introduced by
// the B-line model-consistency slice. Absent on legacy lines (those
// stay parsed=true with empty values); the JSON emitter omits empty
// strings via the omitempty tag below.
type personaLogEntry struct {
	raw     string
	ts      time.Time
	stage   string
	session string
	errMsg  string
	model   string
	baseURL string
	parsed  bool
}

func parsePersonaLogLine(raw string) personaLogEntry {
	fields := strings.Split(raw, "\t")
	if len(fields) < 4 {
		return personaLogEntry{raw: raw}
	}
	ts, err := time.Parse(time.RFC3339Nano, fields[0])
	if err != nil {
		return personaLogEntry{raw: raw}
	}
	if !strings.HasPrefix(fields[1], "stage=") {
		return personaLogEntry{raw: raw, ts: ts}
	}
	entry := personaLogEntry{
		raw:     raw,
		ts:      ts,
		stage:   strings.TrimPrefix(fields[1], "stage="),
		session: strings.TrimPrefix(fields[2], "session="),
		errMsg:  unquoteSafe(strings.TrimPrefix(fields[3], "error=")),
		parsed:  true,
	}
	for _, extra := range fields[4:] {
		switch {
		case strings.HasPrefix(extra, "model="):
			entry.model = strings.TrimPrefix(extra, "model=")
		case strings.HasPrefix(extra, "base_url="):
			entry.baseURL = unquoteSafe(strings.TrimPrefix(extra, "base_url="))
		}
	}
	return entry
}

// readPersonaLogEntries loads logPath and returns one parsed entry per
// non-blank line. The boolean indicates whether the file existed --
// callers (summary / errors / JSON variants) all want to distinguish
// "no file yet" from "read error" but render that signal differently,
// so the helper surfaces both rather than coercing a missing file
// into an empty slice. Trailing newline is trimmed before splitting
// so the empty tail line does not produce a malformed entry.
func readPersonaLogEntries(logPath string) (entries []personaLogEntry, exists bool, err error) {
	data, err := os.ReadFile(logPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, false, nil
		}
		return nil, false, err
	}
	for _, raw := range strings.Split(strings.TrimRight(string(data), "\n"), "\n") {
		if strings.TrimSpace(raw) == "" {
			continue
		}
		entries = append(entries, parsePersonaLogLine(raw))
	}
	return entries, true, nil
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

	out := payload{
		LogPath: logPath,
		Filter: filterOut{
			Tail:         tail,
			Stage:        stageFilter,
			SinceSeconds: int64(since.Seconds()),
		},
		Entries: []entryOut{},
	}

	all, _, err := readPersonaLogEntries(logPath)
	if err != nil {
		fmt.Fprintf(stderr, "persona errors: read log: %v\n", err)
		return 1
	}
	if all == nil {
		// File missing or empty: emit the empty-payload skeleton so
		// the JSON shape stays uniform and downstream consumers can
		// branch on summary.total_entries == 0 rather than special-
		// casing a different top-level shape.
		encoded, mErr := json.Marshal(out)
		if mErr != nil {
			fmt.Fprintf(stderr, "persona errors: emit json: %v\n", mErr)
			return 1
		}
		_, _ = stdout.Write(encoded)
		_, _ = fmt.Fprintln(stdout)
		return 0
	}

	var cutoff time.Time
	if since > 0 {
		cutoff = now.Add(-since)
	}

	filtered := make([]personaLogEntry, 0, len(all))
	for _, e := range all {
		if stageFilter != "" && (!e.parsed || e.stage != stageFilter) {
			continue
		}
		if since > 0 {
			if e.ts.IsZero() || e.ts.Before(cutoff) {
				continue
			}
		}
		filtered = append(filtered, e)
	}
	shown := filtered
	if tail > 0 && len(filtered) > tail {
		shown = filtered[len(filtered)-tail:]
	}

	out.Summary.TotalEntries = len(all)
	out.Summary.Shown = len(shown)
	for _, e := range shown {
		entry := entryOut{Raw: e.raw}
		if e.parsed {
			entry.Timestamp = e.ts.UTC().Format(time.RFC3339Nano)
			entry.Stage = e.stage
			entry.Session = e.session
			entry.Error = e.errMsg
			entry.Model = e.model
			entry.BaseURL = e.baseURL
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

// unquoteSafe applies strconv.Unquote when the input is a quoted
// string, otherwise returns the input verbatim. Used because the
// log writer formats errors as %q which produces a quoted string,
// and JSON consumers want the unquoted value.
func unquoteSafe(s string) string {
	if len(s) >= 2 && s[0] == '"' && s[len(s)-1] == '"' {
		if unquoted, err := strconv.Unquote(s); err == nil {
			return unquoted
		}
	}
	return s
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
	all, exists, err := readPersonaLogEntries(logPath)
	if err != nil {
		fmt.Fprintf(stderr, "persona errors: read log: %v\n", err)
		return 1
	}
	if !exists {
		header()
		fmt.Fprintln(stdout)
		fmt.Fprintln(stdout, "No log file yet. Either extraction has not produced any failures in this workdir, or the runtime has not been opened here.")
		return 0
	}

	var cutoff time.Time
	if since > 0 {
		cutoff = now.Add(-since)
	}

	filtered := make([]personaLogEntry, 0, len(all))
	for _, e := range all {
		if stageFilter != "" && (!e.parsed || e.stage != stageFilter) {
			continue
		}
		if since > 0 {
			if e.ts.IsZero() || e.ts.Before(cutoff) {
				continue
			}
		}
		filtered = append(filtered, e)
	}

	shown := filtered
	if tail > 0 && len(filtered) > tail {
		shown = filtered[len(filtered)-tail:]
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

	if len(shown) == 0 {
		fmt.Fprintln(stdout, "No matching entries.")
		return 0
	}

	for _, e := range shown {
		fmt.Fprintln(stdout, e.raw)
	}
	fmt.Fprintln(stdout)
	if len(filtered) != len(all) {
		fmt.Fprintf(stdout, "%d of %d entries shown (filtered from %d total).\n", len(shown), len(filtered), len(all))
	} else {
		fmt.Fprintf(stdout, "%d of %d entries shown.\n", len(shown), len(all))
	}
	return 0
}
