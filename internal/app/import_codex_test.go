package app

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"obsidian-harness/internal/model"
)

func TestRuntimeImportCodexJSONLWritesPlaceholderCheckpointAndSingleDayRollup(t *testing.T) {
	loc := useFixedLocalZone(t)
	workDir := t.TempDir()
	transcriptPath := filepath.Join(workDir, "single-day.jsonl")
	writeCodexJSONL(t, transcriptPath,
		`{"timestamp":"2026-04-22T09:00:00+08:00","type":"session_meta","payload":{"id":"session-1","agent_nickname":"Codex"}}`,
		`{"timestamp":"2026-04-22T09:05:00+08:00","type":"event_msg","payload":{"type":"user_message","message":"review the weekly drift"}}`,
		`{"timestamp":"2026-04-22T10:05:00+08:00","type":"event_msg","payload":{"type":"agent_message","phase":"commentary","message":"drafted the next checkpoint"}}`,
	)

	runtime, err := OpenRuntime(workDir)
	if err != nil {
		t.Fatalf("OpenRuntime() error = %v", err)
	}

	result, err := runtime.ImportCodexJSONL(ImportCodexJSONLParams{
		InputPath: transcriptPath,
		AgentID:   "codex",
		SessionID: "session-1",
	}, time.Date(2026, 4, 22, 23, 45, 0, 0, loc))
	if err != nil {
		t.Fatalf("ImportCodexJSONL() error = %v", err)
	}

	if result.AgentID != "codex" {
		t.Fatalf("AgentID = %q, want codex", result.AgentID)
	}
	if result.SessionID != "session-1" {
		t.Fatalf("SessionID = %q, want session-1", result.SessionID)
	}
	if len(result.Checkpoints) != 3 {
		t.Fatalf("len(Checkpoints) = %d, want 3", len(result.Checkpoints))
	}

	placeholder := result.Checkpoints[1]
	if placeholder.State != model.CheckpointPlaceholder {
		t.Fatalf("placeholder State = %q, want %q", placeholder.State, model.CheckpointPlaceholder)
	}
	if !strings.Contains(placeholder.Content, "no incremental transcript was ingested") {
		t.Fatalf("placeholder Content = %q, want empty-window marker", placeholder.Content)
	}
	if placeholder.Window.WindowStart.Hour() != 9 || placeholder.Window.WindowStart.Minute() != 30 {
		t.Fatalf("placeholder window start = %s, want 09:30 local window", placeholder.Window.WindowStart.Format(time.RFC3339))
	}

	if len(result.Reports) != 1 {
		t.Fatalf("len(Reports) = %d, want 1", len(result.Reports))
	}
	report := result.Reports[0]
	wantDay := model.NormalizeDay(time.Date(2026, 4, 22, 12, 0, 0, 0, loc))
	if !report.ReportDay.Equal(wantDay) {
		t.Fatalf("ReportDay = %s, want %s", report.ReportDay.Format(time.RFC3339), wantDay.Format(time.RFC3339))
	}
	if len(report.WindowKeys) != 3 {
		t.Fatalf("len(report.WindowKeys) = %d, want 3", len(report.WindowKeys))
	}

	checkpointMarkdown := readFile(t, placeholder.Path)
	if !strings.Contains(checkpointMarkdown, "state: `placeholder`") {
		t.Fatalf("checkpoint markdown missing placeholder state: %q", checkpointMarkdown)
	}
	if !strings.Contains(checkpointMarkdown, "09:30-10:00") {
		t.Fatalf("checkpoint markdown missing empty window range: %q", checkpointMarkdown)
	}

	reportMarkdown := readFile(t, report.Path)
	if !strings.Contains(reportMarkdown, "checkpoints: 3") {
		t.Fatalf("daily report markdown missing checkpoint count: %q", reportMarkdown)
	}
	if !strings.Contains(reportMarkdown, "09:30") {
		t.Fatalf("daily report markdown missing placeholder window entry: %q", reportMarkdown)
	}
}

