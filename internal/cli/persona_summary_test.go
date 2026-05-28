package cli

import (
	"bytes"
	"encoding/json"
	"os"
	"strings"
	"testing"
	"time"

	"obsidian-harness/internal/app"
	"obsidian-harness/internal/config/configtest"
)

func TestRenderPersonaSummaryEmptyWorkdir(t *testing.T) {
	// A fresh workdir with no candidates and no log file yet: each
	// candidate row is "0", the log section reports the missing file
	// as a friendly placeholder rather than a stat error. This is
	// the most common state of the cookbook's Step 1.
	view := app.PersonaSummaryView{
		Workdir: "/work/abs",
		LogPath: "/work/abs/state/logs/persona-extract.log",
		ExtractLog: app.PersonaExtractLogSummary{
			Exists:  false,
			ByStage: map[string]int{},
		},
	}
	var buf bytes.Buffer
	renderPersonaSummary(&buf, view)
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
	// Synthesize a view with 5 log entries across 3 named stages
	// plus the malformed bucket. The summary should show total=5,
	// the four buckets sorted alphabetically, and the latest
	// RFC3339Nano timestamp as the freshness anchor.
	latest, _ := time.Parse(time.RFC3339Nano, "2026-05-22T11:00:00Z")
	view := app.PersonaSummaryView{
		Workdir: "/work/sample",
		LogPath: "/work/sample/state/logs/persona-extract.log",
		Candidates: app.PersonaCandidateCounts{
			Open:          3,
			DraftedLinked: 2,
			DraftedOrphan: 1,
			Dismissed:     4,
		},
		ExtractLog: app.PersonaExtractLogSummary{
			Exists:       true,
			TotalEntries: 5,
			ByStage: map[string]int{
				"extract":       2,
				"store":         1,
				"parse_warning": 1,
				"(malformed)":   1,
			},
			LastEntry: latest,
		},
	}
	var buf bytes.Buffer
	renderPersonaSummary(&buf, view)
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
	view := app.PersonaSummaryView{
		Workdir: "/work/sample",
		LogPath: "/work/sample/state/logs/persona-extract.log",
		Candidates: app.PersonaCandidateCounts{
			Open: 1,
		},
		ExtractLog: app.PersonaExtractLogSummary{
			Exists:  true,
			ByStage: map[string]int{},
		},
	}
	var buf bytes.Buffer
	renderPersonaSummary(&buf, view)
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

func TestRunPersonaSummaryFailOnOrphanExitsTwoWhenOrphanPresent(t *testing.T) {
	// Health-check mode: --fail-on-orphan returns exit 2 when at
	// least one candidate is in the partial-orphan shape. Exit 2
	// is distinct from 0 (clean) and 1 (command error) so CI
	// scripts can branch on the actual signal.
	configtest.IsolateHome(t)
	workDir := t.TempDir()
	forcePartialDraftedCandidateCLI(t, workDir, "major", "economics", "I major in economics")

	var stdout, stderr bytes.Buffer
	exit := Run([]string{"persona", "summary", "--workdir", workDir, "--fail-on-orphan"}, &bytes.Buffer{}, &stdout, &stderr, "test")
	if exit != 2 {
		t.Fatalf("exit = %d, want 2 when --fail-on-orphan trips", exit)
	}
	// Dashboard still printed BEFORE the check signal, so the
	// operator sees the numbers regardless of exit code.
	if !strings.Contains(stdout.String(), "partial-orphan 1") {
		t.Fatalf("dashboard should still print the orphan row:\n%s", stdout.String())
	}
	for _, want := range []string{
		"--fail-on-orphan tripped",
		"1 partial-orphan candidate",
		"recover --link",
		"recover --force-dismiss",
	} {
		if !strings.Contains(stderr.String(), want) {
			t.Fatalf("stderr missing %q:\n%s", want, stderr.String())
		}
	}
}

func TestRunPersonaSummaryFailOnOrphanExitsZeroWhenClean(t *testing.T) {
	// Same flag, but no orphan present: exit 0, no health-check
	// stderr block. The dashboard still renders normally.
	configtest.IsolateHome(t)
	workDir := t.TempDir()
	candidateID := seedPersonaCandidateCLI(t, workDir, "major", "economics", "I major in economics")
	if exit := Run([]string{"persona", "candidates", "draft", "--workdir", workDir, candidateID}, &bytes.Buffer{}, &bytes.Buffer{}, &bytes.Buffer{}, "test"); exit != 0 {
		t.Fatalf("seed draft exit = %d", exit)
	}

	var stdout, stderr bytes.Buffer
	exit := Run([]string{"persona", "summary", "--workdir", workDir, "--fail-on-orphan"}, &bytes.Buffer{}, &stdout, &stderr, "test")
	if exit != 0 {
		t.Fatalf("exit = %d, want 0 when no orphan; stderr = %q", exit, stderr.String())
	}
	if strings.Contains(stderr.String(), "fail-on-orphan tripped") {
		t.Fatalf("clean state should not emit check message:\n%s", stderr.String())
	}
	if !strings.Contains(stdout.String(), "partial-orphan 0") {
		t.Fatalf("dashboard should show partial-orphan 0:\n%s", stdout.String())
	}
}

func TestRunPersonaSummaryNoFlagAllowsOrphan(t *testing.T) {
	// Default behavior (no --fail-on-orphan): orphan present, exit
	// still 0. The flag is opt-in so existing scripts that just
	// want the dashboard never get a new failure mode.
	configtest.IsolateHome(t)
	workDir := t.TempDir()
	forcePartialDraftedCandidateCLI(t, workDir, "major", "economics", "I major in economics")

	var stdout, stderr bytes.Buffer
	exit := Run([]string{"persona", "summary", "--workdir", workDir}, &bytes.Buffer{}, &stdout, &stderr, "test")
	if exit != 0 {
		t.Fatalf("exit = %d, want 0 without --fail-on-orphan; stderr = %q", exit, stderr.String())
	}
	if !strings.Contains(stdout.String(), "partial-orphan 1") {
		t.Fatalf("dashboard should still show orphan count:\n%s", stdout.String())
	}
}

func TestRunPersonaSummaryJSONShapeIsStable(t *testing.T) {
	// JSON output mode produces a single-line object with a
	// frozen-shape contract: workdir / log_path / candidates /
	// extract_log. Tests against the documented field names
	// because monitoring consumers parse them.
	configtest.IsolateHome(t)
	workDir := t.TempDir()
	// Promote one candidate so drafted_linked = 1; the unpromoted
	// remainder stays in open = 0 (only one candidate was seeded).
	candidateID := seedPersonaCandidateCLI(t, workDir, "major", "economics", "I major in economics")
	if exit := Run([]string{"persona", "candidates", "draft", "--workdir", workDir, candidateID}, &bytes.Buffer{}, &bytes.Buffer{}, &bytes.Buffer{}, "test"); exit != 0 {
		t.Fatalf("seed draft exit = %d", exit)
	}

	var stdout, stderr bytes.Buffer
	exit := Run([]string{"persona", "summary", "--workdir", workDir, "--json"}, &bytes.Buffer{}, &stdout, &stderr, "test")
	if exit != 0 {
		t.Fatalf("exit = %d, stderr = %q", exit, stderr.String())
	}

	// stdout must be a single JSON line that parses into the
	// documented shape.
	out := strings.TrimSpace(stdout.String())
	if strings.Count(out, "\n") != 0 {
		t.Fatalf("JSON output should be a single line, got:\n%s", out)
	}
	var got struct {
		Workdir    string         `json:"workdir"`
		LogPath    string         `json:"log_path"`
		Candidates map[string]int `json:"candidates"`
		ExtractLog struct {
			Exists       bool           `json:"exists"`
			TotalEntries int            `json:"total_entries"`
			ByStage      map[string]int `json:"by_stage"`
			LastEntry    string         `json:"last_entry"`
		} `json:"extract_log"`
	}
	if err := json.Unmarshal([]byte(out), &got); err != nil {
		t.Fatalf("output is not valid JSON: %v\nstdout: %s", err, out)
	}
	if got.Workdir != workDir {
		t.Fatalf("workdir = %q, want %q", got.Workdir, workDir)
	}
	if got.Candidates["open"] != 0 {
		t.Fatalf("candidates.open = %d, want 0", got.Candidates["open"])
	}
	if got.Candidates["drafted_linked"] != 1 {
		t.Fatalf("candidates.drafted_linked = %d, want 1", got.Candidates["drafted_linked"])
	}
	if got.Candidates["drafted_orphan"] != 0 {
		t.Fatalf("candidates.drafted_orphan = %d, want 0", got.Candidates["drafted_orphan"])
	}
	if got.Candidates["dismissed"] != 0 {
		t.Fatalf("candidates.dismissed = %d, want 0", got.Candidates["dismissed"])
	}
	// OpenRuntime creates the log file in append mode, so by the
	// time `summary` runs the file exists but is empty. exists is
	// thus the freshness signal "the runtime opened a log handle in
	// this workdir", not "any failure has been recorded"; the
	// latter is total_entries > 0.
	if !got.ExtractLog.Exists {
		t.Fatalf("extract_log.exists should be true (OpenRuntime creates the file in append mode); got %+v", got.ExtractLog)
	}
	if got.ExtractLog.TotalEntries != 0 {
		t.Fatalf("total_entries = %d, want 0 on a workdir that never failed", got.ExtractLog.TotalEntries)
	}
	if got.ExtractLog.LastEntry != "" {
		t.Fatalf("last_entry = %q, want empty string when no entries parsed", got.ExtractLog.LastEntry)
	}
}

func TestRunPersonaSummaryJSONWithLogEntries(t *testing.T) {
	// JSON mode with a seeded log file: total_entries, by_stage
	// breakdown, and last_entry should reflect the file content.
	configtest.IsolateHome(t)
	workDir := t.TempDir()

	// Open + close to materialize the state dir layout, then write
	// the log file directly.
	seedRuntime, err := app.OpenRuntimeWithConfigOptions(workDir, configtest.IsolatedOptions(t))
	if err != nil {
		t.Fatalf("seed OpenRuntime: %v", err)
	}
	logPath := seedRuntime.PersonaExtractLogPath()
	if err := seedRuntime.Close(); err != nil {
		t.Fatalf("seed Close: %v", err)
	}
	lines := []string{
		"2026-05-23T01:00:00Z\tstage=extract\tsession=s1\terror=\"timeout\"",
		"2026-05-23T02:00:00Z\tstage=parse_warning\tsession=s2\terror=\"low confidence\"",
		"2026-05-23T02:30:00Z\tstage=parse_warning\tsession=s3\terror=\"empty evidence\"",
	}
	if err := os.WriteFile(logPath, []byte(strings.Join(lines, "\n")+"\n"), 0o644); err != nil {
		t.Fatalf("write log fixture: %v", err)
	}

	var stdout, stderr bytes.Buffer
	exit := Run([]string{"persona", "summary", "--workdir", workDir, "--json"}, &bytes.Buffer{}, &stdout, &stderr, "test")
	if exit != 0 {
		t.Fatalf("exit = %d, stderr = %q", exit, stderr.String())
	}
	var got struct {
		ExtractLog struct {
			Exists       bool           `json:"exists"`
			TotalEntries int            `json:"total_entries"`
			ByStage      map[string]int `json:"by_stage"`
			LastEntry    string         `json:"last_entry"`
		} `json:"extract_log"`
	}
	if err := json.Unmarshal([]byte(strings.TrimSpace(stdout.String())), &got); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	if !got.ExtractLog.Exists {
		t.Fatalf("extract_log.exists should be true with seeded log; got %+v", got.ExtractLog)
	}
	if got.ExtractLog.TotalEntries != 3 {
		t.Fatalf("total_entries = %d, want 3", got.ExtractLog.TotalEntries)
	}
	if got.ExtractLog.ByStage["extract"] != 1 {
		t.Fatalf("by_stage.extract = %d, want 1", got.ExtractLog.ByStage["extract"])
	}
	if got.ExtractLog.ByStage["parse_warning"] != 2 {
		t.Fatalf("by_stage.parse_warning = %d, want 2", got.ExtractLog.ByStage["parse_warning"])
	}
	if got.ExtractLog.LastEntry != "2026-05-23T02:30:00Z" {
		t.Fatalf("last_entry = %q, want 2026-05-23T02:30:00Z", got.ExtractLog.LastEntry)
	}
}

func TestRunPersonaSummaryJSONStillExitsTwoWithOrphan(t *testing.T) {
	// JSON and --fail-on-orphan compose: stdout stays clean JSON
	// for jq, stderr carries the check-tripped hint, exit 2.
	configtest.IsolateHome(t)
	workDir := t.TempDir()
	forcePartialDraftedCandidateCLI(t, workDir, "major", "economics", "I major in economics")

	var stdout, stderr bytes.Buffer
	exit := Run([]string{"persona", "summary", "--workdir", workDir, "--json", "--fail-on-orphan"}, &bytes.Buffer{}, &stdout, &stderr, "test")
	if exit != 2 {
		t.Fatalf("exit = %d, want 2 with orphan + --fail-on-orphan + --json", exit)
	}
	var got struct {
		Candidates map[string]int `json:"candidates"`
	}
	if err := json.Unmarshal([]byte(strings.TrimSpace(stdout.String())), &got); err != nil {
		t.Fatalf("stdout JSON invalid (the check signal should NOT have polluted stdout): %v", err)
	}
	if got.Candidates["drafted_orphan"] != 1 {
		t.Fatalf("expected drafted_orphan=1, got %d", got.Candidates["drafted_orphan"])
	}
	if !strings.Contains(stderr.String(), "fail-on-orphan tripped") {
		t.Fatalf("stderr should still carry the check signal:\n%s", stderr.String())
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
