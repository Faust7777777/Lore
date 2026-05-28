package app

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

// PersonaExtractErrorEntry is the structured shape of one line in
// the persona-extract.log file. The writer lives in
// internal/console/session.go (Session.logPersonaExtractError) and
// emits the canonical TSV record:
//
//	<RFC3339Nano UTC>\tstage=<extract|store|parse_warning>\tsession=<id>\terror=<%q>[\tmodel=<plain>\tbase_url=<%q>]
//
// Parsed=false marks lines that did not conform (truncated writes,
// future schema additions). Such lines preserve Raw so an operator
// can still see them; the canonical fields stay zero-value and any
// stage/since filtering skips them rather than crashing.
type PersonaExtractErrorEntry struct {
	Raw       string
	Parsed    bool
	Timestamp time.Time
	Stage     string
	Session   string
	Error     string
	Model     string
	BaseURL   string
}

// PersonaErrorsFilter parameterises ListPersonaExtractErrors.
// Zero-value Stage means "all stages"; zero Since means "no time
// filter"; zero Tail means "no limit". Now is parameter-injected so
// callers / tests can pin a stable reference for Since-based
// filtering against fixture timestamps.
type PersonaErrorsFilter struct {
	Stage string
	Since time.Duration
	Tail  int
	Now   time.Time
}

// PersonaExtractErrorsResult bundles the filtered entry slice with
// the metadata operators need to render dashboards: log path,
// existence flag, and the raw (pre-filter) entry count. Consumers
// can build either tail-style text rendering or JSON shape from
// this without re-reading the file.
type PersonaExtractErrorsResult struct {
	LogPath      string
	LogExists    bool
	TotalEntries int
	Entries      []PersonaExtractErrorEntry
}

// ListPersonaExtractErrors reads <StateDir>/logs/persona-extract.log,
// parses each line, applies the filter, and returns the matching
// entries plus enough metadata for the consumer to render a
// dashboard. A missing file is NOT an error: LogExists=false with an
// empty Entries slice lets the caller render a "no log yet" message
// without branching on err.
//
// Filter semantics mirror the existing `lore persona errors` flags:
//
//   - Stage filter drops lines whose parsed stage does not match.
//     Unparsed (malformed) lines are dropped under stage filters.
//   - Since filter drops lines whose timestamp is older than Now - Since.
//     Zero timestamps (unparsed lines) are dropped under since filters.
//   - Tail keeps the most recent N entries after the above filters.
//
// TotalEntries reflects the raw line count before any filter is
// applied so dashboards can show "X of Y entries shown".
func (r *Runtime) ListPersonaExtractErrors(filter PersonaErrorsFilter) (PersonaExtractErrorsResult, error) {
	if r == nil {
		return PersonaExtractErrorsResult{}, fmt.Errorf("app: runtime is not initialized")
	}
	logPath := r.PersonaExtractLogPath()
	result := PersonaExtractErrorsResult{LogPath: logPath, Entries: []PersonaExtractErrorEntry{}}

	all, exists, err := readPersonaLogFile(logPath)
	if err != nil {
		return result, err
	}
	result.LogExists = exists
	result.TotalEntries = len(all)
	if !exists || len(all) == 0 {
		return result, nil
	}

	now := filter.Now
	if now.IsZero() {
		now = time.Now().UTC()
	}
	var cutoff time.Time
	if filter.Since > 0 {
		cutoff = now.Add(-filter.Since)
	}

	filtered := make([]PersonaExtractErrorEntry, 0, len(all))
	for _, entry := range all {
		if filter.Stage != "" && (!entry.Parsed || entry.Stage != filter.Stage) {
			continue
		}
		if filter.Since > 0 {
			if entry.Timestamp.IsZero() || entry.Timestamp.Before(cutoff) {
				continue
			}
		}
		filtered = append(filtered, entry)
	}
	if filter.Tail > 0 && len(filtered) > filter.Tail {
		filtered = filtered[len(filtered)-filter.Tail:]
	}
	result.Entries = filtered
	return result, nil
}

// readPersonaLogFile loads logPath and returns one parsed entry per
// non-blank line. The exists return distinguishes "no file yet" from
// "read error" so callers (CLI errors / summary / TUI panel) can
// render the friendly empty state without branching on os.IsNotExist.
// Trailing newline is trimmed before splitting so the empty tail line
// does not produce a malformed entry.
func readPersonaLogFile(logPath string) (entries []PersonaExtractErrorEntry, exists bool, err error) {
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
		entries = append(entries, parsePersonaExtractLogLine(raw))
	}
	return entries, true, nil
}

// parsePersonaExtractLogLine decodes one TSV record. The format
// contract is documented on PersonaExtractErrorEntry and pinned by
// tests in both internal/console (writer) and internal/cli
// (round-trip). Legacy four-column lines (no model/base_url tail)
// parse with empty Model / BaseURL.
func parsePersonaExtractLogLine(raw string) PersonaExtractErrorEntry {
	fields := strings.Split(raw, "\t")
	if len(fields) < 4 {
		return PersonaExtractErrorEntry{Raw: raw}
	}
	ts, err := time.Parse(time.RFC3339Nano, fields[0])
	if err != nil {
		return PersonaExtractErrorEntry{Raw: raw}
	}
	if !strings.HasPrefix(fields[1], "stage=") {
		return PersonaExtractErrorEntry{Raw: raw, Timestamp: ts}
	}
	entry := PersonaExtractErrorEntry{
		Raw:       raw,
		Parsed:    true,
		Timestamp: ts,
		Stage:     strings.TrimPrefix(fields[1], "stage="),
		Session:   strings.TrimPrefix(fields[2], "session="),
		Error:     unquoteSafePersonaField(strings.TrimPrefix(fields[3], "error=")),
	}
	for _, extra := range fields[4:] {
		switch {
		case strings.HasPrefix(extra, "model="):
			entry.Model = strings.TrimPrefix(extra, "model=")
		case strings.HasPrefix(extra, "base_url="):
			entry.BaseURL = unquoteSafePersonaField(strings.TrimPrefix(extra, "base_url="))
		}
	}
	return entry
}

// unquoteSafePersonaField applies strconv.Unquote when the input is a
// quoted string, otherwise returns the input verbatim. The writer
// formats error / base_url via %q which produces a quoted form;
// consumers (JSON emit, TUI display) want the unquoted value.
func unquoteSafePersonaField(s string) string {
	if len(s) >= 2 && s[0] == '"' && s[len(s)-1] == '"' {
		if unquoted, err := strconv.Unquote(s); err == nil {
			return unquoted
		}
	}
	return s
}
