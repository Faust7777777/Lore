package main

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/fsnotify/fsnotify"

	"obsidian-harness/internal/app"
	"obsidian-harness/internal/cli"
)

func TestCodexAttachEventMatchesTargetPath(t *testing.T) {
	target := filepath.Clean(`C:\tmp\session.jsonl`)
	if !cli.CodexAttachEventMatches(fsnotify.Event{Name: `C:\tmp\session.jsonl`}, target) {
		t.Fatal("codexAttachEventMatches() = false, want true for same path")
	}
	if cli.CodexAttachEventMatches(fsnotify.Event{Name: `C:\tmp\other.jsonl`}, target) {
		t.Fatal("codexAttachEventMatches() = true, want false for different file")
	}
}

func TestRunCodexAttachLoopSyncsOnWatcherEventBeforePoll(t *testing.T) {
	workDir := t.TempDir()
	transcriptPath := filepath.Join(workDir, "session.jsonl")
	if err := os.WriteFile(transcriptPath, []byte("seed\n"), 0o644); err != nil {
		t.Fatalf("WriteFile(seed) error = %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	syncer := &countingCodexSyncer{
		onSync: func(count int) {
			if count >= 2 {
				cancel()
			}
		},
	}

	errCh := make(chan error, 1)
	startedAt := time.Now()
	go func() {
		errCh <- cli.RunCodexAttachLoop(ctx, syncer, app.ImportCodexJSONLParams{
			InputPath: transcriptPath,
			AgentID:   "codex",
			SessionID: "session-watch",
		}, 5*time.Second, io.Discard, io.Discard)
	}()

	time.Sleep(200 * time.Millisecond)
	if err := os.WriteFile(transcriptPath, []byte("seed\nnext\n"), 0o644); err != nil {
		t.Fatalf("WriteFile(update) error = %v", err)
	}

	select {
	case err := <-errCh:
		if err != nil {
			t.Fatalf("runCodexAttachLoop() error = %v", err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("runCodexAttachLoop() did not react to watcher event in time")
	}

	if elapsed := time.Since(startedAt); elapsed >= 5*time.Second {
		t.Fatalf("attach loop elapsed = %s, want watcher-triggered sync before poll interval", elapsed)
	}
	if got := syncer.Count(); got < 2 {
		t.Fatalf("sync count = %d, want at least 2 (initial + watcher-triggered)", got)
	}
}

type countingCodexSyncer struct {
	mu     sync.Mutex
	count  int
	onSync func(count int)
}

func (c *countingCodexSyncer) SyncCodexJSONLContext(_ context.Context, _ app.ImportCodexJSONLParams, _ time.Time) (app.SyncCodexJSONLResult, error) {
	c.mu.Lock()
	c.count++
	count := c.count
	onSync := c.onSync
	c.mu.Unlock()

	if onSync != nil {
		onSync(count)
	}
	return app.SyncCodexJSONLResult{
		Changed: count == 1 || count == 2,
		Import: app.ImportCodexJSONLResult{
			AgentID:   "codex",
			SessionID: "session-watch",
		},
	}, nil
}

func (c *countingCodexSyncer) Count() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.count
}
