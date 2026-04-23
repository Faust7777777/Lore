package app

import (
	"bytes"
	"context"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/fsnotify/fsnotify"

	"obsidian-harness/internal/vault"
)

func TestRuntimeScanVaultChangesPrimesThenCreatesDraft(t *testing.T) {
	workDir := t.TempDir()
	runtime, err := OpenRuntime(workDir)
	if err != nil {
		t.Fatalf("OpenRuntime() error = %v", err)
	}

	now := time.Date(2026, 4, 23, 9, 0, 0, 0, time.Local)
	if _, err := runtime.Bootstrap(now); err != nil {
		t.Fatalf("Bootstrap() error = %v", err)
	}

	relPath := filepath.Join("0-排期", "04-执行", "week.md")
	absPath := filepath.Join(runtime.Config.Paths.VaultRoot, relPath)
	writeTestPlan(t, runtime, absPath, "# Week\n\n- [ ] first pass", now.Add(-2*time.Second))

	first, err := runtime.ScanVaultChanges(now)
	if err != nil {
		t.Fatalf("ScanVaultChanges(first) error = %v", err)
	}
	if first.TriggeredDrafts != 0 {
		t.Fatalf("first.TriggeredDrafts = %d, want 0 during priming", first.TriggeredDrafts)
	}
	if first.Primed == 0 {
		t.Fatalf("first.Primed = %d, want at least 1", first.Primed)
	}

	secondNow := now.Add(5 * time.Second)
	writeTestPlan(t, runtime, absPath, "# Week\n\n- [x] second pass", secondNow.Add(-2*time.Second))

	second, err := runtime.ScanVaultChanges(secondNow)
	if err != nil {
		t.Fatalf("ScanVaultChanges(second) error = %v", err)
	}
	if second.TriggeredDrafts != 1 {
		t.Fatalf("second.TriggeredDrafts = %d, want 1", second.TriggeredDrafts)
	}

	drafts, err := runtime.ListDrafts()
	if err != nil {
		t.Fatalf("ListDrafts() error = %v", err)
	}
	if len(drafts) != 1 {
		t.Fatalf("len(drafts) = %d, want 1", len(drafts))
	}
	if got := drafts[0].EvidenceRefs[0]; got != "0-排期/04-执行/week.md" {
		t.Fatalf("EvidenceRefs[0] = %q, want normalized week path", got)
	}
}

func TestRuntimeScanVaultChangesWaitsForDebounceBeforeCreatingDraft(t *testing.T) {
	workDir := t.TempDir()
	runtime, err := OpenRuntime(workDir)
	if err != nil {
		t.Fatalf("OpenRuntime() error = %v", err)
	}
	runtime.Config.Vault.DebounceWindow = 500 * time.Millisecond

	now := time.Date(2026, 4, 23, 9, 0, 0, 0, time.Local)
	if _, err := runtime.Bootstrap(now); err != nil {
		t.Fatalf("Bootstrap() error = %v", err)
	}

	relPath := filepath.Join("0-排期", "04-执行", "week.md")
	absPath := filepath.Join(runtime.Config.Paths.VaultRoot, relPath)
	writeTestPlan(t, runtime, absPath, "# Week\n\n- [ ] initial", now.Add(-2*time.Second))

	if _, err := runtime.ScanVaultChanges(now); err != nil {
		t.Fatalf("ScanVaultChanges(prime) error = %v", err)
	}

	unstableNow := now.Add(2 * time.Second)
	writeTestPlan(t, runtime, absPath, "# Week\n\n- [x] updated", unstableNow.Add(-100*time.Millisecond))

	pending, err := runtime.ScanVaultChanges(unstableNow)
	if err != nil {
		t.Fatalf("ScanVaultChanges(pending) error = %v", err)
	}
	if pending.Pending != 1 {
		t.Fatalf("pending.Pending = %d, want 1", pending.Pending)
	}
	if pending.TriggeredDrafts != 0 {
		t.Fatalf("pending.TriggeredDrafts = %d, want 0", pending.TriggeredDrafts)
	}

	stable, err := runtime.ScanVaultChanges(unstableNow.Add(time.Second))
	if err != nil {
		t.Fatalf("ScanVaultChanges(stable) error = %v", err)
	}
	if stable.TriggeredDrafts != 1 {
		t.Fatalf("stable.TriggeredDrafts = %d, want 1", stable.TriggeredDrafts)
	}
}

