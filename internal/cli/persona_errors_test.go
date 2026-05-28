package cli

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"obsidian-harness/internal/app"
	"obsidian-harness/internal/config/configtest"
)

// writePersonaLog seeds the persona-extract.log file with the given
// lines for renderPersonaExtractErrors unit tests. Mirrors the
// real session.go logger format: ts\tstage=X\tsession=Y\terror="..."
func writePersonaLog(t *testing.T, lines []string) string {
	t.Helper()
	dir := t.TempDir()
	logPath := filepath.Join(dir, "persona-extract.log")
	if err := os.WriteFile(logPath, []byte(strings.Join(lines, "\n")+"\n"), 0o644); err != nil {
		t.Fatalf("write log fixture: %v", err)
	}
	return logPath
}

func TestRenderPersonaExtractErrorsShowsAllByDefault(t *testing.T) {
	logPath := writePersonaLog(t, []string{
		"2026-05-22T10:00:00Z\tstage=extract\tsession=s1\terror=\"timeout\"",
		"2026-05-22T10:01:00Z\tstage=store\tsession=s2\terror=\"disk full\"",
		"2026-05-22T10:02:00Z\tstage=parse_warning\tsession=s3\terror=\"low confidence\"",
	})

	var stdout, stderr bytes.Buffer
	now := time.Date(2026, 5, 22, 10, 30, 0, 0, time.UTC)
	if exit := renderPersonaExtractErrors(&stdout, &stderr, logPath, 0, "", 0, now); exit != 0 {
		t.Fatalf("exit = %d, stderr = %q", exit, stderr.String())
	}
	out := stdout.String()
	for _, want := range []string{
		"Persona Extraction Errors",
		logPath,
		"stage=extract",
		"stage=store",
		"stage=parse_warning",
		"3 of 3 entries shown",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("output missing %q:\n%s", want, out)
		}
	}
}

func TestRenderPersonaExtractErrorsTailLimitsToLastN(t *testing.T) {
	// Five entries seeded; --tail 2 keeps only the newest two.
	logPath := writePersonaLog(t, []string{
		"2026-05-22T10:00:00Z\tstage=extract\tsession=s1\terror=\"e1\"",
		"2026-05-22T10:01:00Z\tstage=extract\tsession=s2\terror=\"e2\"",
		"2026-05-22T10:02:00Z\tstage=extract\tsession=s3\terror=\"e3\"",
		"2026-05-22T10:03:00Z\tstage=extract\tsession=s4\terror=\"e4\"",
		"2026-05-22T10:04:00Z\tstage=extract\tsession=s5\terror=\"e5\"",
	})

	var stdout, stderr bytes.Buffer
	now := time.Date(2026, 5, 22, 10, 30, 0, 0, time.UTC)
	if exit := renderPersonaExtractErrors(&stdout, &stderr, logPath, 2, "", 0, now); exit != 0 {
		t.Fatalf("exit = %d", exit)
	}
	out := stdout.String()
	if !strings.Contains(out, "session=s4") || !strings.Contains(out, "session=s5") {
		t.Fatalf("tail=2 should keep s4 and s5:\n%s", out)
	}
	for _, gone := range []string{"session=s1", "session=s2", "session=s3"} {
		if strings.Contains(out, gone) {
			t.Fatalf("tail=2 should drop %q:\n%s", gone, out)
		}
	}
	if !strings.Contains(out, "tail=2") {
		t.Fatalf("output missing tail filter label:\n%s", out)
	}
}

