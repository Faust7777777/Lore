package app

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/fsnotify/fsnotify"

	"obsidian-harness/internal/model"
	"obsidian-harness/internal/store"
	"obsidian-harness/internal/vault"
)

type VaultDaemonScanResult struct {
	ScannedPlanDocs int
	Primed          int
	Pending         int
	TriggeredDrafts int
	DraftIDs        []string
}

type VaultDaemonRunOptions struct {
	PollEvery  time.Duration
	Once       bool
	Stdout     io.Writer
	CodexJSONL *ImportCodexJSONLParams
}

const codexWatchDebounce = 200 * time.Millisecond

func (r *Runtime) ScanVaultChanges(now time.Time) (VaultDaemonScanResult, error) {
	if _, err := r.Bootstrap(now); err != nil {
		return VaultDaemonScanResult{}, err
	}
	paths, err := vault.WalkMarkdownPaths(r.Config.Paths.VaultRoot, "")
	if err != nil {
		return VaultDaemonScanResult{}, err
	}
	result, _, err := r.scanVaultPaths(now, paths)
	return result, err
}

func (r *Runtime) scanVaultPaths(now time.Time, relPaths []string) (VaultDaemonScanResult, []string, error) {
	if _, err := r.Bootstrap(now); err != nil {
		return VaultDaemonScanResult{}, nil, err
	}

	result := VaultDaemonScanResult{
		DraftIDs: make([]string, 0),
	}
	pendingPaths := make([]string, 0)
	seen := make(map[string]struct{}, len(relPaths))

	for _, relPath := range relPaths {
		relPath = vault.NormalizeRelativePath(relPath)
		if relPath == "" || vault.ShouldIgnoreRelativePath(relPath) {
			continue
		}
		if strings.ToLower(filepath.Ext(relPath)) != ".md" {
			continue
		}
		if _, exists := seen[relPath]; exists {
			continue
		}
		seen[relPath] = struct{}{}

		docClass := r.Harness.DocClassify(relPath).DocClass
		if !isDaemonPlanClass(docClass) {
			continue
		}
		result.ScannedPlanDocs++

		absPath := filepath.Join(r.Config.Paths.VaultRoot, filepath.FromSlash(relPath))
		info, err := os.Stat(absPath)
		if err != nil {
			if os.IsNotExist(err) {
				continue
			}
			return VaultDaemonScanResult{}, nil, err
		}

		content, hash, normalizedPath, err := vault.ReadRelativeWithHash(r.Config.Paths.VaultRoot, relPath)
		if err != nil {
			if os.IsNotExist(err) {
				continue
			}
			return VaultDaemonScanResult{}, nil, err
		}

		cursorKey := vaultWatchCursorKey(normalizedPath)
		lastHash, err := r.Store.Cursors().GetCursor(cursorKey)
		if err != nil {
			if err != store.ErrNotFound {
				return VaultDaemonScanResult{}, nil, err
			}
			if err := r.Store.Cursors().SaveCursor(cursorKey, hash); err != nil {
				return VaultDaemonScanResult{}, nil, err
			}
			result.Primed++
			continue
		}

		if lastHash == hash {
			continue
		}
		if !changeStable(now, info.ModTime(), r.Config.Vault.DebounceWindow) {
			result.Pending++
			pendingPaths = append(pendingPaths, normalizedPath)
			continue
		}

		draft, err := r.Harness.ObserveDocumentChange(normalizedPath, content, now)
		if err != nil {
			return VaultDaemonScanResult{}, nil, err
		}
		if err := r.Store.Cursors().SaveCursor(cursorKey, hash); err != nil {
			return VaultDaemonScanResult{}, nil, err
		}
		result.TriggeredDrafts++
		result.DraftIDs = append(result.DraftIDs, draft.ID)
	}

	return result, pendingPaths, nil
}

