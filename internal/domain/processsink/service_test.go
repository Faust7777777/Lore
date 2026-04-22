package processsink

import (
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
