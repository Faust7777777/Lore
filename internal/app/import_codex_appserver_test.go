package app

import (
	"fmt"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"obsidian-harness/internal/adapter/codexappserver"
	"obsidian-harness/internal/model"
)

type fakeCodexAppServerReader struct {
	thread        codexappserver.Thread
	err           error
	lastRead      codexappserver.ReadThreadParams
	readCallCount int
}

func (f *fakeCodexAppServerReader) ReadThread(params codexappserver.ReadThreadParams) (codexappserver.Thread, error) {
	f.lastRead = params
	f.readCallCount++
	if f.err != nil {
		return codexappserver.Thread{}, f.err
	}
	return f.thread, nil
}

func TestRuntimeImportCodexAppServerThreadWritesCheckpointsAndRollup(t *testing.T) {
	loc := useFixedLocalZone(t)
	workDir := t.TempDir()

	runtime, err := openRuntimeWithFakeProcessSinkSummarizer(workDir)
	if err != nil {
		t.Fatalf("OpenRuntime() error = %v", err)
	}

	firstTurnAt := time.Date(2026, 4, 22, 9, 5, 0, 0, loc)
	secondTurnAt := time.Date(2026, 4, 22, 10, 5, 0, 0, loc)
	result, err := runtime.ImportCodexAppServerThread(ImportCodexAppServerThreadParams{
		Thread: codexappserver.Thread{
			ID:        "thread-live",
			CreatedAt: firstTurnAt.Unix(),
			Turns: []codexappserver.Turn{
				{
					ID:        "turn-1",
					CreatedAt: firstTurnAt.Unix(),
					Items: []codexappserver.Item{
						{Type: "user_message", Text: "review the current milestone"},
						{Type: "assistant_message", Phase: "commentary", Text: "captured the current risks"},
					},
				},
				{
					ID:        "turn-2",
					CreatedAt: secondTurnAt.Unix(),
					Items: []codexappserver.Item{
						{Type: "assistant_message", Phase: "operate", Text: "prepared the next implementation slice"},
					},
				},
			},
		},
		AgentID:   "codex",
		SessionID: "session-live",
	}, time.Date(2026, 4, 22, 23, 45, 0, 0, loc))
	if err != nil {
		t.Fatalf("ImportCodexAppServerThread() error = %v", err)
	}

	if result.AgentID != "codex" {
		t.Fatalf("AgentID = %q, want codex", result.AgentID)
	}
	if result.SessionID != "session-live" {
		t.Fatalf("SessionID = %q, want session-live", result.SessionID)
	}
	if len(result.Checkpoints) != 3 {
		t.Fatalf("len(Checkpoints) = %d, want 3", len(result.Checkpoints))
	}
	if result.Checkpoints[1].State != model.CheckpointPlaceholder {
		t.Fatalf("Checkpoints[1].State = %q, want %q", result.Checkpoints[1].State, model.CheckpointPlaceholder)
	}
	if len(result.Reports) != 1 {
		t.Fatalf("len(Reports) = %d, want 1", len(result.Reports))
	}
	if len(result.Reports[0].WindowKeys) != 3 {
		t.Fatalf("len(Reports[0].WindowKeys) = %d, want 3", len(result.Reports[0].WindowKeys))
	}

	firstCheckpointMarkdown := readFile(t, result.Checkpoints[0].Path)
	if !strings.Contains(firstCheckpointMarkdown, "thread-live.jsonl") {
		t.Fatalf("first checkpoint markdown = %q, want synthetic source path label", firstCheckpointMarkdown)
	}
	if !strings.Contains(firstCheckpointMarkdown, "captured the current risks") {
		t.Fatalf("first checkpoint markdown = %q, want assistant content", firstCheckpointMarkdown)
	}

	reportMarkdown := readFile(t, result.Reports[0].Path)
	if !strings.Contains(reportMarkdown, "09:00") || !strings.Contains(reportMarkdown, "10:00") {
		t.Fatalf("daily report markdown = %q, want both touched windows", reportMarkdown)
	}
}

