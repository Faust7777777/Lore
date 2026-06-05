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

	// Generous margins so the test is robust under heavy parallel CI load
	// (the watcher fires in ~200ms normally; the happy path stays sub-second).
	// The invariant being checked is "the watcher triggers the second sync
	// before the poll would": the poll interval (60s) is far larger than the
	// watcher latency, and the wait deadline (30s) is kept below the poll
	// interval so a genuinely broken watcher still fails (no second sync
	// arrives before the deadline).
	const pollEvery = 60 * time.Second
	const waitDeadline = 30 * time.Second

	errCh := make(chan error, 1)
	startedAt := time.Now()
	go func() {
		errCh <- cli.RunCodexAttachLoop(ctx, syncer, app.ImportCodexJSONLParams{
			InputPath: transcriptPath,
			AgentID:   "codex",
			SessionID: "session-watch",
		}, pollEvery, io.Discard, io.Discard)
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
	case <-time.After(waitDeadline):
		t.Fatal("runCodexAttachLoop() did not react to watcher event in time")
	}

	if elapsed := time.Since(startedAt); elapsed >= pollEvery {
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
