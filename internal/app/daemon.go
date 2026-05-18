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

	"obsidian-harness/internal/domain/docclass"
	"obsidian-harness/internal/model"
	"obsidian-harness/internal/store"
	"obsidian-harness/internal/vault"
)

type VaultDaemonScanResult struct {
	ScannedMarkdownDocs int
	ScannedPlanDocs     int
	Primed              int
	Pending             int
	TriggeredDrafts     int
	OutOfBandNotes      int
	GovernanceFindings  int
	DraftIDs            []string
	FindingIDs          []string
}

type vaultDaemonChangeClass string

const (
	vaultDaemonChangePlan     vaultDaemonChangeClass = "plan"
	vaultDaemonChangeNote     vaultDaemonChangeClass = "note"
	vaultDaemonChangeGoverned vaultDaemonChangeClass = "governed"
)

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
	if err != nil {
		return VaultDaemonScanResult{}, err
	}
	if err := r.Store.Cursors().SaveCursor(vaultWatchBaselineCursorKey(), fmt.Sprintf("%d", now.UnixNano())); err != nil {
		return VaultDaemonScanResult{}, err
	}
	return result, nil
}

func (r *Runtime) scanVaultPaths(now time.Time, relPaths []string) (VaultDaemonScanResult, []string, error) {
	if _, err := r.Bootstrap(now); err != nil {
		return VaultDaemonScanResult{}, nil, err
	}
	classifier, err := docclass.NewClassifier(docclass.RecommendedRules())
	if err != nil {
		return VaultDaemonScanResult{}, nil, err
	}
	processSinkRelDir, err := vaultRelPath(r.Config.Paths.VaultRoot, r.Config.Paths.ProcessSinkDir)
	if err != nil {
		return VaultDaemonScanResult{}, nil, err
	}
	baselineComplete, err := r.vaultWatchBaselineComplete()
	if err != nil {
		return VaultDaemonScanResult{}, nil, err
	}

	result := VaultDaemonScanResult{
		DraftIDs:   make([]string, 0),
		FindingIDs: make([]string, 0),
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

		docClass, changeClass := classifyVaultDaemonPath(relPath, classifier, r.Config.Vault.ManagedCore, processSinkRelDir)
		result.ScannedMarkdownDocs++
		if changeClass == vaultDaemonChangePlan {
			result.ScannedPlanDocs++
		}

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
			if !baselineComplete {
				if err := r.Store.Cursors().SaveCursor(cursorKey, hash); err != nil {
					return VaultDaemonScanResult{}, nil, err
				}
				result.Primed++
				continue
			}
		} else if lastHash == hash {
			continue
		}
		if !changeStable(now, info.ModTime(), r.Config.Vault.DebounceWindow) {
			result.Pending++
			pendingPaths = append(pendingPaths, normalizedPath)
			continue
		}

		switch changeClass {
		case vaultDaemonChangePlan:
			draft, err := r.Harness.ObserveDocumentChange(normalizedPath, content, now)
			if err != nil {
				return VaultDaemonScanResult{}, nil, err
			}
			if err := r.Store.Cursors().SaveCursor(cursorKey, hash); err != nil {
				return VaultDaemonScanResult{}, nil, err
			}
			result.TriggeredDrafts++
			result.DraftIDs = append(result.DraftIDs, draft.ID)
		case vaultDaemonChangeGoverned:
			findingID, err := r.recordOutOfBandScanFinding(model.AuditGovernanceFinding, model.FindingGovernanceReviewNeeded, model.FindingSeverityWarning, normalizedPath, docClass, "review_required", hash, now)
			if err != nil {
				return VaultDaemonScanResult{}, nil, err
			}
			if err := r.Store.Cursors().SaveCursor(cursorKey, hash); err != nil {
				return VaultDaemonScanResult{}, nil, err
			}
			result.GovernanceFindings++
			result.FindingIDs = append(result.FindingIDs, findingID)
		default:
			findingID, err := r.recordOutOfBandScanFinding(model.AuditOutOfBandVaultWrite, model.FindingOutOfBandVaultWrite, model.FindingSeverityInfo, normalizedPath, model.DocClassNote, "audit_only", hash, now)
			if err != nil {
				return VaultDaemonScanResult{}, nil, err
			}
			if err := r.Store.Cursors().SaveCursor(cursorKey, hash); err != nil {
				return VaultDaemonScanResult{}, nil, err
			}
			result.OutOfBandNotes++
			result.FindingIDs = append(result.FindingIDs, findingID)
		}
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

	watcher, err := newVaultWatcherFunc(r.Config.Paths.VaultRoot)
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
		if result.hasVisibleActivity() {
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
		"Vault daemon scan\n- scanned markdown docs: %d\n- scanned plan docs: %d\n- primed: %d\n- pending: %d\n- triggered drafts: %d\n- out-of-band notes: %d\n- governance findings: %d\n",
		result.ScannedMarkdownDocs,
		result.ScannedPlanDocs,
		result.Primed,
		result.Pending,
		result.TriggeredDrafts,
		result.OutOfBandNotes,
		result.GovernanceFindings,
	)
	if len(result.DraftIDs) > 0 {
		fmt.Fprintf(stdout, "- draft ids: %s\n", strings.Join(result.DraftIDs, ", "))
	}
	if len(result.FindingIDs) > 0 {
		fmt.Fprintf(stdout, "- finding ids: %s\n", strings.Join(result.FindingIDs, ", "))
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

func (r *Runtime) vaultWatchBaselineComplete() (bool, error) {
	_, err := r.Store.Cursors().GetCursor(vaultWatchBaselineCursorKey())
	if err == nil {
		return true, nil
	}
	if err == store.ErrNotFound {
		return false, nil
	}
	return false, err
}

func classifyVaultDaemonPath(relPath string, classifier docclass.Classifier, managed model.ManagedCorePaths, processSinkRelDir string) (model.DocClass, vaultDaemonChangeClass) {
	relPath = vault.NormalizeRelativePath(relPath)
	switch {
	case sameDaemonRelPath(relPath, managed.SystemDoc):
		return model.DocClassSystemDoc, vaultDaemonChangeGoverned
	case sameDaemonRelPath(relPath, managed.ProgressIndex):
		return model.DocClassProgressIndex, vaultDaemonChangeGoverned
	case sameDaemonRelPath(relPath, managed.Persona):
		return model.DocClassPersona, vaultDaemonChangeGoverned
	case sameDaemonRelPath(relPath, managed.AgentDoc):
		return model.DocClassAgentDoc, vaultDaemonChangeGoverned
	case sameDaemonRelPath(relPath, managed.IdentityDoc):
		return model.DocClassIdentityDoc, vaultDaemonChangeGoverned
	case isUnderDaemonRelPath(relPath, processSinkRelDir):
		return model.DocClassCheckpoint, vaultDaemonChangeGoverned
	}

	classification := classifier.Classify(relPath)
	if isDaemonPlanClass(classification.Class) {
		return classification.Class, vaultDaemonChangePlan
	}
	return model.DocClassNote, vaultDaemonChangeNote
}

func (r *Runtime) recordOutOfBandScanFinding(auditKind model.AuditKind, findingKind model.FindingKind, severity model.FindingSeverity, target string, docClass model.DocClass, action string, hash string, at time.Time) (string, error) {
	auditID := daemonScanAuditID(auditKind, target, at)
	if err := r.Store.Audit().AppendAudit(model.AuditRecord{
		ID:         auditID,
		Kind:       auditKind,
		Actor:      "vault_daemon",
		Target:     target,
		OccurredAt: at,
		Metadata: map[string]string{
			"doc_class": string(docClass),
			"action":    action,
			"hash":      hash,
		},
	}); err != nil {
		return "", err
	}

	findingID := daemonScanFindingID(findingKind, target, at)
	finding := model.Finding{
		ID:       findingID,
		Kind:     findingKind,
		State:    model.FindingOpen,
		Severity: severity,
		Target: model.DocumentRef{
			Path:        target,
			Class:       docClass,
			BaseVersion: hash,
		},
		Title:      daemonScanFindingTitle(findingKind),
		Summary:    daemonScanFindingSummary(findingKind),
		Source:     "vault_daemon",
		AuditID:    auditID,
		DetectedAt: at,
		UpdatedAt:  at,
		Metadata: map[string]string{
			"action": action,
			"hash":   hash,
		},
	}
	if err := r.Store.Findings().SaveFinding(finding); err != nil {
		return "", err
	}
	return findingID, nil
}

func daemonScanFindingTitle(kind model.FindingKind) string {
	switch kind {
	case model.FindingGovernanceReviewNeeded:
		return "Governed document changed outside Lore"
	default:
		return "Out-of-band vault note change"
	}
}

func daemonScanFindingSummary(kind model.FindingKind) string {
	switch kind {
	case model.FindingGovernanceReviewNeeded:
		return "A governed document changed outside Lore. Review is required before treating the change as governed state."
	default:
		return "An ordinary markdown note changed outside Lore. The change was audited and may need review for knowledge quality."
	}
}

func daemonScanAuditID(kind model.AuditKind, target string, at time.Time) string {
	normalizedTarget := vault.NormalizeRelativePath(target)
	normalizedTarget = strings.NewReplacer("/", "-", "\\", "-", " ", "-").Replace(normalizedTarget)
	normalizedTarget = truncateRunes(normalizedTarget, 80)
	return fmt.Sprintf("%s-%s-%d", kind, normalizedTarget, at.UnixNano())
}

func daemonScanFindingID(kind model.FindingKind, target string, at time.Time) string {
	normalizedTarget := vault.NormalizeRelativePath(target)
	normalizedTarget = strings.NewReplacer("/", "-", "\\", "-", " ", "-").Replace(normalizedTarget)
	normalizedTarget = truncateRunes(normalizedTarget, 80)
	return fmt.Sprintf("%s-%s-%d", kind, normalizedTarget, at.UnixNano())
}

func truncateRunes(value string, limit int) string {
	if limit <= 0 {
		return ""
	}
	runes := []rune(value)
	if len(runes) <= limit {
		return value
	}
	return string(runes[:limit])
}

func (result VaultDaemonScanResult) hasVisibleActivity() bool {
	return result.ScannedPlanDocs > 0 ||
		result.TriggeredDrafts > 0 ||
		result.OutOfBandNotes > 0 ||
		result.GovernanceFindings > 0 ||
		result.Pending > 0
}

func vaultRelPath(root string, absolutePath string) (string, error) {
	relPath, err := filepath.Rel(root, absolutePath)
	if err != nil {
		return "", err
	}
	normalized := vault.NormalizeRelativePath(relPath)
	if normalized == "." || strings.HasPrefix(normalized, "../") || normalized == ".." {
		return "", nil
	}
	return normalized, nil
}

func sameDaemonRelPath(a string, b string) bool {
	return strings.EqualFold(vault.NormalizeRelativePath(a), vault.NormalizeRelativePath(b))
}

func isUnderDaemonRelPath(relPath string, parent string) bool {
	relPath = vault.NormalizeRelativePath(relPath)
	parent = vault.NormalizeRelativePath(parent)
	if parent == "" {
		return false
	}
	return sameDaemonRelPath(relPath, parent) || strings.HasPrefix(strings.ToLower(relPath), strings.ToLower(parent)+"/")
}

func vaultWatchCursorKey(relPath string) string {
	return "vaultwatch:" + relPath
}

func vaultWatchBaselineCursorKey() string {
	return "vaultwatch:baseline"
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