func TestRenderPersonaExtractErrorsStageFilter(t *testing.T) {
	logPath := writePersonaLog(t, []string{
		"2026-05-22T10:00:00Z\tstage=extract\tsession=s1\terror=\"timeout\"",
		"2026-05-22T10:01:00Z\tstage=store\tsession=s2\terror=\"disk full\"",
		"2026-05-22T10:02:00Z\tstage=parse_warning\tsession=s3\terror=\"low confidence\"",
	})

	var stdout, stderr bytes.Buffer
	now := time.Date(2026, 5, 22, 10, 30, 0, 0, time.UTC)
	if exit := renderPersonaExtractErrors(&stdout, &stderr, logPath, 0, "parse_warning", 0, now); exit != 0 {
		t.Fatalf("exit = %d", exit)
	}
	out := stdout.String()
	if !strings.Contains(out, "session=s3") {
		t.Fatalf("stage filter should keep parse_warning row:\n%s", out)
	}
	for _, gone := range []string{"session=s1", "session=s2"} {
		if strings.Contains(out, gone) {
			t.Fatalf("stage=parse_warning should drop %q:\n%s", gone, out)
		}
	}
	if !strings.Contains(out, "1 of 1 entries shown (filtered from 3 total)") {
		t.Fatalf("output missing filtered footer:\n%s", out)
	}
}

func TestRenderPersonaExtractErrorsSinceFilter(t *testing.T) {
	// Two old entries + one recent entry; --since 30m keeps only the recent one.
	logPath := writePersonaLog(t, []string{
		"2026-05-22T08:00:00Z\tstage=extract\tsession=old1\terror=\"e\"",
		"2026-05-22T09:00:00Z\tstage=extract\tsession=old2\terror=\"e\"",
		"2026-05-22T10:20:00Z\tstage=extract\tsession=recent\terror=\"e\"",
	})

	var stdout, stderr bytes.Buffer
	now := time.Date(2026, 5, 22, 10, 30, 0, 0, time.UTC)
	if exit := renderPersonaExtractErrors(&stdout, &stderr, logPath, 0, "", 30*time.Minute, now); exit != 0 {
		t.Fatalf("exit = %d", exit)
	}
	out := stdout.String()
	if !strings.Contains(out, "session=recent") {
		t.Fatalf("since=30m should keep recent row:\n%s", out)
	}
	for _, gone := range []string{"session=old1", "session=old2"} {
		if strings.Contains(out, gone) {
			t.Fatalf("since=30m should drop %q:\n%s", gone, out)
		}
	}
	if !strings.Contains(out, "since=30m0s") {
		t.Fatalf("output missing since filter label:\n%s", out)
	}
}

func TestRenderPersonaExtractErrorsNoLogFileIsFriendly(t *testing.T) {
	// Pre-runtime workdir or post-cleanup state: log file missing
	// is normal, not an error. The command should explain the
	// state without exit code 1 so operator scripts can pipe it
	// safely.
	missing := filepath.Join(t.TempDir(), "nonexistent.log")
	var stdout, stderr bytes.Buffer
	if exit := renderPersonaExtractErrors(&stdout, &stderr, missing, 0, "", 0, time.Now().UTC()); exit != 0 {
		t.Fatalf("exit = %d, want 0 for missing log", exit)
	}
	out := stdout.String()
	if !strings.Contains(out, "No log file yet") {
		t.Fatalf("output missing friendly empty message:\n%s", out)
	}
}

func TestRenderPersonaExtractErrorsKeepsMalformedRawLines(t *testing.T) {
	// A line that does not match the canonical format (e.g. a
	// future schema, a partial line from a crashed write) is still
	// shown to the operator -- dropping it silently would hide a
	// real signal. Stage / since filters skip it because parsing
	// failed, but with no filters the line lands in the output.
	logPath := writePersonaLog(t, []string{
		"this is not a tab-delimited log line",
		"2026-05-22T10:00:00Z\tstage=extract\tsession=s1\terror=\"e\"",
	})

	var stdout, stderr bytes.Buffer
	now := time.Date(2026, 5, 22, 10, 30, 0, 0, time.UTC)
	if exit := renderPersonaExtractErrors(&stdout, &stderr, logPath, 0, "", 0, now); exit != 0 {
		t.Fatalf("exit = %d", exit)
	}
	out := stdout.String()
	if !strings.Contains(out, "this is not a tab-delimited log line") {
		t.Fatalf("malformed line dropped silently:\n%s", out)
	}
	if !strings.Contains(out, "session=s1") {
		t.Fatalf("canonical line missing:\n%s", out)
	}
}

