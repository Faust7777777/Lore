package externaljsonl

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadFileParsesGenericExternalTranscript(t *testing.T) {
	path := filepath.Join(t.TempDir(), "class-session.jsonl")
	content := "" +
		"{\"type\":\"session_meta\",\"agent_id\":\"Claude Code\",\"session_id\":\"class-1\"}\n" +
		"{\"timestamp\":\"2026-04-22T09:05:00+08:00\",\"role\":\"user\",\"text\":\"summarize today's e-commerce lecture\"}\n" +
		"{\"timestamp\":\"2026-04-22T09:35:00+08:00\",\"role\":\"assistant\",\"phase\":\"final\",\"text\":\"platform strategy and network effects\"}\n"
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	transcript, err := LoadFile(path)
	if err != nil {
		t.Fatalf("LoadFile() error = %v", err)
	}

	if transcript.AgentID != "claude-code" {
		t.Fatalf("AgentID = %q, want claude-code", transcript.AgentID)
	}
	if transcript.SessionID != "class-1" {
		t.Fatalf("SessionID = %q, want class-1", transcript.SessionID)
	}
	if len(transcript.Events) != 2 {
		t.Fatalf("len(Events) = %d, want 2", len(transcript.Events))
	}
	if transcript.Events[0].Role != "user" || transcript.Events[0].Text != "summarize today's e-commerce lecture" {
		t.Fatalf("Events[0] = %+v, want user lecture request", transcript.Events[0])
	}
	if transcript.Events[1].Role != "assistant" || transcript.Events[1].Phase != "final" || transcript.Events[1].Text != "platform strategy and network effects" {
		t.Fatalf("Events[1] = %+v, want assistant final response", transcript.Events[1])
	}
}

func TestLoadFileRejectsUnsupportedRole(t *testing.T) {
	path := filepath.Join(t.TempDir(), "bad-role.jsonl")
	content := "{\"timestamp\":\"2026-04-22T09:05:00+08:00\",\"role\":\"tool\",\"text\":\"hidden tool data\"}\n"
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	if _, err := LoadFile(path); err == nil {
		t.Fatal("LoadFile() error = nil, want unsupported role error")
	}
}

func TestLoadFileKeepsFirstSessionMetaIdentity(t *testing.T) {
	path := filepath.Join(t.TempDir(), "multi-meta.jsonl")
	content := "" +
		"{\"type\":\"session_meta\",\"agent_id\":\"Claude Code\",\"session_id\":\"class-1\"}\n" +
		"{\"type\":\"session_meta\",\"agent_id\":\"Gemini CLI\",\"session_id\":\"class-2\"}\n" +
		"{\"timestamp\":\"2026-04-22T09:05:00+08:00\",\"role\":\"user\",\"text\":\"keep first identity\"}\n"
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	transcript, err := LoadFile(path)
	if err != nil {
		t.Fatalf("LoadFile() error = %v", err)
	}

	if transcript.AgentID != "claude-code" {
		t.Fatalf("AgentID = %q, want first session_meta agent", transcript.AgentID)
	}
	if transcript.SessionID != "class-1" {
		t.Fatalf("SessionID = %q, want first session_meta session", transcript.SessionID)
	}
}
