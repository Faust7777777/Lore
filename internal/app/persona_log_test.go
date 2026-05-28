package app

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"obsidian-harness/internal/config/configtest"
)

// seedPersonaLogFile writes lines verbatim (one per slice element,
// newline-joined) into a fresh runtime's persona-extract.log so the
// reader tests below can drive ListPersonaExtractErrors against a
// known fixture. Returns the runtime so the test can call methods
// against it; the runtime is registered for Close via t.Cleanup.
func seedPersonaLogFile(t *testing.T, lines []string) *Runtime {
	t.Helper()
	configtest.IsolateHome(t)
	workDir := t.TempDir()
	rt, err := OpenRuntimeWithConfigOptions(workDir, configtest.IsolatedOptions(t))
	if err != nil {
		t.Fatalf("OpenRuntime: %v", err)
	}
	t.Cleanup(func() { _ = rt.Close() })

	logPath := rt.PersonaExtractLogPath()
	if logPath == "" {
		t.Fatal("PersonaExtractLogPath() empty")
	}
	if err := os.MkdirAll(filepath.Dir(logPath), 0o755); err != nil {
		t.Fatalf("mkdir log dir: %v", err)
	}
	content := ""
	if len(lines) > 0 {
		content = ""
		for i, line := range lines {
			if i > 0 {
				content += "\n"
			}
			content += line
		}
		content += "\n"
	}
	if err := os.WriteFile(logPath, []byte(content), 0o644); err != nil {
		t.Fatalf("seed log: %v", err)
	}
	return rt
}

func TestListPersonaExtractErrorsReturnsAllEntriesByDefault(t *testing.T) {
	rt := seedPersonaLogFile(t, []string{
		"2026-05-28T10:00:00Z\tstage=extract\tsession=s1\terror=\"timeout\"",
		"2026-05-28T10:01:00Z\tstage=store\tsession=s2\terror=\"disk full\"",
		"2026-05-28T10:02:00Z\tstage=parse_warning\tsession=s3\terror=\"low confidence\"",
	})

	got, err := rt.ListPersonaExtractErrors(PersonaErrorsFilter{
		Now: time.Date(2026, 5, 28, 10, 30, 0, 0, time.UTC),
	})
	if err != nil {
		t.Fatalf("ListPersonaExtractErrors: %v", err)
	}
	if !got.LogExists {
		t.Fatalf("LogExists = false; want true")
	}
	if got.TotalEntries != 3 || len(got.Entries) != 3 {
		t.Fatalf("counts = total %d shown %d, want 3/3", got.TotalEntries, len(got.Entries))
	}
	if got.Entries[0].Stage != "extract" || got.Entries[0].Session != "s1" || got.Entries[0].Error != "timeout" {
		t.Fatalf("entries[0] = %+v", got.Entries[0])
	}
	if got.LogPath == "" {
		t.Fatal("LogPath empty; want PersonaExtractLogPath()")
	}
}

func TestListPersonaExtractErrorsAppliesStageFilter(t *testing.T) {
	rt := seedPersonaLogFile(t, []string{
		"2026-05-28T10:00:00Z\tstage=extract\tsession=s1\terror=\"e\"",
		"2026-05-28T10:01:00Z\tstage=store\tsession=s2\terror=\"e\"",
		"2026-05-28T10:02:00Z\tstage=parse_warning\tsession=s3\terror=\"e\"",
	})

	got, err := rt.ListPersonaExtractErrors(PersonaErrorsFilter{
		Stage: "parse_warning",
		Now:   time.Date(2026, 5, 28, 10, 30, 0, 0, time.UTC),
	})
	if err != nil {
		t.Fatalf("ListPersonaExtractErrors: %v", err)
	}
	if got.TotalEntries != 3 {
		t.Fatalf("TotalEntries = %d, want 3 (raw count, pre-filter)", got.TotalEntries)
	}
	if len(got.Entries) != 1 || got.Entries[0].Stage != "parse_warning" {
		t.Fatalf("entries = %+v, want exactly the parse_warning row", got.Entries)
	}
}

func TestListPersonaExtractErrorsAppliesSinceFilter(t *testing.T) {
	rt := seedPersonaLogFile(t, []string{
		"2026-05-28T08:00:00Z\tstage=extract\tsession=old\terror=\"e\"",
		"2026-05-28T10:20:00Z\tstage=extract\tsession=fresh\terror=\"e\"",
	})

	got, err := rt.ListPersonaExtractErrors(PersonaErrorsFilter{
		Since: 30 * time.Minute,
		Now:   time.Date(2026, 5, 28, 10, 30, 0, 0, time.UTC),
	})
	if err != nil {
		t.Fatalf("ListPersonaExtractErrors: %v", err)
	}
	if len(got.Entries) != 1 || got.Entries[0].Session != "fresh" {
		t.Fatalf("since=30m should keep only the fresh row; got %+v", got.Entries)
	}
}

