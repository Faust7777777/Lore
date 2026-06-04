package cli

import (
	"context"
	"fmt"
	"io"
	"path/filepath"
	"strings"
	"time"

	"github.com/fsnotify/fsnotify"

	"obsidian-harness/internal/app"
)

const codexAttachDebounce = 200 * time.Millisecond

type CodexJSONLSyncer interface {
	SyncCodexJSONLContext(ctx context.Context, params app.ImportCodexJSONLParams, now time.Time) (app.SyncCodexJSONLResult, error)
}

func RunCodexAttachLoop(ctx context.Context, syncer CodexJSONLSyncer, params app.ImportCodexJSONLParams, pollEvery time.Duration, stdout io.Writer, stderr io.Writer) error {
	if ctx == nil {
		ctx = context.Background()
	}
	if pollEvery <= 0 {
		pollEvery = 5 * time.Second
	}

	absoluteInputPath, err := filepath.Abs(filepath.Clean(params.InputPath))
	if err != nil {
		return err
	}
	params.InputPath = absoluteInputPath

	fmt.Fprintf(stdout, "Attaching Codex JSONL\n- input: %s\n- poll: %s\n", params.InputPath, pollEvery)
	if err := syncCodexAttachOnce(ctx, syncer, params, stdout); err != nil {
		return err
	}

	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		fmt.Fprintf(stdout, "Codex attach watcher unavailable\n- error: %s\n- mode: polling fallback\n", err)
	} else {
		watchDir := filepath.Dir(absoluteInputPath)
		if err := watcher.Add(watchDir); err != nil {
			watcher.Close()
			watcher = nil
			fmt.Fprintf(stdout, "Codex attach watcher unavailable\n- error: %s\n- mode: polling fallback\n", err)
		} else {
			defer watcher.Close()
			fmt.Fprintln(stdout, "Codex attach watcher active")
		}
	}

	ticker := time.NewTicker(pollEvery)
	defer ticker.Stop()

	var (
		debounceTimer *time.Timer
		debounceCh    <-chan time.Time
	)
	resetDebounce := func() {
		if debounceTimer == nil {
			debounceTimer = time.NewTimer(codexAttachDebounce)
			debounceCh = debounceTimer.C
			return
		}
		if !debounceTimer.Stop() {
			select {
			case <-debounceTimer.C:
			default:
			}
		}
		debounceTimer.Reset(codexAttachDebounce)
		debounceCh = debounceTimer.C
	}
	stopDebounce := func() {
		if debounceTimer == nil {
			return
		}
		if !debounceTimer.Stop() {
			select {
			case <-debounceTimer.C:
			default:
			}
		}
		debounceCh = nil
	}

	for {
		var (
			watchEvents <-chan fsnotify.Event
			watchErrors <-chan error
		)
		if watcher != nil {
			watchEvents = watcher.Events
			watchErrors = watcher.Errors
		}

		select {
		case <-ctx.Done():
			stopDebounce()
			fmt.Fprintln(stdout, "Attach stopped")
			return nil
		case event, ok := <-watchEvents:
			if !ok {
				watcher = nil
				continue
			}
			if !CodexAttachEventMatches(event, absoluteInputPath) {
				continue
			}
			if event.Op&(fsnotify.Create|fsnotify.Write|fsnotify.Rename|fsnotify.Chmod) == 0 {
				continue
			}
			resetDebounce()
		case err, ok := <-watchErrors:
			if !ok {
				watcher = nil
				continue
			}
			fmt.Fprintf(stdout, "Codex attach watcher error\n- error: %s\n- mode: polling fallback\n", err)
			if watcher != nil {
				watcher.Close()
			}
			watcher = nil
		case <-debounceCh:
			debounceCh = nil
			if err := syncCodexAttachOnce(ctx, syncer, params, stdout); err != nil {
				fmt.Fprintf(stderr, "attach-codex-jsonl: %v\n", err)
			}
		case <-ticker.C:
			if err := syncCodexAttachOnce(ctx, syncer, params, stdout); err != nil {
				fmt.Fprintf(stderr, "attach-codex-jsonl: %v\n", err)
			}
		}
	}
}

func syncCodexAttachOnce(ctx context.Context, syncer CodexJSONLSyncer, params app.ImportCodexJSONLParams, stdout io.Writer) error {
	result, err := syncer.SyncCodexJSONLContext(ctx, params, time.Now())
	if err != nil {
		return err
	}
	writeCodexAttachSummary(stdout, result)
	return nil
}

func writeCodexAttachSummary(stdout io.Writer, result app.SyncCodexJSONLResult) {
	if !result.Changed {
		fmt.Fprintln(stdout, "No changes")
		return
	}
	fmt.Fprintf(
		stdout,
		"Synced\n- agent: %s\n- session: %s\n- checkpoints: %d\n- daily reports: %d\n",
		result.Import.AgentID,
		result.Import.SessionID,
		len(result.Import.Checkpoints),
		len(result.Import.Reports),
	)
}

func CodexAttachEventMatches(event fsnotify.Event, absoluteInputPath string) bool {
	eventPath := filepath.Clean(event.Name)
	targetPath := filepath.Clean(absoluteInputPath)
	return strings.EqualFold(eventPath, targetPath)
}
