package cli

import (
	"bytes"
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