func TestRenderPersonaExtractErrorsEmptyFile(t *testing.T) {
	// File exists but has no entries. Distinct from missing-file:
	// extraction is running, just nothing failed yet.
	logPath := filepath.Join(t.TempDir(), "persona-extract.log")
	if err := os.WriteFile(logPath, []byte(""), 0o644); err != nil {
		t.Fatalf("write empty fixture: %v", err)
	}
	var stdout, stderr bytes.Buffer
	now := time.Date(2026, 5, 22, 10, 30, 0, 0, time.UTC)
	if exit := renderPersonaExtractErrors(&stdout, &stderr, logPath, 0, "", 0, now); exit != 0 {
		t.Fatalf("exit = %d", exit)
	}
	out := stdout.String()
	if !strings.Contains(out, "No matching entries") {
		t.Fatalf("output missing empty marker:\n%s", out)
	}
}

func TestRunPersonaErrorsEndToEnd(t *testing.T) {
	// CLI surface test: seed a log file in a real workdir's state
	// directory via OpenRuntime, then invoke `lore persona errors`
	// through Run() and assert the output reaches stdout.
	configtest.IsolateHome(t)
	workDir := t.TempDir()

	// Open the runtime once so the state directory + log file
	// layout get materialized, then close so the file handle is
	// released before the CLI under test opens its own.
	runtime, err := app.OpenRuntimeWithConfigOptions(workDir, configtest.IsolatedOptions(t))
	if err != nil {
		t.Fatalf("OpenRuntime: %v", err)
	}
	logPath := runtime.PersonaExtractLogPath()
	if logPath == "" {
		runtime.Close()
		t.Fatal("PersonaExtractLogPath() returned empty")
	}
	if err := runtime.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	// Append a deterministic failure line so the read-side
	// command has something to surface.
	line := "2026-05-22T10:00:00Z\tstage=extract\tsession=lore-test\terror=\"upstream provider unavailable\"\n"
	if err := os.WriteFile(logPath, []byte(line), 0o644); err != nil {
		t.Fatalf("seed log line: %v", err)
	}

	var stdout, stderr bytes.Buffer
	exit := Run([]string{"persona", "errors", "--workdir", workDir}, &bytes.Buffer{}, &stdout, &stderr, "test")
	if exit != 0 {
		t.Fatalf("exit = %d, stderr = %q", exit, stderr.String())
	}
	out := stdout.String()
	for _, want := range []string{
		"Persona Extraction Errors",
		logPath,
		"upstream provider unavailable",
		"session=lore-test",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("output missing %q:\n%s", want, out)
		}
	}
}

func TestRunPersonaErrorsRejectsBadStage(t *testing.T) {
	configtest.IsolateHome(t)
	workDir := t.TempDir()

	var stdout, stderr bytes.Buffer
	exit := Run([]string{"persona", "errors", "--workdir", workDir, "--stage", "bogus"}, &bytes.Buffer{}, &stdout, &stderr, "test")
	if exit == 0 {
		t.Fatalf("expected non-zero exit for bad --stage; stdout=%q", stdout.String())
	}
	if !strings.Contains(stderr.String(), "extract, store, parse_warning") {
		t.Fatalf("stderr should mention valid stages: %q", stderr.String())
	}
}

func TestRunPersonaErrorsRejectsNegativeTail(t *testing.T) {
	configtest.IsolateHome(t)
	workDir := t.TempDir()

	var stdout, stderr bytes.Buffer
	exit := Run([]string{"persona", "errors", "--workdir", workDir, "--tail", "-1"}, &bytes.Buffer{}, &stdout, &stderr, "test")
	if exit == 0 {
		t.Fatalf("expected non-zero exit for negative --tail; stdout=%q", stdout.String())
	}
	if !strings.Contains(stderr.String(), "--tail must be >= 0") {
		t.Fatalf("stderr missing tail validation message: %q", stderr.String())
	}
}

