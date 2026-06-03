package processsink

import (
	"strings"
	"testing"
	"time"

	"obsidian-harness/internal/model"
	"obsidian-harness/internal/store/memory"
)

type memoryWriter struct {
	files map[string]string
}

func newMemoryWriter() *memoryWriter {
	return &memoryWriter{files: make(map[string]string)}
}

func (w *memoryWriter) Write(path string, content []byte) error {
	w.files[path] = string(content)
	return nil
}

func TestWriteCheckpointUpsertsByWindowKey(t *testing.T) {
	st := memory.New()
	writer := newMemoryWriter()
	svc := NewService(st.ProcessSink(), writer, "process-sink")
	now := time.Date(2026, 4, 22, 12, 0, 0, 0, time.UTC)

	window := model.SessionWindow{
		AgentID:     "codex",
		SessionID:   "s1",
		WindowStart: now,
		WindowEnd:   now.Add(30 * time.Minute),
	}

	first, err := svc.WriteCheckpoint(window, "checkpoint", "first summary", "raw-1", now)
	if err != nil {
		t.Fatalf("WriteCheckpoint() error = %v", err)
	}
	second, err := svc.WriteCheckpoint(window, "checkpoint", "updated summary", "raw-2", now.Add(10*time.Minute))
	if err != nil {
		t.Fatalf("WriteCheckpoint() second error = %v", err)
	}

	if second.WindowKey != first.WindowKey {
		t.Fatalf("WindowKey changed: %q != %q", second.WindowKey, first.WindowKey)
	}
	if second.CreatedAt != first.CreatedAt {
		t.Fatalf("CreatedAt changed on upsert")
	}
	if second.Content != "updated summary" {
		t.Fatalf("Content = %q, want updated summary", second.Content)
	}
	if len(writer.files) != 1 {
		t.Fatalf("expected one written file, got %d", len(writer.files))
	}
}

func TestRollupDayProducesStableSingleReportPerAgentDay(t *testing.T) {
	st := memory.New()
	writer := newMemoryWriter()
	svc := NewService(st.ProcessSink(), writer, "process-sink")
	day := time.Date(2026, 4, 22, 9, 0, 0, 0, time.UTC)

	_, err := svc.WriteCheckpoint(model.SessionWindow{
		AgentID:     "codex",
		SessionID:   "s1",
		WindowStart: day,
		WindowEnd:   day.Add(30 * time.Minute),
	}, "morning", "morning summary", "", day)
	if err != nil {
		t.Fatalf("WriteCheckpoint() error = %v", err)
	}

	first, err := svc.RollupDay("codex", day, day.Add(12*time.Hour))
	if err != nil {
		t.Fatalf("RollupDay() error = %v", err)
	}
	second, err := svc.RollupDay("codex", day, day.Add(13*time.Hour))
	if err != nil {
		t.Fatalf("RollupDay() second error = %v", err)
	}

	if first.Path != second.Path {
		t.Fatalf("Path changed: %q != %q", second.Path, first.Path)
	}
	if first.CreatedAt != second.CreatedAt {
		t.Fatalf("CreatedAt changed on daily upsert")
	}
	if len(second.WindowKeys) != 1 {
		t.Fatalf("WindowKeys = %d, want 1", len(second.WindowKeys))
	}
}

func TestRollupDayWithSummaryOverridesGeneratedContent(t *testing.T) {
	// RollupDayWithSummary feeds an LLM-produced title/content that must
	// override the auto-generated "<agent> daily report" title and the
	// checkpoint-list body RollupDay falls back to -- while still
	// collecting the day's checkpoint window keys. The orchestrator
	// calls this on the summarized path (harness.go); the plain RollupDay
	// test only covers the fallback, leaving this override unguarded.
	st := memory.New()
	writer := newMemoryWriter()
	svc := NewService(st.ProcessSink(), writer, "process-sink")
	day := time.Date(2026, 4, 22, 9, 0, 0, 0, time.UTC)

	if _, err := svc.WriteCheckpoint(model.SessionWindow{
		AgentID:     "codex",
		SessionID:   "s1",
		WindowStart: day,
		WindowEnd:   day.Add(30 * time.Minute),
	}, "morning", "morning summary", "", day); err != nil {
		t.Fatalf("WriteCheckpoint() error = %v", err)
	}

	report, err := svc.RollupDayWithSummary("codex", day, "Big Day", "wrote the parser and shipped it", day.Add(12*time.Hour))
	if err != nil {
		t.Fatalf("RollupDayWithSummary() error = %v", err)
	}
	if report.Title != "Big Day" {
		t.Fatalf("Title = %q, want the provided summary title", report.Title)
	}
	if report.Content != "wrote the parser and shipped it" {
		t.Fatalf("Content = %q, want the provided summary content (not the checkpoint list)", report.Content)
	}
	// Override replaces the body, but checkpoint window keys are still
	// gathered from the day's checkpoints.
	if len(report.WindowKeys) != 1 {
		t.Fatalf("WindowKeys = %d, want 1 (checkpoints still collected)", len(report.WindowKeys))
	}
}

func TestWriteCheckpointEmptyContentBecomesPlaceholder(t *testing.T) {
	// A checkpoint written with blank content (no transcript ingested
	// for the window) must downgrade to the placeholder state with a
	// self-describing body naming the window, not persist an empty doc.
	st := memory.New()
	writer := newMemoryWriter()
	svc := NewService(st.ProcessSink(), writer, "process-sink")
	start := time.Date(2026, 4, 22, 14, 0, 0, 0, time.UTC)
	window := model.SessionWindow{
		AgentID:     "codex",
		SessionID:   "s1",
		WindowStart: start,
		WindowEnd:   start.Add(30 * time.Minute),
	}

	doc, err := svc.WriteCheckpoint(window, "", "   ", "raw", start)
	if err != nil {
		t.Fatalf("WriteCheckpoint() error = %v", err)
	}
	if doc.State != model.CheckpointPlaceholder {
		t.Fatalf("State = %q, want %q for empty content", doc.State, model.CheckpointPlaceholder)
	}
	if !strings.Contains(doc.Content, "no incremental transcript was ingested for 14:00-14:30") {
		t.Fatalf("placeholder content = %q, want the no-transcript message naming the window", doc.Content)
	}
}