func TestRuntimeImportCodexJSONLRollsUpAcrossTouchedDays(t *testing.T) {
	loc := useFixedLocalZone(t)
	workDir := t.TempDir()
	transcriptPath := filepath.Join(workDir, "cross-day.jsonl")
	writeCodexJSONL(t, transcriptPath,
		`{"timestamp":"2026-04-22T23:40:00+08:00","type":"session_meta","payload":{"id":"session-cross","agent_nickname":"Codex"}}`,
		`{"timestamp":"2026-04-22T23:50:00+08:00","type":"event_msg","payload":{"type":"user_message","message":"wrap the evening session"}}`,
		`{"timestamp":"2026-04-23T00:10:00+08:00","type":"event_msg","payload":{"type":"agent_message","phase":"commentary","message":"started the next day handoff"}}`,
	)

	runtime, err := OpenRuntime(workDir)
	if err != nil {
		t.Fatalf("OpenRuntime() error = %v", err)
	}

	result, err := runtime.ImportCodexJSONL(ImportCodexJSONLParams{
		InputPath: transcriptPath,
		AgentID:   "codex",
		SessionID: "session-cross",
	}, time.Date(2026, 4, 23, 8, 30, 0, 0, loc))
	if err != nil {
		t.Fatalf("ImportCodexJSONL() error = %v", err)
	}

	if len(result.Checkpoints) != 2 {
		t.Fatalf("len(Checkpoints) = %d, want 2", len(result.Checkpoints))
	}
	if len(result.Reports) != 2 {
		t.Fatalf("len(Reports) = %d, want 2", len(result.Reports))
	}

	firstDay := model.NormalizeDay(time.Date(2026, 4, 22, 23, 0, 0, 0, loc))
	secondDay := model.NormalizeDay(time.Date(2026, 4, 23, 1, 0, 0, 0, loc))

	if !result.Reports[0].ReportDay.Equal(firstDay) {
		t.Fatalf("Reports[0].ReportDay = %s, want %s", result.Reports[0].ReportDay.Format(time.RFC3339), firstDay.Format(time.RFC3339))
	}
	if !result.Reports[1].ReportDay.Equal(secondDay) {
		t.Fatalf("Reports[1].ReportDay = %s, want %s", result.Reports[1].ReportDay.Format(time.RFC3339), secondDay.Format(time.RFC3339))
	}
	if len(result.Reports[0].WindowKeys) != 1 {
		t.Fatalf("len(Reports[0].WindowKeys) = %d, want 1", len(result.Reports[0].WindowKeys))
	}
	if len(result.Reports[1].WindowKeys) != 1 {
		t.Fatalf("len(Reports[1].WindowKeys) = %d, want 1", len(result.Reports[1].WindowKeys))
	}

	firstMarkdown := readFile(t, result.Reports[0].Path)
	if !strings.Contains(firstMarkdown, "23:30") {
		t.Fatalf("first daily report missing 23:30 entry: %q", firstMarkdown)
	}

	secondMarkdown := readFile(t, result.Reports[1].Path)
	if !strings.Contains(secondMarkdown, "00:00") {
		t.Fatalf("second daily report missing 00:00 entry: %q", secondMarkdown)
	}
}

func useFixedLocalZone(t *testing.T) *time.Location {
	t.Helper()

	previous := time.Local
	loc := time.FixedZone("UTC+8", 8*60*60)
	time.Local = loc
	t.Cleanup(func() {
		time.Local = previous
	})
	return loc
}

func writeCodexJSONL(t *testing.T, path string, lines ...string) {
	t.Helper()

	content := strings.Join(lines, "\n") + "\n"
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("WriteFile(%s) error = %v", path, err)
	}
}

func readFile(t *testing.T, path string) string {
	t.Helper()

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile(%s) error = %v", path, err)
	}
	return string(data)
}