func TestRenderPersonaErrorsJSONShapeIsStable(t *testing.T) {
	// JSON mode mirrors the summary --json contract: stable
	// field names so monitoring consumers can rely on them. Each
	// entry exposes the structured fields (timestamp, stage,
	// session, error) plus the original raw line so a downstream
	// reader can still grep the file format.
	logPath := writePersonaLog(t, []string{
		"2026-05-23T01:00:00Z\tstage=extract\tsession=lore-test\terror=\"upstream timeout\"",
		"2026-05-23T01:05:00Z\tstage=parse_warning\tsession=lore-test\terror=\"low confidence\"",
	})

	var stdout, stderr bytes.Buffer
	now := time.Date(2026, 5, 23, 2, 0, 0, 0, time.UTC)
	if exit := emitPersonaExtractErrorsJSON(&stdout, &stderr, logPath, 0, "", 0, now); exit != 0 {
		t.Fatalf("exit = %d, stderr = %q", exit, stderr.String())
	}
	out := strings.TrimSpace(stdout.String())
	if strings.Count(out, "\n") != 0 {
		t.Fatalf("JSON output should be a single line, got:\n%s", out)
	}

	var got struct {
		LogPath string `json:"log_path"`
		Filter  struct {
			Tail         int    `json:"tail"`
			Stage        string `json:"stage"`
			SinceSeconds int64  `json:"since_seconds"`
		} `json:"filter"`
		Summary struct {
			TotalEntries int `json:"total_entries"`
			Shown        int `json:"shown"`
		} `json:"summary"`
		Entries []struct {
			Timestamp string `json:"timestamp"`
			Stage     string `json:"stage"`
			Session   string `json:"session"`
			Error     string `json:"error"`
			Raw       string `json:"raw"`
		} `json:"entries"`
	}
	if err := json.Unmarshal([]byte(out), &got); err != nil {
		t.Fatalf("output is not valid JSON: %v\nstdout: %s", err, out)
	}
	if got.LogPath != logPath {
		t.Fatalf("log_path = %q, want %q", got.LogPath, logPath)
	}
	if got.Summary.TotalEntries != 2 || got.Summary.Shown != 2 {
		t.Fatalf("summary = %+v, want total=2 shown=2", got.Summary)
	}
	if len(got.Entries) != 2 {
		t.Fatalf("entries = %d, want 2", len(got.Entries))
	}
	if got.Entries[0].Stage != "extract" {
		t.Fatalf("entries[0].stage = %q, want extract", got.Entries[0].Stage)
	}
	if got.Entries[0].Session != "lore-test" {
		t.Fatalf("entries[0].session = %q, want lore-test", got.Entries[0].Session)
	}
	if got.Entries[0].Error != "upstream timeout" {
		t.Fatalf("entries[0].error = %q, want unquoted 'upstream timeout'", got.Entries[0].Error)
	}
	if !strings.Contains(got.Entries[0].Raw, "\t") {
		t.Fatalf("entries[0].raw should preserve tabs from original line; got %q", got.Entries[0].Raw)
	}
}

func TestRenderPersonaErrorsJSONHonorsStageAndSinceFilters(t *testing.T) {
	// JSON mode applies the same filter-then-tail order as the
	// human render: stage + since first, tail last. Summary.shown
	// reflects after filtering; total_entries reflects raw count.
	logPath := writePersonaLog(t, []string{
		"2026-05-23T00:00:00Z\tstage=extract\tsession=s1\terror=\"e1\"",
		"2026-05-23T00:30:00Z\tstage=store\tsession=s2\terror=\"e2\"",
		"2026-05-23T01:00:00Z\tstage=parse_warning\tsession=s3\terror=\"e3\"",
		"2026-05-23T01:30:00Z\tstage=parse_warning\tsession=s4\terror=\"e4\"",
	})

	var stdout, stderr bytes.Buffer
	now := time.Date(2026, 5, 23, 2, 0, 0, 0, time.UTC)
	if exit := emitPersonaExtractErrorsJSON(&stdout, &stderr, logPath, 0, "parse_warning", 90*time.Minute, now); exit != 0 {
		t.Fatalf("exit = %d", exit)
	}
	var got struct {
		Summary struct {
			TotalEntries int `json:"total_entries"`
			Shown        int `json:"shown"`
		} `json:"summary"`
		Entries []struct {
			Session string `json:"session"`
		} `json:"entries"`
	}
	if err := json.Unmarshal([]byte(strings.TrimSpace(stdout.String())), &got); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	if got.Summary.TotalEntries != 4 {
		t.Fatalf("total_entries = %d, want 4 (raw count regardless of filter)", got.Summary.TotalEntries)
	}
	if got.Summary.Shown != 2 {
		t.Fatalf("shown = %d, want 2 (stage=parse_warning + within last 90m)", got.Summary.Shown)
	}
	sessions := []string{got.Entries[0].Session, got.Entries[1].Session}
	for _, want := range []string{"s3", "s4"} {
		found := false
		for _, s := range sessions {
			if s == want {
				found = true
			}
		}
		if !found {
			t.Fatalf("expected session %q in filtered entries, got %v", want, sessions)
		}
	}
}

