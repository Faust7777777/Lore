package app

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

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

func (r *Runtime) ScanVaultChanges(now time.Time) (VaultDaemonScanResult, error) {
	if _, err := r.Bootstrap(now); err != nil {
		return VaultDaemonScanResult{}, err
	}

	paths, err := vault.WalkMarkdownPaths(r.Config.Paths.VaultRoot, "")
	if err != nil {
		return VaultDaemonScanResult{}, err
	}

	result := VaultDaemonScanResult{
		DraftIDs: make([]string, 0),
	}

	for _, relPath := range paths {
		docClass := r.Harness.DocClassify(relPath).DocClass
		if !isDaemonPlanClass(docClass) {
			continue
		}
		result.ScannedPlanDocs++

		absPath := filepath.Join(r.Config.Paths.VaultRoot, filepath.FromSlash(relPath))
		info, err := os.Stat(absPath)
		if err != nil {
			return VaultDaemonScanResult{}, err
		}

		content, hash, normalizedPath, err := vault.ReadRelativeWithHash(r.Config.Paths.VaultRoot, relPath)
		if err != nil {
			return VaultDaemonScanResult{}, err
		}

		cursorKey := vaultWatchCursorKey(normalizedPath)
		lastHash, err := r.Store.Cursors().GetCursor(cursorKey)
		if err != nil {
			if err != store.ErrNotFound {
				return VaultDaemonScanResult{}, err
			}
			if err := r.Store.Cursors().SaveCursor(cursorKey, hash); err != nil {
				return VaultDaemonScanResult{}, err
			}
			result.Primed++
			continue
		}

		if lastHash == hash {
			continue
		}
		if !changeStable(now, info.ModTime(), r.Config.Vault.DebounceWindow) {
			result.Pending++
			continue
		}

		draft, err := r.Harness.ObserveDocumentChange(normalizedPath, content, now)
		if err != nil {
			return VaultDaemonScanResult{}, err
		}
		if err := r.Store.Cursors().SaveCursor(cursorKey, hash); err != nil {
			return VaultDaemonScanResult{}, err
		}
		result.TriggeredDrafts++
		result.DraftIDs = append(result.DraftIDs, draft.ID)
	}

	return result, nil
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
	writeDaemonLine(opts.Stdout, "Vault daemon running\n- poll: %s\n- debounce: %s\n", opts.PollEvery, r.Config.Vault.DebounceWindow)

	for {
		result, err := r.ScanVaultChanges(time.Now())
		if err != nil {
			return err
		}
		writeDaemonScanSummary(opts.Stdout, result)

		if opts.CodexJSONL != nil && strings.TrimSpace(opts.CodexJSONL.InputPath) != "" {
			syncResult, err := r.SyncCodexJSONL(*opts.CodexJSONL, time.Now())
			if err != nil {
				r.Harness.UpdateDependencies(modelAvailable, false)
				writeDaemonCodexError(opts.Stdout, err)
			} else {
				writeDaemonCodexSummary(opts.Stdout, syncResult)
			}
		}
		if opts.Once {
			return nil
		}

		timer := time.NewTimer(opts.PollEvery)
		select {
		case <-ctx.Done():
			timer.Stop()
			writeDaemonLine(opts.Stdout, "Vault daemon stopped\n")
			return nil
		case <-timer.C:
		}
	}
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
