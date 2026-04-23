package app

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

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

	var output bytes.Buffer
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