func writeTestPlan(t *testing.T, runtime *Runtime, absPath string, content string, modTime time.Time) {
	t.Helper()

	if _, err := vault.WriteFileAtomic(absPath, []byte(content), runtime.Config.Vault.TempSuffix); err != nil {
		t.Fatalf("WriteFileAtomic(%s) error = %v", absPath, err)
	}
	if err := os.Chtimes(absPath, modTime, modTime); err != nil {
		t.Fatalf("Chtimes(%s) error = %v", absPath, err)
	}
}

func TestVaultDaemonScanSummaryIncludesDraftIDs(t *testing.T) {
	var output strings.Builder

	writeDaemonScanSummary(&output, VaultDaemonScanResult{
		ScannedPlanDocs: 2,
		Primed:          1,
		Pending:         0,
		TriggeredDrafts: 1,
		DraftIDs:        []string{"draft-1"},
	})

	text := output.String()
	for _, expected := range []string{
		"Vault daemon scan",
		"scanned plan docs: 2",
		"triggered drafts: 1",
		"draft ids: draft-1",
	} {
		if !strings.Contains(text, expected) {
			t.Fatalf("expected summary to contain %q, got %q", expected, text)
		}
	}
}

func TestRunVaultDaemonContinuesAfterCodexSyncFailure(t *testing.T) {
	workDir := t.TempDir()
	runtime, err := OpenRuntime(workDir)
	if err != nil {
		t.Fatalf("OpenRuntime() error = %v", err)
	}

	var output lockedBuffer
	err = runtime.RunVaultDaemon(context.Background(), VaultDaemonRunOptions{
		Once:   true,
		Stdout: &output,
		CodexJSONL: &ImportCodexJSONLParams{
			InputPath: filepath.Join(workDir, "missing.jsonl"),
			AgentID:   "codex",
			SessionID: "session-missing",
		},
	})
	if err != nil {
		t.Fatalf("RunVaultDaemon() error = %v, want non-fatal sync failure", err)
	}
	text := output.String()
	for _, expected := range []string{
		"Vault daemon running",
		"Vault daemon scan",
		"Codex JSONL sync failed",
	} {
		if !strings.Contains(text, expected) {
			t.Fatalf("expected daemon output to contain %q, got %q", expected, text)
		}
	}
}

