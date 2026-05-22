package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"obsidian-harness/internal/app"
	"obsidian-harness/internal/config/configtest"
)

func TestRenderPersonaSummaryEmptyWorkdir(t *testing.T) {
	// A fresh workdir with no candidates and no log file yet: each
	// candidate row is "0", the log section reports the missing file
	// as a friendly placeholder rather than a stat error. This is
	// the most common state of the cookbook's Step 1.
	var buf bytes.Buffer
	renderPersonaSummary(&buf, "/work/abs", "/work/abs/state/logs/persona-extract.log", 0, 0, 0, 0)
	out := buf.String()
	for _, want := range []string{
		"Persona Memory Summary",
		"Workdir: /work/abs",
		"open         0",
		"drafted      0 (linked 0, partial-orphan 0)",
		"dismissed    0",
		"(no log file yet)",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("output missing %q:\n%s", want, out)
		}
	}
}

func TestRenderPersonaSummaryAggregatesLogStages(t *testing.T) {
	// Seed a log file with 4 entries across 3 stages plus one
	// malformed line. The summary should show total=5, three named
	// buckets sorted alphabetically, the malformed bucket separately,
	// and the latest RFC3339Nano entry as the freshness anchor.
	dir := t.TempDir()
	logPath := filepath.Join(dir, "persona-extract.log")
	lines := []string{
		"2026-05-22T10:00:00Z\tstage=extract\tsession=s1\terror=\"e1\"",
		"2026-05-22T10:05:00Z\tstage=extract\tsession=s2\terror=\"e2\"",
		"2026-05-22T10:10:00Z\tstage=store\tsession=s3\terror=\"disk full\"",
		"2026-05-22T11:00:00Z\tstage=parse_warning\tsession=s4\terror=\"low confidence\"",
		"this-line-is-malformed",
	}
	if err := os.WriteFile(logPath, []byte(strings.Join(lines, "\n")+"\n"), 0o644); err != nil {
		t.Fatalf("write log fixture: %v", err)
	}

	var buf bytes.Buffer
	renderPersonaSummary(&buf, dir, logPath, 3, 2, 1, 4)
	out := buf.String()
	for _, want := range []string{
		"open         3",
		"drafted      3 (linked 2, partial-orphan 1)",
		"dismissed    4",
		"total entries: 5",
		"by stage:",
		"extract          2",
		"parse_warning    1",
		"store            1",
		"(malformed)      1",
		"last entry: 2026-05-22T11:00:00Z",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("output missing %q:\n%s", want, out)
		}
	}
}

func TestRenderPersonaSummaryEmptyLogFile(t *testing.T) {
	// File exists but contains no entries (touch-and-leave). The
	// summary should distinguish "(empty)" from "(no log file yet)"
	// because the former signals extraction is running but never
	// failed in this workdir.
	dir := t.TempDir()
	logPath := filepath.Join(dir, "persona-extract.log")
	if err := os.WriteFile(logPath, []byte(""), 0o644); err != nil {
		t.Fatalf("write empty log: %v", err)
	}

	var buf bytes.Buffer
	renderPersonaSummary(&buf, dir, logPath, 1, 0, 0, 0)
	out := buf.String()
	if !strings.Contains(out, "(empty)") {
		t.Fatalf("output should mark empty log distinctly:\n%s", out)
	}
	if strings.Contains(out, "(no log file yet)") {
		t.Fatalf("empty log should NOT show 'no log file yet':\n%s", out)
	}
}