func TestRenderPersonaErrorsJSONMissingLogReturnsEmptyEntries(t *testing.T) {
	// Missing log -> JSON with empty entries + total=0 + exit 0.
	// Matches the human render's "no log file yet" branch
	// semantically; programmatic consumers prefer empty arrays
	// over distinct error codes for an empty workdir.
	missing := filepath.Join(t.TempDir(), "nonexistent.log")
	var stdout, stderr bytes.Buffer
	if exit := emitPersonaExtractErrorsJSON(&stdout, &stderr, missing, 0, "", 0, time.Now().UTC()); exit != 0 {
		t.Fatalf("exit = %d, want 0 for missing log", exit)
	}
	var got struct {
		Summary struct {
			TotalEntries int `json:"total_entries"`
			Shown        int `json:"shown"`
		} `json:"summary"`
		Entries []map[string]any `json:"entries"`
	}
	if err := json.Unmarshal([]byte(strings.TrimSpace(stdout.String())), &got); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	if got.Summary.TotalEntries != 0 || got.Summary.Shown != 0 {
		t.Fatalf("missing log should give zero counts; got %+v", got.Summary)
	}
	if got.Entries == nil {
		t.Fatalf("entries field should serialize as [] not null")
	}
}

func TestRunPersonaErrorsJSONEndToEnd(t *testing.T) {
	// Full CLI path through Run() with --json flag set.
	configtest.IsolateHome(t)
	workDir := t.TempDir()

	runtime, err := app.OpenRuntimeWithConfigOptions(workDir, configtest.IsolatedOptions(t))
	if err != nil {
		t.Fatalf("seed OpenRuntime: %v", err)
	}
	logPath := runtime.PersonaExtractLogPath()
	if err := runtime.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	line := "2026-05-23T01:00:00Z\tstage=extract\tsession=lore-cli-test\terror=\"upstream provider unavailable\"\n"
	if err := os.WriteFile(logPath, []byte(line), 0o644); err != nil {
		t.Fatalf("seed line: %v", err)
	}

	var stdout, stderr bytes.Buffer
	exit := Run([]string{"persona", "errors", "--workdir", workDir, "--json"}, &bytes.Buffer{}, &stdout, &stderr, "test")
	if exit != 0 {
		t.Fatalf("exit = %d, stderr = %q", exit, stderr.String())
	}
	var got struct {
		Summary struct {
			Shown int `json:"shown"`
		} `json:"summary"`
		Entries []struct {
			Stage   string `json:"stage"`
			Error   string `json:"error"`
			Session string `json:"session"`
		} `json:"entries"`
	}
	if err := json.Unmarshal([]byte(strings.TrimSpace(stdout.String())), &got); err != nil {
		t.Fatalf("invalid JSON: %v\nstdout: %s", err, stdout.String())
	}
	if got.Summary.Shown != 1 {
		t.Fatalf("shown = %d, want 1", got.Summary.Shown)
	}
	if got.Entries[0].Stage != "extract" {
		t.Fatalf("stage = %q, want extract", got.Entries[0].Stage)
	}
	if got.Entries[0].Error != "upstream provider unavailable" {
		t.Fatalf("error = %q, want unquoted", got.Entries[0].Error)
	}
	if got.Entries[0].Session != "lore-cli-test" {
		t.Fatalf("session = %q, want lore-cli-test", got.Entries[0].Session)
	}
}