func TestRunVaultDaemonKeepsRunningAfterCodexSyncFailureUntilCancel(t *testing.T) {
	workDir := t.TempDir()
	runtime, err := OpenRuntime(workDir)
	if err != nil {
		t.Fatalf("OpenRuntime() error = %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	var output lockedBuffer
	errCh := make(chan error, 1)
	go func() {
		errCh <- runtime.RunVaultDaemon(ctx, VaultDaemonRunOptions{
			PollEvery: 200 * time.Millisecond,
			Stdout:    &output,
			CodexJSONL: &ImportCodexJSONLParams{
				InputPath: filepath.Join(workDir, "missing.jsonl"),
				AgentID:   "codex",
				SessionID: "session-missing-loop",
			},
		})
	}()

	waitForCondition(t, 2*time.Second, func() bool {
		text := output.String()
		return strings.Contains(text, "Vault daemon running") && strings.Contains(text, "Codex JSONL sync failed")
	})

	time.Sleep(300 * time.Millisecond)
	select {
	case err := <-errCh:
		t.Fatalf("RunVaultDaemon() exited early with error = %v, want it to keep running until cancel", err)
	default:
	}

	cancel()
	select {
	case err := <-errCh:
		if err != nil {
			t.Fatalf("RunVaultDaemon() error after cancel = %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("RunVaultDaemon() did not stop after cancel")
	}
}

func TestRunVaultDaemonWatcherCreatesDraftAfterFileChange(t *testing.T) {
	workDir := t.TempDir()
	runtime, err := OpenRuntime(workDir)
	if err != nil {
		t.Fatalf("OpenRuntime() error = %v", err)
	}
	runtime.Config.Vault.DebounceWindow = 50 * time.Millisecond

	now := time.Date(2026, 4, 23, 9, 0, 0, 0, time.Local)
	if _, err := runtime.Bootstrap(now); err != nil {
		t.Fatalf("Bootstrap() error = %v", err)
	}

	relPath := filepath.Join("0-éŽºæŽ“æ¹¡", "04-éŽµÑ†î”‘", "week.md")
	absPath := filepath.Join(runtime.Config.Paths.VaultRoot, relPath)
	writeTestPlan(t, runtime, absPath, "# Week\n\n- [ ] initial", now.Add(-time.Second))
	if _, err := runtime.ScanVaultChanges(now); err != nil {
		t.Fatalf("ScanVaultChanges(prime) error = %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	var output lockedBuffer
	errCh := make(chan error, 1)
	go func() {
		errCh <- runtime.RunVaultDaemon(ctx, VaultDaemonRunOptions{
			PollEvery:  250 * time.Millisecond,
			Stdout:     &output,
			CodexJSONL: nil,
		})
	}()

	waitForCondition(t, 2*time.Second, func() bool {
		return strings.Contains(output.String(), "Vault watcher active")
	})

	writeTestPlan(t, runtime, absPath, "# Week\n\n- [x] watcher update", time.Now())
	waitForCondition(t, 2*time.Second, func() bool {
		drafts, err := runtime.ListDrafts()
		return err == nil && len(drafts) == 1
	})

	cancel()
	select {
	case err := <-errCh:
		if err != nil {
			t.Fatalf("RunVaultDaemon() error = %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("RunVaultDaemon() did not stop after cancel")
	}
}

func TestRunVaultDaemonWatcherIgnoresObsidianDirectory(t *testing.T) {
	workDir := t.TempDir()
	runtime, err := OpenRuntime(workDir)
	if err != nil {
		t.Fatalf("OpenRuntime() error = %v", err)
	}
	runtime.Config.Vault.DebounceWindow = 50 * time.Millisecond

	now := time.Date(2026, 4, 23, 9, 0, 0, 0, time.Local)
	if _, err := runtime.Bootstrap(now); err != nil {
		t.Fatalf("Bootstrap() error = %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	var output bytes.Buffer
	errCh := make(chan error, 1)
	go func() {
		errCh <- runtime.RunVaultDaemon(ctx, VaultDaemonRunOptions{
			PollEvery: 250 * time.Millisecond,
			Stdout:    &output,
		})
	}()

	waitForCondition(t, 2*time.Second, func() bool {
		return strings.Contains(output.String(), "Vault watcher active")
	})

	ignoredPath := filepath.Join(runtime.Config.Paths.VaultRoot, ".obsidian", "04-éŽµÑ†î”‘", "week.md")
	writeTestPlan(t, runtime, ignoredPath, "# Week\n\n- [x] ignored", time.Now())
	time.Sleep(250 * time.Millisecond)

	drafts, err := runtime.ListDrafts()
	if err != nil {
		t.Fatalf("ListDrafts() error = %v", err)
	}
	if len(drafts) != 0 {
		t.Fatalf("len(drafts) = %d, want 0 for ignored watcher path", len(drafts))
	}

	cancel()
	select {
	case err := <-errCh:
		if err != nil {
			t.Fatalf("RunVaultDaemon() error = %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("RunVaultDaemon() did not stop after cancel")
	}
}

func TestRunVaultDaemonWatcherSyncsCodexJSONLBeforePoll(t *testing.T) {
	loc := useFixedLocalZone(t)
	workDir := t.TempDir()
	runtime, err := openRuntimeWithFakeProcessSinkSummarizer(workDir)
	if err != nil {
		t.Fatalf("OpenRuntime() error = %v", err)
	}
	runtime.Config.Vault.DebounceWindow = 50 * time.Millisecond

	transcriptPath := filepath.Join(workDir, "daemon-codex-watch.jsonl")
	writeCodexJSONL(t, transcriptPath,
		`{"timestamp":"2026-04-22T09:00:00+08:00","type":"session_meta","payload":{"id":"session-daemon-watch","agent_nickname":"Codex"}}`,
		`{"timestamp":"2026-04-22T09:05:00+08:00","type":"event_msg","payload":{"type":"user_message","message":"first watcher sync"}}`,
	)
	fakeWatcher := newFakeFileEventWatcher(transcriptPath)
	previousWatcherFactory := newSingleFileWatcherFunc
	newSingleFileWatcherFunc = func(path string) (fileEventWatcher, error) {
		return fakeWatcher, nil
	}
	t.Cleanup(func() {
		newSingleFileWatcherFunc = previousWatcherFactory
	})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	var output lockedBuffer
	errCh := make(chan error, 1)
	startedAt := time.Now()
	go func() {
		errCh <- runtime.RunVaultDaemon(ctx, VaultDaemonRunOptions{
			PollEvery: 5 * time.Second,
			Stdout:    &output,
			CodexJSONL: &ImportCodexJSONLParams{
				InputPath: transcriptPath,
				AgentID:   "codex",
				SessionID: "session-daemon-watch",
			},
		})
	}()

	waitForCondition(t, 2*time.Second, func() bool {
		return strings.Contains(output.String(), "Codex watcher active")
	})

	file, err := os.OpenFile(transcriptPath, os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		t.Fatalf("OpenFile(append) error = %v", err)
	}
	if _, err := file.WriteString("{\"timestamp\":\"2026-04-22T09:35:00+08:00\",\"type\":\"event_msg\",\"payload\":{\"type\":\"agent_message\",\"phase\":\"commentary\",\"message\":\"second watcher sync\"}}\n"); err != nil {
		file.Close()
		t.Fatalf("WriteString(append) error = %v", err)
	}
	if err := file.Close(); err != nil {
		t.Fatalf("Close(append) error = %v", err)
	}
	now := time.Now()
	if err := os.Chtimes(transcriptPath, now, now); err != nil {
		t.Fatalf("Chtimes(transcript) error = %v", err)
	}
	fakeWatcher.Emit(fsnotify.Event{Name: transcriptPath, Op: fsnotify.Write})

	waitForCondition(t, 4*time.Second, func() bool {
		return strings.Count(output.String(), "Codex JSONL synced") >= 2
	})

	cancel()
	select {
	case err := <-errCh:
		if err != nil {
			t.Fatalf("RunVaultDaemon() error = %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("RunVaultDaemon() did not stop after cancel")
	}

	view, err := runtime.ProcessSinkDay("codex", time.Date(2026, 4, 22, 12, 0, 0, 0, loc))
	if err != nil {
		t.Fatalf("ProcessSinkDay() error = %v", err)
	}
	foundWatcherWindow := false
	for _, checkpoint := range view.Checkpoints {
		if checkpoint.Window.WindowStart.Equal(time.Date(2026, 4, 22, 9, 30, 0, 0, loc)) {
			foundWatcherWindow = true
			break
		}
	}
	if !foundWatcherWindow {
		t.Fatalf("expected watcher-triggered 09:30 checkpoint, got %+v", view.Checkpoints)
	}

	if elapsed := time.Since(startedAt); elapsed >= 5*time.Second {
		t.Fatalf("daemon elapsed = %s, want watcher-triggered codex sync before poll interval", elapsed)
	}
}

type fakeFileEventWatcher struct {
	path   string
	events chan fsnotify.Event
	errors chan error
}

func newFakeFileEventWatcher(path string) *fakeFileEventWatcher {
	return &fakeFileEventWatcher{
		path:   path,
		events: make(chan fsnotify.Event, 8),
		errors: make(chan error, 1),
	}
}

func (f *fakeFileEventWatcher) Events() <-chan fsnotify.Event {
	return f.events
}

func (f *fakeFileEventWatcher) Errors() <-chan error {
	return f.errors
}

func (f *fakeFileEventWatcher) Close() error {
	close(f.events)
	close(f.errors)
	return nil
}

func (f *fakeFileEventWatcher) Matches(event fsnotify.Event) bool {
	return sameWatchPath(event.Name, f.path)
}

func (f *fakeFileEventWatcher) Emit(event fsnotify.Event) {
	f.events <- event
}

func waitForCondition(t *testing.T, timeout time.Duration, condition func() bool) {
	t.Helper()

	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if condition() {
			return
		}
		time.Sleep(25 * time.Millisecond)
	}
	t.Fatalf("condition was not met within %s", timeout)
}

type lockedBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (b *lockedBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Write(p)
}

func (b *lockedBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.String()
}

var _ io.Writer = (*lockedBuffer)(nil)