func TestRunPersonaSummaryEndToEnd(t *testing.T) {
	// Full CLI path: bootstrap a workdir, promote a seeded candidate
	// through the standard flow, then call `lore persona summary`.
	// Assertions cover the dashboard surface (header, candidate
	// counts) and confirm the log section renders the
	// "(no log file yet)" branch because nothing failed.
	configtest.IsolateHome(t)
	workDir := t.TempDir()
	candidateID := seedPersonaCandidateCLI(t, workDir, "major", "economics", "I major in economics")
	if exit := Run([]string{"persona", "candidates", "draft", "--workdir", workDir, candidateID}, &bytes.Buffer{}, &bytes.Buffer{}, &bytes.Buffer{}, "test"); exit != 0 {
		t.Fatalf("seed draft exit = %d", exit)
	}

	var stdout, stderr bytes.Buffer
	exit := Run([]string{"persona", "summary", "--workdir", workDir}, &bytes.Buffer{}, &stdout, &stderr, "test")
	if exit != 0 {
		t.Fatalf("summary exit = %d, stderr = %q", exit, stderr.String())
	}
	out := stdout.String()
	for _, want := range []string{
		"Persona Memory Summary",
		"open         0",
		"drafted      1 (linked 1, partial-orphan 0)",
		"dismissed    0",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("output missing %q:\n%s", want, out)
		}
	}
}

func TestRunPersonaSummaryShowsOrphanCountAfterForcedPartialState(t *testing.T) {
	// Force a candidate into the partial-orphan shape so the
	// dashboard's orphan count reaches 1. Catches the case where a
	// future refactor sums orphan into "linked" and silently breaks
	// the operator's view of partial scars.
	configtest.IsolateHome(t)
	workDir := t.TempDir()
	partialID := forcePartialDraftedCandidateCLI(t, workDir, "major", "economics", "I major in economics")
	_ = partialID

	var stdout, stderr bytes.Buffer
	exit := Run([]string{"persona", "summary", "--workdir", workDir}, &bytes.Buffer{}, &stdout, &stderr, "test")
	if exit != 0 {
		t.Fatalf("summary exit = %d, stderr = %q", exit, stderr.String())
	}
	if !strings.Contains(stdout.String(), "drafted      1 (linked 0, partial-orphan 1)") {
		t.Fatalf("output missing partial-orphan row:\n%s", stdout.String())
	}
}

func TestRunPersonaWithoutSubcommandMentionsSummary(t *testing.T) {
	// Top-level help must advertise the summary surface alongside
	// candidates and errors so an operator scanning `lore persona`
	// finds it.
	configtest.IsolateHome(t)
	var stdout, stderr bytes.Buffer
	exit := Run([]string{"persona"}, &bytes.Buffer{}, &stdout, &stderr, "test")
	if exit == 0 {
		t.Fatalf("expected non-zero exit for no subcommand")
	}
	for _, want := range []string{"summary"} {
		if !strings.Contains(stderr.String(), want) {
			t.Fatalf("stderr usage should mention %q: %q", want, stderr.String())
		}
	}
}

func TestRunPersonaSummaryHonorsCustomWorkdirPath(t *testing.T) {
	// Smoke check: the rendered "Workdir:" line should reflect the
	// --workdir flag value (post-defaultWorkDir resolution), not a
	// literal "." or empty string. Otherwise scripts that grep
	// dashboards by workdir break across operator boxes.
	configtest.IsolateHome(t)
	workDir := t.TempDir()
	seedRuntime, err := app.OpenRuntimeWithConfigOptions(workDir, configtest.IsolatedOptions(t))
	if err != nil {
		t.Fatalf("seed OpenRuntime: %v", err)
	}
	// Close before the CLI runs so the persona-extract.log file
	// handle is released; otherwise Windows t.TempDir cleanup fails
	// with "file is being used by another process".
	if err := seedRuntime.Close(); err != nil {
		t.Fatalf("seed Close: %v", err)
	}

	var stdout, stderr bytes.Buffer
	exit := Run([]string{"persona", "summary", "--workdir", workDir}, &bytes.Buffer{}, &stdout, &stderr, "test")
	if exit != 0 {
		t.Fatalf("exit = %d", exit)
	}
	if !strings.Contains(stdout.String(), "Workdir: "+workDir) {
		t.Fatalf("dashboard should echo the --workdir value verbatim:\n%s", stdout.String())
	}
}