func (r *Runtime) RunVaultDaemon(ctx context.Context, opts VaultDaemonRunOptions) error {
	if ctx == nil {
		ctx = context.Background()
	}
	if opts.PollEvery <= 0 {
		opts.PollEvery = 2 * time.Second
	}

	modelAvailable := r.ProcessSinkSummarizer != nil && r.processSinkSummarizerErr == nil
	r.Harness.UpdateDependencies(modelAvailable, false)
	writeDaemonLine(opts.Stdout, "Vault daemon running\n- sync tick: %s\n- debounce: %s\n", opts.PollEvery, r.Config.Vault.DebounceWindow)

	result, err := r.ScanVaultChanges(time.Now())
	if err != nil {
		return err
	}
	writeDaemonScanSummary(opts.Stdout, result)

	r.syncCodexJSONLIfConfigured(opts, modelAvailable)
	if opts.Once {
		return nil
	}

	watcher, err := newVaultWatcher(r.Config.Paths.VaultRoot)
	if err != nil {
		writeDaemonLine(opts.Stdout, "Vault watcher unavailable\n- error: %s\n- mode: polling fallback\n", err)
	} else {
		defer watcher.Close()
		writeDaemonLine(opts.Stdout, "Vault watcher active\n")
	}

	var codexWatcher fileEventWatcher
	if opts.CodexJSONL != nil && strings.TrimSpace(opts.CodexJSONL.InputPath) != "" {
		codexWatcher, err = newSingleFileWatcherFunc(opts.CodexJSONL.InputPath)
		if err != nil {
			writeDaemonLine(opts.Stdout, "Codex watcher unavailable\n- error: %s\n- mode: polling fallback\n", err)
		} else {
			defer codexWatcher.Close()
			writeDaemonLine(opts.Stdout, "Codex watcher active\n")
		}
	}

	ticker := time.NewTicker(opts.PollEvery)
	defer ticker.Stop()

	var (
		debounceTimer      *time.Timer
		debounceCh         <-chan time.Time
		codexDebounceTimer *time.Timer
		codexDebounceCh    <-chan time.Time
		codexSyncDone      = make(chan struct{}, 1)
		codexSyncRunning   bool
	)
	pendingPaths := make(map[string]struct{})

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
	stopCodexDebounce := func() {
		if codexDebounceTimer == nil {
			return
		}
		if !codexDebounceTimer.Stop() {
			select {
			case <-codexDebounceTimer.C:
			default:
			}
		}
		codexDebounceCh = nil
	}
	resetDebounce := func() {
		if debounceTimer == nil {
			debounceTimer = time.NewTimer(r.Config.Vault.DebounceWindow)
			debounceCh = debounceTimer.C
			return
		}
		if !debounceTimer.Stop() {
			select {
			case <-debounceTimer.C:
			default:
			}
		}
		debounceTimer.Reset(r.Config.Vault.DebounceWindow)
		debounceCh = debounceTimer.C
	}
	resetCodexDebounce := func() {
		if codexDebounceTimer == nil {
			codexDebounceTimer = time.NewTimer(codexWatchDebounce)
			codexDebounceCh = codexDebounceTimer.C
			return
		}
		if !codexDebounceTimer.Stop() {
			select {
			case <-codexDebounceTimer.C:
			default:
			}
		}
		codexDebounceTimer.Reset(codexWatchDebounce)
		codexDebounceCh = codexDebounceTimer.C
	}
	flushPending := func(now time.Time) error {
		if len(pendingPaths) == 0 {
			return nil
		}
		paths := make([]string, 0, len(pendingPaths))
		for path := range pendingPaths {
			paths = append(paths, path)
		}
		sort.Strings(paths)
		clear(pendingPaths)

		result, stillPending, err := r.scanVaultPaths(now, paths)
		if err != nil {
			return err
		}
		if result.ScannedPlanDocs > 0 {
			writeDaemonScanSummary(opts.Stdout, result)
		}
		for _, path := range stillPending {
			pendingPaths[path] = struct{}{}
		}
		if len(pendingPaths) > 0 {
			resetDebounce()
		} else {
			debounceCh = nil
		}
		return nil
	}
	stopAndReturn := func() error {
		stopDebounce()
		stopCodexDebounce()
		writeDaemonLine(opts.Stdout, "Vault daemon stopped\n")
		return nil
	}
	startCodexSync := func() {
		if opts.CodexJSONL == nil || strings.TrimSpace(opts.CodexJSONL.InputPath) == "" || codexSyncRunning {
			return
		}
		codexSyncRunning = true
		go func() {
			r.syncCodexJSONLIfConfigured(opts, modelAvailable)
			codexSyncDone <- struct{}{}
		}()
	}

	for {
		if ctx.Err() != nil {
			return stopAndReturn()
		}

		var (
			watchEvents <-chan fsnotify.Event
			watchErrors <-chan error
			codexEvents <-chan fsnotify.Event
			codexErrors <-chan error
		)
		if watcher != nil {
			watchEvents = watcher.Events()
			watchErrors = watcher.Errors()
		}
		if codexWatcher != nil {
			codexEvents = codexWatcher.Events()
			codexErrors = codexWatcher.Errors()
		}

		select {
		case <-ctx.Done():
			return stopAndReturn()
		case event, ok := <-watchEvents:
			if !ok {
				watcher = nil
				continue
			}
			paths, err := watcher.CollectPaths(event)
			if err != nil {
				writeDaemonLine(opts.Stdout, "Vault watcher event failed\n- path: %s\n- error: %s\n", event.Name, err)
				continue
			}
			if len(paths) == 0 {
				continue
			}
			for _, path := range paths {
				pendingPaths[path] = struct{}{}
			}
			resetDebounce()
		case event, ok := <-codexEvents:
			if !ok {
				codexWatcher = nil
				continue
			}
			if !codexWatcher.Matches(event) {
				continue
			}
			if event.Op&(fsnotify.Create|fsnotify.Write|fsnotify.Rename|fsnotify.Chmod) == 0 {
				continue
			}
			resetCodexDebounce()
		case err, ok := <-watchErrors:
			if !ok {
				watcher = nil
				continue
			}
			writeDaemonLine(opts.Stdout, "Vault watcher error\n- error: %s\n- mode: polling fallback\n", err)
			if watcher != nil {
				watcher.Close()
			}
			watcher = nil
		case err, ok := <-codexErrors:
			if !ok {
				codexWatcher = nil
				continue
			}
			writeDaemonLine(opts.Stdout, "Codex watcher error\n- error: %s\n- mode: polling fallback\n", err)
			if codexWatcher != nil {
				codexWatcher.Close()
			}
			codexWatcher = nil
		case now := <-debounceCh:
			debounceCh = nil
			if err := flushPending(now); err != nil {
				return err
			}
			if ctx.Err() != nil {
				return stopAndReturn()
			}
		case <-codexDebounceCh:
			codexDebounceCh = nil
			startCodexSync()
			if ctx.Err() != nil {
				return stopAndReturn()
			}
		case <-codexSyncDone:
			codexSyncRunning = false
		case now := <-ticker.C:
			if watcher == nil {
				result, err := r.ScanVaultChanges(now)
				if err != nil {
					return err
				}
				writeDaemonScanSummary(opts.Stdout, result)
			}
			if codexWatcher == nil {
				startCodexSync()
			}
			if ctx.Err() != nil {
				return stopAndReturn()
			}
		}
	}
}

