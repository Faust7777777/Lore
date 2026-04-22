package codexjsonl

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestLoadFileParsesSessionEventsAndDedupes(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "rollout-abc.jsonl")
	content := "" +
		"{\"timestamp\":\"2026-04-22T09:00:00+08:00\",\"type\":\"session_meta\",\"payload\":{\"id\":\"session-1\",\"agent_nickname\":\"Parfit\"}}\n" +
		"{\"timestamp\":\"2026-04-22T09:01:00+08:00\",\"type\":\"event_msg\",\"payload\":{\"type\":\"user_message\",\"message\":\"build the adapter\"}}\n" +
		"{\"timestamp\":\"2026-04-22T09:01:00+08:00\",\"type\":\"response_item\",\"payload\":{\"type\":\"message\",\"role\":\"user\",\"content\":[{\"type\":\"input_text\",\"text\":\"build the adapter\"}]}}\n" +
		"{\"timestamp\":\"2026-04-22T09:05:00+08:00\",\"type\":\"event_msg\",\"payload\":{\"type\":\"agent_message\",\"phase\":\"commentary\",\"message\":\"reading files\"}}\n"
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	transcript, err := LoadFile(path)
	if err != nil {
		t.Fatalf("LoadFile() error = %v", err)
	}
	if transcript.SessionID != "rollout-abc" {
		t.Fatalf("SessionID = %q, want rollout-abc", transcript.SessionID)
	}
	if transcript.AgentID != "parfit" {
		t.Fatalf("AgentID = %q, want parfit", transcript.AgentID)
	}
	if len(transcript.Events) != 2 {
		t.Fatalf("len(Events) = %d, want 2", len(transcript.Events))
	}
}

func TestLoadFileKeepsRolloutSessionIDWhenParentMetaAppearsLater(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "rollout-current.jsonl")
	content := "" +
		"{\"timestamp\":\"2026-04-22T09:00:00+08:00\",\"type\":\"session_meta\",\"payload\":{\"id\":\"rollout-current\",\"agent_nickname\":\"Parfit\"}}\n" +
		"{\"timestamp\":\"2026-04-22T09:00:01+08:00\",\"type\":\"session_meta\",\"payload\":{\"id\":\"parent-thread\"}}\n" +
		"{\"timestamp\":\"2026-04-22T09:01:00+08:00\",\"type\":\"event_msg\",\"payload\":{\"type\":\"user_message\",\"message\":\"keep current session id\"}}\n"
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	transcript, err := LoadFile(path)
	if err != nil {
		t.Fatalf("LoadFile() error = %v", err)
	}
	if transcript.SessionID != "rollout-current" {
		t.Fatalf("SessionID = %q, want rollout-current", transcript.SessionID)
	}
}

func TestBuildWindowsFillsEmptyGapWithPlaceholderPayload(t *testing.T) {
	transcript := Transcript{
		AgentID:   "codex",
		SessionID: "session-1",
		Events: []Event{
			{Timestamp: time.Date(2026, 4, 22, 10, 5, 0, 0, time.Local), Role: "user", Text: "first", OffsetStart: 10},
			{Timestamp: time.Date(2026, 4, 22, 11, 5, 0, 0, time.Local), Role: "assistant", Text: "second", OffsetStart: 40},
		},
	}

	windows := BuildWindows(transcript, 30*time.Minute)
	if len(windows) != 3 {
		t.Fatalf("len(windows) = %d, want 3", len(windows))
	}
	if windows[1].Content != "" {
		t.Fatalf("middle window content = %q, want empty placeholder trigger", windows[1].Content)
	}
	if windows[1].EventCount != 0 {
		t.Fatalf("middle window EventCount = %d, want 0", windows[1].EventCount)
	}
	if windows[1].SourceOffset != 10 {
		t.Fatalf("middle window SourceOffset = %d, want 10", windows[1].SourceOffset)
	}
}

func TestLoadFileSupportsHistoryJSONL(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "history.jsonl")
	content := "{\"session_id\":\"session-1\",\"ts\":1774583565,\"text\":\"hi\"}\n"
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	transcript, err := LoadFile(path)
	if err != nil {
		t.Fatalf("LoadFile() error = %v", err)
	}
	if len(transcript.Events) != 1 {
		t.Fatalf("len(Events) = %d, want 1", len(transcript.Events))
	}
	if transcript.Events[0].Role != "user" {
		t.Fatalf("Role = %q, want user", transcript.Events[0].Role)
	}
}

func TestLoadTailConsumesOnlyCompleteLinesAndTracksOffsets(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "tail.jsonl")
	metaLine := `{"timestamp":"2026-04-22T09:00:00+08:00","type":"session_meta","payload":{"id":"session-tail","agent_nickname":"Codex"}}`
	firstLine := `{"timestamp":"2026-04-22T09:05:00+08:00","type":"event_msg","payload":{"type":"user_message","message":"first event"}}`
	secondLine := `{"timestamp":"2026-04-22T09:35:00+08:00","type":"event_msg","payload":{"type":"agent_message","phase":"commentary","message":"second event"}}`
	initial := metaLine + "\n" + firstLine + "\n" + secondLine
	if err := os.WriteFile(path, []byte(initial), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	transcript, nextOffset, err := LoadTail(path, 0)
	if err != nil {
		t.Fatalf("LoadTail() error = %v", err)
	}
	if len(transcript.Events) != 1 {
		t.Fatalf("len(Events) = %d, want 1", len(transcript.Events))
	}
	wantNextOffset := int64(len(metaLine) + 1 + len(firstLine) + 1)
	if nextOffset != wantNextOffset {
		t.Fatalf("nextOffset = %d, want %d", nextOffset, wantNextOffset)
	}
	if transcript.Events[0].OffsetStart != int64(len(metaLine)+1) {
		t.Fatalf("OffsetStart = %d, want %d", transcript.Events[0].OffsetStart, len(metaLine)+1)
	}
	if transcript.Events[0].OffsetEnd != wantNextOffset {
		t.Fatalf("OffsetEnd = %d, want %d", transcript.Events[0].OffsetEnd, wantNextOffset)
	}

	windows := BuildWindows(transcript, 30*time.Minute)
	if len(windows) != 1 {
		t.Fatalf("len(windows) = %d, want 1", len(windows))
	}
	if windows[0].SourceOffset != transcript.Events[0].OffsetStart {
		t.Fatalf("SourceOffset = %d, want %d", windows[0].SourceOffset, transcript.Events[0].OffsetStart)
	}

	if err := os.WriteFile(path, []byte(initial+"\n"), 0o644); err != nil {
		t.Fatalf("WriteFile() second error = %v", err)
	}
	tail, tailOffset, err := LoadTail(path, nextOffset)
	if err != nil {
		t.Fatalf("LoadTail(tail) error = %v", err)
	}
	if len(tail.Events) != 1 {
		t.Fatalf("len(tail.Events) = %d, want 1", len(tail.Events))
	}
	if tail.Events[0].OffsetStart != nextOffset {
		t.Fatalf("tail OffsetStart = %d, want %d", tail.Events[0].OffsetStart, nextOffset)
	}
	if tailOffset != int64(len(initial)+1) {
		t.Fatalf("tailOffset = %d, want %d", tailOffset, len(initial)+1)
	}
}
