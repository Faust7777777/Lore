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
	"unicode/utf8"

	"github.com/fsnotify/fsnotify"

	"obsidian-harness/internal/config/configtest"
	"obsidian-harness/internal/model"
	"obsidian-harness/internal/vault"
)

func TestRuntimeScanVaultChangesPrimesThenCreatesDraft(t *testing.T) {
	workDir := t.TempDir()
	runtime, err := openRuntimeForTest(t, workDir)
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
	runtime, err := openRuntimeForTest(t, workDir)
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

func TestRuntimeScanVaultChangesAuditsOutOfBandOrdinaryNote(t *testing.T) {
	workDir := t.TempDir()
	runtime, err := openRuntimeForTest(t, workDir)
	if err != nil {
		t.Fatalf("OpenRuntime() error = %v", err)
	}
	runtime.Config.Vault.DebounceWindow = 500 * time.Millisecond

	now := time.Date(2026, 4, 28, 9, 0, 0, 0, time.Local)
	if _, err := runtime.Bootstrap(now); err != nil {
		t.Fatalf("Bootstrap() error = %v", err)
	}

	relPath := filepath.Join("03-notes", "class", "ecommerce.md")
	absPath := filepath.Join(runtime.Config.Paths.VaultRoot, relPath)
	writeTestPlan(t, runtime, absPath, "# E-commerce\n\nInitial note.", now.Add(-time.Second))
	if _, err := runtime.ScanVaultChanges(now); err != nil {
		t.Fatalf("ScanVaultChanges(prime) error = %v", err)
	}

	secondNow := now.Add(5 * time.Second)
	writeTestPlan(t, runtime, absPath, "# E-commerce\n\nChanged outside Lore.", secondNow.Add(-time.Second))
	result, err := runtime.ScanVaultChanges(secondNow)
	if err != nil {
		t.Fatalf("ScanVaultChanges(changed) error = %v", err)
	}
	if result.OutOfBandNotes != 1 {
		t.Fatalf("result.OutOfBandNotes = %d, want 1", result.OutOfBandNotes)
	}
	if result.TriggeredDrafts != 0 {
		t.Fatalf("result.TriggeredDrafts = %d, want 0 for ordinary note audit", result.TriggeredDrafts)
	}
	assertNoDrafts(t, runtime)
	assertAuditRecord(t, runtime, model.AuditOutOfBandVaultWrite, "03-notes/class/ecommerce.md", map[string]string{
		"doc_class": "note",
		"action":    "audit_only",
	})
	assertFinding(t, runtime, model.FindingOutOfBandVaultWrite, "03-notes/class/ecommerce.md", model.DocClassNote, model.FindingSeverityInfo, map[string]string{
		"action": "audit_only",
	})
}

func TestRuntimeScanVaultChangesAuditsGovernedCoreOutOfBandChange(t *testing.T) {
	workDir := t.TempDir()
	runtime, err := openRuntimeForTest(t, workDir)
	if err != nil {
		t.Fatalf("OpenRuntime() error = %v", err)
	}
	runtime.Config.Vault.DebounceWindow = 500 * time.Millisecond

	now := time.Date(2026, 4, 28, 10, 0, 0, 0, time.Local)
	if _, err := runtime.Bootstrap(now); err != nil {
		t.Fatalf("Bootstrap() error = %v", err)
	}
	if _, err := runtime.ScanVaultChanges(now); err != nil {
		t.Fatalf("ScanVaultChanges(prime) error = %v", err)
	}

	secondNow := now.Add(5 * time.Second)
	absPath := filepath.Join(runtime.Config.Paths.VaultRoot, runtime.Config.Vault.ManagedCore.Persona)
	writeTestPlan(t, runtime, absPath, "# Persona\n\nChanged outside Lore.", secondNow.Add(-time.Second))
	result, err := runtime.ScanVaultChanges(secondNow)
	if err != nil {
		t.Fatalf("ScanVaultChanges(changed) error = %v", err)
	}
	if result.GovernanceFindings != 1 {
		t.Fatalf("result.GovernanceFindings = %d, want 1", result.GovernanceFindings)
	}
	if result.TriggeredDrafts != 0 {
		t.Fatalf("result.TriggeredDrafts = %d, want 0 for governed finding", result.TriggeredDrafts)
	}
	assertNoDrafts(t, runtime)
	assertAuditRecord(t, runtime, model.AuditGovernanceFinding, vault.NormalizeRelativePath(runtime.Config.Vault.ManagedCore.Persona), map[string]string{
		"doc_class": "persona",
		"action":    "review_required",
	})
	assertFinding(t, runtime, model.FindingGovernanceReviewNeeded, vault.NormalizeRelativePath(runtime.Config.Vault.ManagedCore.Persona), model.DocClassPersona, model.FindingSeverityWarning, map[string]string{
		"action": "review_required",
	})
}

func TestRuntimeScanVaultChangesAuditsProcessSinkOutOfBandChange(t *testing.T) {
	workDir := t.TempDir()
	runtime, err := openRuntimeForTest(t, workDir)
	if err != nil {
		t.Fatalf("OpenRuntime() error = %v", err)
	}
	runtime.Config.Vault.DebounceWindow = 500 * time.Millisecond

	now := time.Date(2026, 4, 28, 11, 0, 0, 0, time.Local)
	if _, err := runtime.Bootstrap(now); err != nil {
		t.Fatalf("Bootstrap() error = %v", err)
	}

	processSinkRelDir, err := filepath.Rel(runtime.Config.Paths.VaultRoot, runtime.Config.Paths.ProcessSinkDir)
	if err != nil {
		t.Fatalf("Rel(process sink) error = %v", err)
	}
	relPath := filepath.Join(processSinkRelDir, "codex", "2026-04-28.md")
	absPath := filepath.Join(runtime.Config.Paths.VaultRoot, relPath)
	writeTestPlan(t, runtime, absPath, "# Codex checkpoint\n\nInitial.", now.Add(-time.Second))
	if _, err := runtime.ScanVaultChanges(now); err != nil {
		t.Fatalf("ScanVaultChanges(prime) error = %v", err)
	}

	secondNow := now.Add(5 * time.Second)
	writeTestPlan(t, runtime, absPath, "# Codex checkpoint\n\nChanged outside Lore.", secondNow.Add(-time.Second))
	result, err := runtime.ScanVaultChanges(secondNow)
	if err != nil {
		t.Fatalf("ScanVaultChanges(changed) error = %v", err)
	}
	if result.GovernanceFindings != 1 {
		t.Fatalf("result.GovernanceFindings = %d, want 1", result.GovernanceFindings)
	}
	if result.OutOfBandNotes != 0 {
		t.Fatalf("result.OutOfBandNotes = %d, want 0 for process-sink", result.OutOfBandNotes)
	}
	assertNoDrafts(t, runtime)
	assertAuditRecord(t, runtime, model.AuditGovernanceFinding, vault.NormalizeRelativePath(relPath), map[string]string{
		"doc_class": "checkpoint",
		"action":    "review_required",
	})
	assertFinding(t, runtime, model.FindingGovernanceReviewNeeded, vault.NormalizeRelativePath(relPath), model.DocClassCheckpoint, model.FindingSeverityWarning, map[string]string{
		"action": "review_required",
	})
}

func TestRuntimeScanVaultChangesAuditsNewOutOfBandOrdinaryNoteAfterBaseline(t *testing.T) {
	workDir := t.TempDir()
	runtime, err := openRuntimeForTest(t, workDir)
	if err != nil {
		t.Fatalf("OpenRuntime() error = %v", err)
	}
	runtime.Config.Vault.DebounceWindow = 500 * time.Millisecond

	now := time.Date(2026, 4, 28, 12, 0, 0, 0, time.Local)
	if _, err := runtime.Bootstrap(now); err != nil {
		t.Fatalf("Bootstrap() error = %v", err)
	}
	if _, err := runtime.ScanVaultChanges(now); err != nil {
		t.Fatalf("ScanVaultChanges(baseline) error = %v", err)
	}

	secondNow := now.Add(5 * time.Second)
	relPath := filepath.Join("03-notes", "class", "new-note.md")
	absPath := filepath.Join(runtime.Config.Paths.VaultRoot, relPath)
	writeTestPlan(t, runtime, absPath, "# New Note\n\nCreated outside Lore.", secondNow.Add(-time.Second))
	result, err := runtime.ScanVaultChanges(secondNow)
	if err != nil {
		t.Fatalf("ScanVaultChanges(new note) error = %v", err)
	}
	if result.Primed != 0 {
		t.Fatalf("result.Primed = %d, want 0 for new file after baseline", result.Primed)
	}
	if result.OutOfBandNotes != 1 {
		t.Fatalf("result.OutOfBandNotes = %d, want 1", result.OutOfBandNotes)
	}
	assertNoDrafts(t, runtime)
	assertAuditRecord(t, runtime, model.AuditOutOfBandVaultWrite, "03-notes/class/new-note.md", map[string]string{
		"doc_class": "note",
		"action":    "audit_only",
	})
	assertFinding(t, runtime, model.FindingOutOfBandVaultWrite, "03-notes/class/new-note.md", model.DocClassNote, model.FindingSeverityInfo, map[string]string{
		"action": "audit_only",
	})
}

func TestRuntimeScanVaultChangesAuditsRecreatedGovernedCoreAfterBaseline(t *testing.T) {
	workDir := t.TempDir()
	runtime, err := openRuntimeForTest(t, workDir)
	if err != nil {
		t.Fatalf("OpenRuntime() error = %v", err)
	}
	runtime.Config.Vault.DebounceWindow = 500 * time.Millisecond

	now := time.Date(2026, 4, 28, 13, 0, 0, 0, time.Local)
	if _, err := runtime.Bootstrap(now); err != nil {
		t.Fatalf("Bootstrap() error = %v", err)
	}
	if err := os.Remove(filepath.Join(runtime.Config.Paths.VaultRoot, runtime.Config.Vault.ManagedCore.Persona)); err != nil {
		t.Fatalf("Remove(persona) error = %v", err)
	}
	if _, err := runtime.ScanVaultChanges(now); err != nil {
		t.Fatalf("ScanVaultChanges(baseline) error = %v", err)
	}

	secondNow := now.Add(5 * time.Second)
	absPath := filepath.Join(runtime.Config.Paths.VaultRoot, runtime.Config.Vault.ManagedCore.Persona)
	writeTestPlan(t, runtime, absPath, "# Persona\n\nRecreated outside Lore.", secondNow.Add(-time.Second))
	result, err := runtime.ScanVaultChanges(secondNow)
	if err != nil {
		t.Fatalf("ScanVaultChanges(new core) error = %v", err)
	}
	if result.Primed != 0 {
		t.Fatalf("result.Primed = %d, want 0 for new governed file after baseline", result.Primed)
	}
	if result.GovernanceFindings != 1 {
		t.Fatalf("result.GovernanceFindings = %d, want 1", result.GovernanceFindings)
	}
	assertNoDrafts(t, runtime)
	assertAuditRecord(t, runtime, model.AuditGovernanceFinding, vault.NormalizeRelativePath(runtime.Config.Vault.ManagedCore.Persona), map[string]string{
		"doc_class": "persona",
		"action":    "review_required",
	})
	assertFinding(t, runtime, model.FindingGovernanceReviewNeeded, vault.NormalizeRelativePath(runtime.Config.Vault.ManagedCore.Persona), model.DocClassPersona, model.FindingSeverityWarning, map[string]string{
		"action": "review_required",
	})
}

func TestRuntimeScanVaultChangesAuditsNewProcessSinkAfterBaseline(t *testing.T) {
	workDir := t.TempDir()
	runtime, err := openRuntimeForTest(t, workDir)
	if err != nil {
		t.Fatalf("OpenRuntime() error = %v", err)
	}
	runtime.Config.Vault.DebounceWindow = 500 * time.Millisecond

	now := time.Date(2026, 4, 28, 14, 0, 0, 0, time.Local)
	if _, err := runtime.Bootstrap(now); err != nil {
		t.Fatalf("Bootstrap() error = %v", err)
	}
	if _, err := runtime.ScanVaultChanges(now); err != nil {
		t.Fatalf("ScanVaultChanges(baseline) error = %v", err)
	}

	secondNow := now.Add(5 * time.Second)
	processSinkRelDir, err := filepath.Rel(runtime.Config.Paths.VaultRoot, runtime.Config.Paths.ProcessSinkDir)
	if err != nil {
		t.Fatalf("Rel(process sink) error = %v", err)
	}
	relPath := filepath.Join(processSinkRelDir, "external", "2026-04-28.md")
	absPath := filepath.Join(runtime.Config.Paths.VaultRoot, relPath)
	writeTestPlan(t, runtime, absPath, "# External checkpoint\n\nCreated outside Lore.", secondNow.Add(-time.Second))
	result, err := runtime.ScanVaultChanges(secondNow)
	if err != nil {
		t.Fatalf("ScanVaultChanges(new process-sink) error = %v", err)
	}
	if result.Primed != 0 {
		t.Fatalf("result.Primed = %d, want 0 for new process-sink file after baseline", result.Primed)
	}
	if result.GovernanceFindings != 1 {
		t.Fatalf("result.GovernanceFindings = %d, want 1", result.GovernanceFindings)
	}
	assertNoDrafts(t, runtime)
	assertAuditRecord(t, runtime, model.AuditGovernanceFinding, vault.NormalizeRelativePath(relPath), map[string]string{
		"doc_class": "checkpoint",
		"action":    "review_required",
	})
	assertFinding(t, runtime, model.FindingGovernanceReviewNeeded, vault.NormalizeRelativePath(relPath), model.DocClassCheckpoint, model.FindingSeverityWarning, map[string]string{
		"action": "review_required",
	})
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

func assertNoDrafts(t *testing.T, runtime *Runtime) {
	t.Helper()

	drafts, err := runtime.ListDrafts()
	if err != nil {
		t.Fatalf("ListDrafts() error = %v", err)
	}
	if len(drafts) != 0 {
		t.Fatalf("len(drafts) = %d, want 0", len(drafts))
	}
}

func assertAuditRecord(t *testing.T, runtime *Runtime, kind model.AuditKind, target string, metadata map[string]string) {
	t.Helper()

	records, err := runtime.Store.Audit().ListAudit(64)
	if err != nil {
		t.Fatalf("ListAudit() error = %v", err)
	}
	for _, record := range records {
		if record.Kind != kind || record.Target != target {
			continue
		}
		matches := true
		for key, want := range metadata {
			if record.Metadata[key] != want {
				matches = false
				break
			}
		}
		if matches {
			return
		}
	}
	t.Fatalf("audit record kind=%q target=%q metadata=%v not found in %+v", kind, target, metadata, records)
}

func assertFinding(t *testing.T, runtime *Runtime, kind model.FindingKind, target string, docClass model.DocClass, severity model.FindingSeverity, metadata map[string]string) {
	t.Helper()

	findings, err := runtime.ListFindings(64)
	if err != nil {
		t.Fatalf("ListFindings() error = %v", err)
	}
	for _, finding := range findings {
		if finding.Kind != kind || finding.Target.Path != target || finding.Target.Class != docClass || finding.Severity != severity {
			continue
		}
		if finding.State != model.FindingOpen {
			t.Fatalf("finding.State = %q, want open for %+v", finding.State, finding)
		}
		if finding.AuditID == "" {
			t.Fatalf("finding.AuditID is empty for %+v", finding)
		}
		matches := true
		for key, want := range metadata {
			if finding.Metadata[key] != want {
				matches = false
				break
			}
		}
		if matches {
			return
		}
	}
	t.Fatalf("finding kind=%q target=%q class=%q metadata=%v not found in %+v", kind, target, docClass, metadata, findings)
}

func TestDaemonScanAuditIDPreservesUTF8WhenTruncatingTarget(t *testing.T) {
	target := strings.Repeat("界", 27) + ".md"

	id := daemonScanAuditID(model.AuditOutOfBandVaultWrite, target, time.Date(2026, 4, 28, 15, 0, 0, 0, time.UTC))

	if !utf8.ValidString(id) {
		t.Fatalf("daemonScanAuditID() produced invalid UTF-8: %q", id)
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
		FindingIDs:      []string{"finding-1"},
	})

	text := output.String()
	for _, expected := range []string{
		"Vault daemon scan",
		"scanned plan docs: 2",
		"triggered drafts: 1",
		"draft ids: draft-1",
		"finding ids: finding-1",
	} {
		if !strings.Contains(text, expected) {
			t.Fatalf("expected summary to contain %q, got %q", expected, text)
		}
	}
}

func TestRunVaultDaemonContinuesAfterCodexSyncFailure(t *testing.T) {
	workDir := t.TempDir()
	runtime, err := openRuntimeForTest(t, workDir)
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
	runtime, err := openRuntimeForTest(t, workDir)
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
	runtime, err := openRuntimeForTest(t, workDir)
	if err != nil {
		t.Fatalf("OpenRuntime() error = %v", err)
	}
	runtime.Config.Vault.DebounceWindow = 0

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

	fakeWatcher := newFakeVaultEventWatcher()
	previousWatcherFactory := newVaultWatcherFunc
	newVaultWatcherFunc = func(root string) (vaultEventWatcher, error) {
		return fakeWatcher, nil
	}
	t.Cleanup(func() {
		newVaultWatcherFunc = previousWatcherFactory
	})

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
	fakeWatcher.Emit(fsnotify.Event{Name: filepath.ToSlash(relPath), Op: fsnotify.Write})
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
	runtime, err := openRuntimeForTest(t, workDir)
	if err != nil {
		t.Fatalf("OpenRuntime() error = %v", err)
	}
	runtime.Config.Vault.DebounceWindow = 0

	now := time.Date(2026, 4, 23, 9, 0, 0, 0, time.Local)
	if _, err := runtime.Bootstrap(now); err != nil {
		t.Fatalf("Bootstrap() error = %v", err)
	}

	fakeWatcher := newFakeVaultEventWatcher()
	previousWatcherFactory := newVaultWatcherFunc
	newVaultWatcherFunc = func(root string) (vaultEventWatcher, error) {
		return fakeWatcher, nil
	}
	t.Cleanup(func() {
		newVaultWatcherFunc = previousWatcherFactory
	})

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
	fakeWatcher.Emit(fsnotify.Event{Name: ".obsidian/04-execution/week.md", Op: fsnotify.Write})
	fakeWatcher.WaitCollected(t)

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
	runtime, err := openRuntimeWithFakeProcessSinkSummarizer(t, workDir)
	if err != nil {
		t.Fatalf("OpenRuntime() error = %v", err)
	}
	runtime.Config.Vault.DebounceWindow = 50 * time.Millisecond

	secondEventAt := time.Now().In(loc).Add(-time.Minute).Truncate(time.Second)
	firstEventAt := secondEventAt.Truncate(30 * time.Minute).Add(-25 * time.Minute)
	secondWindowStart := secondEventAt.Truncate(30 * time.Minute)
	transcriptPath := filepath.Join(workDir, "daemon-codex-watch.jsonl")
	writeCodexJSONL(t, transcriptPath,
		`{"timestamp":"`+firstEventAt.Format(time.RFC3339)+`","type":"session_meta","payload":{"id":"session-daemon-watch","agent_nickname":"Codex"}}`,
		`{"timestamp":"`+firstEventAt.Format(time.RFC3339)+`","type":"event_msg","payload":{"type":"user_message","message":"first watcher sync"}}`,
	)
	fakeWatcher := newFakeFileEventWatcher(transcriptPath)
	previousWatcherFactory := newSingleFileWatcherFunc
	newSingleFileWatcherFunc = func(path string) (fileEventWatcher, error) {
		return fakeWatcher, nil
	}
	t.Cleanup(func() {
		newSingleFileWatcherFunc = previousWatcherFactory
	})
	previousCodexDebounce := codexWatchDebounce
	codexWatchDebounce = 1 * time.Millisecond
	t.Cleanup(func() {
		codexWatchDebounce = previousCodexDebounce
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
	if _, err := file.WriteString(`{"timestamp":"` + secondEventAt.Format(time.RFC3339) + `","type":"event_msg","payload":{"type":"agent_message","phase":"commentary","message":"second watcher sync"}}` + "\n"); err != nil {
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
	fakeWatcher.WaitMatched(t)

	if !waitForConditionOK(4*time.Second, func() bool {
		return strings.Count(output.String(), "Codex JSONL synced") >= 2
	}) {
		t.Fatalf("timed out waiting for second Codex JSONL sync; output:\n%s", output.String())
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

	view, err := runtime.ProcessSinkDay("codex", model.NormalizeDay(secondEventAt))
	if err != nil {
		t.Fatalf("ProcessSinkDay() error = %v", err)
	}
	foundWatcherWindow := false
	for _, checkpoint := range view.Checkpoints {
		if checkpoint.Window.WindowStart.Equal(secondWindowStart) {
			foundWatcherWindow = true
			break
		}
	}
	if !foundWatcherWindow {
		t.Fatalf("expected watcher-triggered %s checkpoint, got %+v", secondWindowStart.Format(time.RFC3339), view.Checkpoints)
	}

	if elapsed := time.Since(startedAt); elapsed >= 5*time.Second {
		t.Fatalf("daemon elapsed = %s, want watcher-triggered codex sync before poll interval", elapsed)
	}
}

type fakeFileEventWatcher struct {
	path    string
	events  chan fsnotify.Event
	errors  chan error
	matched chan fsnotify.Event
}

func newFakeFileEventWatcher(path string) *fakeFileEventWatcher {
	return &fakeFileEventWatcher{
		path:    path,
		events:  make(chan fsnotify.Event, 8),
		errors:  make(chan error, 1),
		matched: make(chan fsnotify.Event, 8),
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
	matched := sameWatchPath(event.Name, f.path)
	if matched {
		f.matched <- event
	}
	return matched
}

func (f *fakeFileEventWatcher) Emit(event fsnotify.Event) {
	f.events <- event
}

func (f *fakeFileEventWatcher) WaitMatched(t *testing.T) {
	t.Helper()
	select {
	case <-f.matched:
	case <-time.After(2 * time.Second):
		t.Fatal("file watcher event was not matched")
	}
}

type fakeVaultEventWatcher struct {
	events    chan fsnotify.Event
	errors    chan error
	collected chan fsnotify.Event
}

func newFakeVaultEventWatcher() *fakeVaultEventWatcher {
	return &fakeVaultEventWatcher{
		events:    make(chan fsnotify.Event, 8),
		errors:    make(chan error, 1),
		collected: make(chan fsnotify.Event, 8),
	}
}

func (f *fakeVaultEventWatcher) Events() <-chan fsnotify.Event {
	return f.events
}

func (f *fakeVaultEventWatcher) Errors() <-chan error {
	return f.errors
}

func (f *fakeVaultEventWatcher) Close() error {
	close(f.events)
	close(f.errors)
	return nil
}

func (f *fakeVaultEventWatcher) CollectPaths(event fsnotify.Event) ([]string, error) {
	f.collected <- event
	relPath := vault.NormalizeRelativePath(event.Name)
	if relPath == "" || vault.ShouldIgnoreRelativePath(relPath) {
		return nil, nil
	}
	if strings.ToLower(filepath.Ext(relPath)) != ".md" {
		return nil, nil
	}
	return []string{relPath}, nil
}

func (f *fakeVaultEventWatcher) Emit(event fsnotify.Event) {
	f.events <- event
}

func (f *fakeVaultEventWatcher) WaitCollected(t *testing.T) {
	t.Helper()
	select {
	case <-f.collected:
	case <-time.After(2 * time.Second):
		t.Fatal("vault watcher event was not collected")
	}
}

func waitForCondition(t *testing.T, timeout time.Duration, condition func() bool) {
	t.Helper()
	if waitForConditionOK(timeout, condition) {
		return
	}
	t.Fatalf("condition was not met within %s", timeout)
}

func waitForConditionOK(timeout time.Duration, condition func() bool) bool {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if condition() {
			return true
		}
		time.Sleep(25 * time.Millisecond)
	}
	return false
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

func openRuntimeForTest(t *testing.T, workDir string) (*Runtime, error) {
	t.Helper()

	runtime, err := OpenRuntimeWithConfigOptions(workDir, configtest.IsolatedOptions(t))
	if err != nil {
		return nil, err
	}
	t.Cleanup(func() {
		if err := runtime.Close(); err != nil {
			t.Fatalf("runtime.Close() error = %v", err)
		}
	})
	return runtime, nil
}
