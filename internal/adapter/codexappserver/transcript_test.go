package codexappserver

import (
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestTranscriptFromThreadMapsItemsErrorsAndSyntheticSource(t *testing.T) {
	loc := useFixedLocalZone(t)

	firstTurnAt := time.Date(2026, 4, 22, 9, 5, 0, 0, loc)
	secondTurnAt := time.Date(2026, 4, 22, 9, 37, 0, 0, loc)
	transcript, err := TranscriptFromThread(Thread{
		ID:            "thread-42",
		ModelProvider: "Codex Pro",
		CreatedAt:     firstTurnAt.Unix(),
		Turns: []Turn{
			{
				ID:        "turn-1",
				CreatedAt: firstTurnAt.Unix(),
				Items: []Item{
					{Type: "user_message", Text: "inspect the repo"},
					{Type: "assistant_message", Phase: "commentary", Text: "checked git status"},
				},
			},
			{
				ID:        "turn-2",
				UpdatedAt: secondTurnAt.UnixMilli(),
				Items: []Item{
					{
						Type:  "assistant_message",
						Phase: "operate",
						Content: []ItemContent{
							{Type: "output_text", Text: "applied the patch"},
							{Type: "output_text", Text: "applied the patch"},
						},
					},
				},
				Error: &TurnError{Message: "waiting for user approval"},
			},
		},
	}, "")
	if err != nil {
		t.Fatalf("TranscriptFromThread() error = %v", err)
	}

	if transcript.AgentID != "codex" {
		t.Fatalf("AgentID = %q, want codex", transcript.AgentID)
	}
	if transcript.SessionID != "thread-42" {
		t.Fatalf("SessionID = %q, want thread-42", transcript.SessionID)
	}
	if filepath.Base(transcript.SourcePath) != "thread-42.jsonl" {
		t.Fatalf("SourcePath = %q, want synthetic thread path", transcript.SourcePath)
	}
	if len(transcript.Events) != 4 {
		t.Fatalf("len(Events) = %d, want 4", len(transcript.Events))
	}

	if transcript.Events[0].Role != "user" || transcript.Events[0].Text != "inspect the repo" {
		t.Fatalf("Events[0] = %+v, want user prompt", transcript.Events[0])
	}
	if transcript.Events[1].Role != "assistant" || transcript.Events[1].Phase != "commentary" {
		t.Fatalf("Events[1] = %+v, want assistant commentary", transcript.Events[1])
	}
	if transcript.Events[2].Role != "assistant" || transcript.Events[2].Phase != "operate" || transcript.Events[2].Text != "applied the patch" {
		t.Fatalf("Events[2] = %+v, want assistant output text", transcript.Events[2])
	}
	if transcript.Events[3].Role != "assistant" || transcript.Events[3].Phase != "error" || transcript.Events[3].Text != "waiting for user approval" {
		t.Fatalf("Events[3] = %+v, want synthesized turn error", transcript.Events[3])
	}
	for index, event := range transcript.Events {
		if !strings.Contains(event.RawLine, `"type":"event_msg"`) {
			t.Fatalf("Events[%d].RawLine = %q, want synthetic event payload", index, event.RawLine)
		}
		if event.OffsetEnd <= event.OffsetStart {
			t.Fatalf("Events[%d] offsets = [%d,%d), want increasing offsets", index, event.OffsetStart, event.OffsetEnd)
		}
	}
}

func TestTranscriptFromThreadUsesTurnFallbackRoleAndTimestampFallback(t *testing.T) {
	loc := useFixedLocalZone(t)
	firstTurnAt := time.Date(2026, 4, 22, 11, 0, 0, 0, loc)

	transcript, err := TranscriptFromThread(Thread{
		ID:        "thread-fallback",
		CreatedAt: firstTurnAt.Unix(),
		Turns: []Turn{
			{
				ID:        "turn-1",
				CreatedAt: firstTurnAt.Unix(),
				Items: []Item{
					{Type: "assistant_message", Text: "primary assistant output"},
					{Type: "message", Content: []ItemContent{{Type: "text", Text: "secondary detail"}}},
				},
			},
			{
				ID: "turn-2",
				Items: []Item{
					{Type: "assistant_message", Text: "missing timestamp still imports"},
				},
			},
		},
	}, "live/thread-fallback.jsonl")
	if err != nil {
		t.Fatalf("TranscriptFromThread() error = %v", err)
	}

	if transcript.SourcePath != filepath.Clean("live/thread-fallback.jsonl") {
		t.Fatalf("SourcePath = %q, want explicit source path", transcript.SourcePath)
	}
	if len(transcript.Events) != 3 {
		t.Fatalf("len(Events) = %d, want 3", len(transcript.Events))
	}
	if transcript.Events[1].Role != "assistant" || transcript.Events[1].Text != "secondary detail" {
		t.Fatalf("Events[1] = %+v, want fallback assistant role", transcript.Events[1])
	}
	if !transcript.Events[2].Timestamp.After(transcript.Events[1].Timestamp) {
		t.Fatalf("Events[2].Timestamp = %s, want monotonic fallback timestamp after %s", transcript.Events[2].Timestamp, transcript.Events[1].Timestamp)
	}
}

func TestTranscriptFromThreadRejectsMaterialTurnWithoutAnyTimestamp(t *testing.T) {
	_, err := TranscriptFromThread(Thread{
		ID: "thread-missing-time",
		Turns: []Turn{
			{
				ID: "turn-1",
				Items: []Item{
					{Type: "assistant_message", Text: "this should not be silently dropped"},
				},
			},
		},
	}, "")
	if err == nil || !strings.Contains(err.Error(), "no timestamp metadata") {
		t.Fatalf("TranscriptFromThread() error = %v, want missing timestamp rejection", err)
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