func TestRuntimeImportCodexAppServerThreadRespectsCustomSourcePath(t *testing.T) {
	loc := useFixedLocalZone(t)
	workDir := t.TempDir()

	runtime, err := openRuntimeWithFakeProcessSinkSummarizer(workDir)
	if err != nil {
		t.Fatalf("OpenRuntime() error = %v", err)
	}

	result, err := runtime.ImportCodexAppServerThread(ImportCodexAppServerThreadParams{
		Thread: codexappserver.Thread{
			ID:        "thread-custom-source",
			CreatedAt: time.Date(2026, 4, 22, 14, 0, 0, 0, loc).Unix(),
			Turns: []codexappserver.Turn{
				{
					ID:        "turn-1",
					CreatedAt: time.Date(2026, 4, 22, 14, 5, 0, 0, loc).Unix(),
					Items: []codexappserver.Item{
						{Type: "assistant_message", Text: "custom source label"},
					},
				},
			},
		},
		SourcePath: "live/codex/thread-custom-source.jsonl",
	}, time.Date(2026, 4, 22, 18, 0, 0, 0, loc))
	if err != nil {
		t.Fatalf("ImportCodexAppServerThread() error = %v", err)
	}

	if result.InputPath != filepath.Clean("live/codex/thread-custom-source.jsonl") {
		t.Fatalf("InputPath = %q, want explicit source path", result.InputPath)
	}
	if len(result.Checkpoints) != 1 {
		t.Fatalf("len(Checkpoints) = %d, want 1", len(result.Checkpoints))
	}
	markdown := readFile(t, result.Checkpoints[0].Path)
	if !strings.Contains(markdown, "thread-custom-source.jsonl") {
		t.Fatalf("checkpoint markdown = %q, want custom source label basename", markdown)
	}
}

func TestRuntimeImportCodexAppServerSourceReadsThreadViaClient(t *testing.T) {
	loc := useFixedLocalZone(t)
	workDir := t.TempDir()

	runtime, err := openRuntimeWithFakeProcessSinkSummarizer(workDir)
	if err != nil {
		t.Fatalf("OpenRuntime() error = %v", err)
	}

	reader := &fakeCodexAppServerReader{
		thread: codexappserver.Thread{
			ID:        "thread-from-reader",
			CreatedAt: time.Date(2026, 4, 22, 16, 0, 0, 0, loc).Unix(),
			Turns: []codexappserver.Turn{
				{
					ID:        "turn-1",
					CreatedAt: time.Date(2026, 4, 22, 16, 5, 0, 0, loc).Unix(),
					Items: []codexappserver.Item{
						{Type: "assistant_message", Text: "imported via reader"},
					},
				},
			},
		},
	}

	result, err := runtime.ImportCodexAppServerSource(reader, ImportCodexAppServerSourceParams{
		ThreadID:  "thread-from-reader",
		AgentID:   "codex",
		SessionID: "session-from-reader",
	}, time.Date(2026, 4, 22, 20, 0, 0, 0, loc))
	if err != nil {
		t.Fatalf("ImportCodexAppServerSource() error = %v", err)
	}

	if reader.readCallCount != 1 {
		t.Fatalf("readCallCount = %d, want 1", reader.readCallCount)
	}
	if reader.lastRead.ThreadID != "thread-from-reader" || !reader.lastRead.IncludeTurns {
		t.Fatalf("lastRead = %+v, want includeTurns thread read", reader.lastRead)
	}
	if result.AgentID != "codex" || result.SessionID != "session-from-reader" {
		t.Fatalf("result identity = (%q,%q), want (codex, session-from-reader)", result.AgentID, result.SessionID)
	}
	if len(result.Checkpoints) != 1 {
		t.Fatalf("len(Checkpoints) = %d, want 1", len(result.Checkpoints))
	}
}

func TestRuntimeImportCodexAppServerSourceRequiresThreadID(t *testing.T) {
	runtime, err := openRuntimeWithFakeProcessSinkSummarizer(t.TempDir())
	if err != nil {
		t.Fatalf("OpenRuntime() error = %v", err)
	}

	_, err = runtime.ImportCodexAppServerSource(&fakeCodexAppServerReader{}, ImportCodexAppServerSourceParams{}, time.Now())
	if err == nil || !strings.Contains(err.Error(), "thread id is required") {
		t.Fatalf("ImportCodexAppServerSource() error = %v, want missing thread id", err)
	}
}

func TestRuntimeImportCodexAppServerSourcePropagatesReaderError(t *testing.T) {
	runtime, err := openRuntimeWithFakeProcessSinkSummarizer(t.TempDir())
	if err != nil {
		t.Fatalf("OpenRuntime() error = %v", err)
	}

	_, err = runtime.ImportCodexAppServerSource(&fakeCodexAppServerReader{
		err: fmt.Errorf("boom"),
	}, ImportCodexAppServerSourceParams{
		ThreadID: "thread-err",
	}, time.Now())
	if err == nil || !strings.Contains(err.Error(), "boom") {
		t.Fatalf("ImportCodexAppServerSource() error = %v, want propagated reader error", err)
	}
}