func (r *Runtime) syncCodexJSONLIfConfigured(opts VaultDaemonRunOptions, modelAvailable bool) {
	if opts.CodexJSONL == nil || strings.TrimSpace(opts.CodexJSONL.InputPath) == "" {
		return
	}
	syncResult, err := r.SyncCodexJSONL(*opts.CodexJSONL, time.Now())
	if err != nil {
		r.Harness.UpdateDependencies(modelAvailable, false)
		writeDaemonCodexError(opts.Stdout, err)
		return
	}
	writeDaemonCodexSummary(opts.Stdout, syncResult)
}

func writeDaemonScanSummary(stdout io.Writer, result VaultDaemonScanResult) {
	if stdout == nil {
		return
	}

	fmt.Fprintf(
		stdout,
		"Vault daemon scan\n- scanned plan docs: %d\n- primed: %d\n- pending: %d\n- triggered drafts: %d\n",
		result.ScannedPlanDocs,
		result.Primed,
		result.Pending,
		result.TriggeredDrafts,
	)
	if len(result.DraftIDs) > 0 {
		fmt.Fprintf(stdout, "- draft ids: %s\n", strings.Join(result.DraftIDs, ", "))
	}
}

func writeDaemonLine(stdout io.Writer, format string, args ...any) {
	if stdout == nil {
		return
	}
	fmt.Fprintf(stdout, format, args...)
}

func writeDaemonCodexSummary(stdout io.Writer, result SyncCodexJSONLResult) {
	if stdout == nil {
		return
	}
	if !result.Changed {
		fmt.Fprintf(stdout, "Codex JSONL unchanged\n- cursor: %s\n", result.Fingerprint)
		return
	}

	fmt.Fprintf(
		stdout,
		"Codex JSONL synced\n- agent: %s\n- session: %s\n- cursor: %s\n- checkpoints: %d\n- daily reports: %d\n",
		result.Import.AgentID,
		result.Import.SessionID,
		result.Fingerprint,
		len(result.Import.Checkpoints),
		len(result.Import.Reports),
	)
}

func writeDaemonCodexError(stdout io.Writer, err error) {
	if stdout == nil || err == nil {
		return
	}
	fmt.Fprintf(stdout, "Codex JSONL sync failed\n- error: %s\n", err)
}

func isDaemonPlanClass(docClass model.DocClass) bool {
	return docClass == model.DocClassPlanMaster || docClass == model.DocClassPlanWeek
}

func vaultWatchCursorKey(relPath string) string {
	return "vaultwatch:" + relPath
}

func changeStable(now time.Time, modTime time.Time, debounce time.Duration) bool {
	if debounce <= 0 {
		return true
	}
	if now.Before(modTime) {
		return false
	}
	return now.Sub(modTime) >= debounce
}