// Note: the round-trip, legacy 4-column, and model-only-tail parser
// tests live in internal/app/persona_log_test.go now that the parsing
// helper itself lives in the app layer (see
// TestListPersonaExtractErrorsParsesModelTagColumns,
// TestParsePersonaExtractLogLineLegacyFourColumnStaysParsed, and
// TestParsePersonaExtractLogLineHandlesModelOnlyTail). The cli still
// owns the JSON / human rendering tests below.

// TestEmitPersonaExtractErrorsJSONIncludesModelAndBaseURL covers the
// downstream JSON surface: when the log carries model-tag columns,
// `lore persona errors --json` must expose them through the
// `model` and `base_url` keys (omitempty on legacy lines).
func TestEmitPersonaExtractErrorsJSONIncludesModelAndBaseURL(t *testing.T) {
	logPath := writePersonaLog(t, []string{
		"2026-05-26T01:00:00Z\tstage=extract\tsession=tagged\terror=\"timeout\"\tmodel=deepseek-chat\tbase_url=\"https://api.deepseek.com/v1\"",
		"2026-05-26T01:01:00Z\tstage=extract\tsession=legacy\terror=\"timeout\"",
	})

	var stdout, stderr bytes.Buffer
	now := time.Date(2026, 5, 26, 2, 0, 0, 0, time.UTC)
	if exit := emitPersonaExtractErrorsJSON(&stdout, &stderr, logPath, 0, "", 0, now); exit != 0 {
		t.Fatalf("exit = %d, stderr = %q", exit, stderr.String())
	}
	var got struct {
		Entries []struct {
			Session string `json:"session"`
			Model   string `json:"model"`
			BaseURL string `json:"base_url"`
		} `json:"entries"`
	}
	if err := json.Unmarshal([]byte(strings.TrimSpace(stdout.String())), &got); err != nil {
		t.Fatalf("invalid JSON: %v\nstdout: %s", err, stdout.String())
	}
	if len(got.Entries) != 2 {
		t.Fatalf("entries = %d, want 2", len(got.Entries))
	}
	tagged := got.Entries[0]
	if tagged.Session != "tagged" {
		t.Fatalf("entries[0].session = %q, want tagged", tagged.Session)
	}
	if tagged.Model != "deepseek-chat" {
		t.Fatalf("entries[0].model = %q, want deepseek-chat", tagged.Model)
	}
	if tagged.BaseURL != "https://api.deepseek.com/v1" {
		t.Fatalf("entries[0].base_url = %q, want unquoted form", tagged.BaseURL)
	}
	legacy := got.Entries[1]
	if legacy.Session != "legacy" {
		t.Fatalf("entries[1].session = %q, want legacy", legacy.Session)
	}
	if legacy.Model != "" || legacy.BaseURL != "" {
		t.Fatalf("entries[1] model/base_url should be empty for legacy line; got model=%q base=%q", legacy.Model, legacy.BaseURL)
	}
	// Confirm omitempty: the raw JSON for the legacy entry must not
	// carry the keys at all so downstream monitoring scripts can
	// distinguish "absent" from "empty string".
	raw := stdout.String()
	if strings.Count(raw, `"model":""`) != 0 || strings.Count(raw, `"base_url":""`) != 0 {
		t.Fatalf("omitempty broken: legacy entry emitted empty model/base_url keys:\n%s", raw)
	}
}

func TestRunPersonaWithoutSubcommandShowsExtendedUsage(t *testing.T) {
	configtest.IsolateHome(t)
	var stdout, stderr bytes.Buffer
	exit := Run([]string{"persona"}, &bytes.Buffer{}, &stdout, &stderr, "test")
	if exit == 0 {
		t.Fatalf("expected non-zero exit for no subcommand")
	}
	for _, want := range []string{"candidates", "errors"} {
		if !strings.Contains(stderr.String(), want) {
			t.Fatalf("stderr usage should mention %q: %q", want, stderr.String())
		}
	}
}
