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
			{Timestamp: time.Date(2026, 4, 22, 10, 5, 0, 0, time.Local), Role: "user", Text: "first"},
			{Timestamp: time.Date(2026, 4, 22, 11, 5, 0, 0, time.Local), Role: "assistant", Text: "second"},
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