func TestListPersonaExtractErrorsTailKeepsLastN(t *testing.T) {
	rt := seedPersonaLogFile(t, []string{
		"2026-05-28T10:00:00Z\tstage=extract\tsession=s1\terror=\"e1\"",
		"2026-05-28T10:01:00Z\tstage=extract\tsession=s2\terror=\"e2\"",
		"2026-05-28T10:02:00Z\tstage=extract\tsession=s3\terror=\"e3\"",
		"2026-05-28T10:03:00Z\tstage=extract\tsession=s4\terror=\"e4\"",
		"2026-05-28T10:04:00Z\tstage=extract\tsession=s5\terror=\"e5\"",
	})

	got, err := rt.ListPersonaExtractErrors(PersonaErrorsFilter{
		Tail: 2,
		Now:  time.Date(2026, 5, 28, 10, 30, 0, 0, time.UTC),
	})
	if err != nil {
		t.Fatalf("ListPersonaExtractErrors: %v", err)
	}
	if len(got.Entries) != 2 {
		t.Fatalf("tail=2 should keep 2 entries; got %d", len(got.Entries))
	}
	if got.Entries[0].Session != "s4" || got.Entries[1].Session != "s5" {
		t.Fatalf("tail=2 should keep s4+s5 in order; got %+v", got.Entries)
	}
	if got.TotalEntries != 5 {
		t.Fatalf("TotalEntries = %d, want 5 (raw count)", got.TotalEntries)
	}
}

func TestListPersonaExtractErrorsParsesModelTagColumns(t *testing.T) {
	// Round-trip the B-line model-consistency contract through the
	// app-level reader: a line with model/base_url columns must
	// expose them on the parsed entry so the TUI panel can show
	// which upstream produced each failure.
	rt := seedPersonaLogFile(t, []string{
		"2026-05-28T10:00:00Z\tstage=extract\tsession=tagged\terror=\"timeout\"\tmodel=deepseek-chat\tbase_url=\"https://api.deepseek.com/v1\"",
	})

	got, err := rt.ListPersonaExtractErrors(PersonaErrorsFilter{
		Now: time.Date(2026, 5, 28, 10, 30, 0, 0, time.UTC),
	})
	if err != nil {
		t.Fatalf("ListPersonaExtractErrors: %v", err)
	}
	if len(got.Entries) != 1 {
		t.Fatalf("entries = %d, want 1", len(got.Entries))
	}
	e := got.Entries[0]
	if e.Model != "deepseek-chat" {
		t.Fatalf("Model = %q, want deepseek-chat", e.Model)
	}
	if e.BaseURL != "https://api.deepseek.com/v1" {
		t.Fatalf("BaseURL = %q, want unquoted form", e.BaseURL)
	}
}

func TestReadPersonaLogFileMissingPathReturnsExistsFalse(t *testing.T) {
	// Distinguishing "log file does not exist yet" from "log file
	// exists but is empty" matters for dashboard rendering: missing
	// means "no extraction has ever run failures here", empty means
	// "extraction is wired but nothing has failed yet". The helper
	// must collapse the os.IsNotExist case into (nil, false, nil) so
	// dashboards branch on exists rather than parsing error.
	missing := filepath.Join(t.TempDir(), "never-created.log")
	entries, exists, err := readPersonaLogFile(missing)
	if err != nil {
		t.Fatalf("readPersonaLogFile on missing path: %v (want nil)", err)
	}
	if exists {
		t.Fatalf("exists = true on missing path; want false")
	}
	if entries != nil {
		t.Fatalf("entries on missing path = %+v, want nil", entries)
	}
}

func TestListPersonaExtractErrorsEmptyLogReturnsZeroEntries(t *testing.T) {
	// Fresh workdir: runtime opens the persona-extract.log file on
	// boot via openPersonaExtractLog so it always exists, but a
	// brand-new workdir has zero failures so the file is empty.
	// ListPersonaExtractErrors must return LogExists=true with
	// TotalEntries=0 + a non-nil empty Entries slice for stable JSON.
	configtest.IsolateHome(t)
	workDir := t.TempDir()
	rt, err := OpenRuntimeWithConfigOptions(workDir, configtest.IsolatedOptions(t))
	if err != nil {
		t.Fatalf("OpenRuntime: %v", err)
	}
	defer rt.Close()

	got, err := rt.ListPersonaExtractErrors(PersonaErrorsFilter{})
	if err != nil {
		t.Fatalf("ListPersonaExtractErrors on empty log: %v", err)
	}
	if !got.LogExists {
		t.Fatalf("LogExists = false on opened-but-empty log; want true")
	}
	if got.TotalEntries != 0 || len(got.Entries) != 0 {
		t.Fatalf("empty log should give zero counts; got %+v", got)
	}
	if got.Entries == nil {
		t.Fatalf("Entries should be non-nil empty slice for stable JSON shape")
	}
}

func TestParsePersonaExtractLogLineKeepsMalformedRaw(t *testing.T) {
	got := parsePersonaExtractLogLine("this is not a tab-delimited log line")
	if got.Parsed {
		t.Fatalf("parsed = true for garbage line; want false")
	}
	if got.Raw != "this is not a tab-delimited log line" {
		t.Fatalf("Raw should preserve the original; got %q", got.Raw)
	}
}

func TestParsePersonaExtractLogLineLegacyFourColumnStaysParsed(t *testing.T) {
	got := parsePersonaExtractLogLine("2026-05-22T10:00:00Z\tstage=extract\tsession=legacy\terror=\"boom\"")
	if !got.Parsed {
		t.Fatalf("legacy 4-column line should be parsed; got %+v", got)
	}
	if got.Stage != "extract" || got.Session != "legacy" || got.Error != "boom" {
		t.Fatalf("legacy parse mismatch: %+v", got)
	}
	if got.Model != "" || got.BaseURL != "" {
		t.Fatalf("legacy line should leave Model/BaseURL empty; got %+v", got)
	}
}
