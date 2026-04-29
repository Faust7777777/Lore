package app

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"obsidian-harness/internal/model"
)

func TestRuntimeImportExternalTranscriptJSONLWritesCheckpointsAndRollup(t *testing.T) {
	loc := useFixedLocalZone(t)
	workDir := t.TempDir()
	transcriptPath := filepath.Join(workDir, "class-session.jsonl")
	content := "" +
		"{\"type\":\"session_meta\",\"agent_id\":\"Claude Code\",\"session_id\":\"class-1\"}\n" +
		"{\"timestamp\":\"2026-04-22T09:05:00+08:00\",\"role\":\"user\",\"text\":\"summarize today's e-commerce lecture\"}\n" +
		"{\"timestamp\":\"2026-04-22T09:35:00+08:00\",\"role\":\"assistant\",\"phase\":\"final\",\"text\":\"platform strategy and network effects\"}\n"
	if err := os.WriteFile(transcriptPath, []byte(content), 0o644); err != nil {
		t.Fatalf("WriteFile(transcript) error = %v", err)
	}

	runtime, err := openRuntimeWithFakeProcessSinkSummarizer(t, workDir)
	if err != nil {
		t.Fatalf("OpenRuntime() error = %v", err)
	}

	result, err := runtime.ImportExternalTranscriptJSONL(ImportExternalTranscriptJSONLParams{
		InputPath: transcriptPath,
	}, time.Date(2026, 4, 22, 23, 30, 0, 0, loc))
	if err != nil {
		t.Fatalf("ImportExternalTranscriptJSONL() error = %v", err)
	}

	if result.AgentID != "claude-code" {
		t.Fatalf("AgentID = %q, want claude-code", result.AgentID)
	}
	if result.SessionID != "class-1" {
		t.Fatalf("SessionID = %q, want class-1", result.SessionID)
	}
	if len(result.Checkpoints) != 2 {
		t.Fatalf("len(Checkpoints) = %d, want 2", len(result.Checkpoints))
	}
	if len(result.Reports) != 1 {
		t.Fatalf("len(Reports) = %d, want 1", len(result.Reports))
	}
	if result.Reports[0].AgentID != "claude-code" {
		t.Fatalf("Report.AgentID = %q, want claude-code", result.Reports[0].AgentID)
	}
	if !result.Reports[0].ReportDay.Equal(model.NormalizeDay(time.Date(2026, 4, 22, 12, 0, 0, 0, loc))) {
		t.Fatalf("ReportDay = %s, want 2026-04-22 local", result.Reports[0].ReportDay.Format(time.RFC3339))
	}

	checkpointMarkdown := readFile(t, result.Checkpoints[0].Path)
	if !strings.Contains(checkpointMarkdown, "summarize today's e-commerce lecture") {
		t.Fatalf("checkpoint markdown missing transcript content: %q", checkpointMarkdown)
	}
	reportMarkdown := readFile(t, result.Reports[0].Path)
	if !strings.Contains(reportMarkdown, "claude-code daily report") {
		t.Fatalf("daily report markdown missing external agent id: %q", reportMarkdown)
	}
}

func TestRuntimeImportExternalTranscriptJSONLRequiresInput(t *testing.T) {
	runtime, err := openRuntimeWithFakeProcessSinkSummarizer(t, t.TempDir())
	if err != nil {
		t.Fatalf("OpenRuntime() error = %v", err)
	}

	_, err = runtime.ImportExternalTranscriptJSONL(ImportExternalTranscriptJSONLParams{}, time.Date(2026, 4, 22, 23, 30, 0, 0, time.Local))
	if err == nil || !strings.Contains(err.Error(), "empty input path") {
		t.Fatalf("ImportExternalTranscriptJSONL() error = %v, want empty input path", err)
	}
}
