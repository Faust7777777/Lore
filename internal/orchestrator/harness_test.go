package orchestrator

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"obsidian-harness/internal/config"
	"obsidian-harness/internal/model"
	"obsidian-harness/internal/store"
	"obsidian-harness/internal/store/memory"
	"obsidian-harness/internal/vault"
)

func TestBootstrapAndDraftLifecycle(t *testing.T) {
	workDir := t.TempDir()
	cfg := config.Default(workDir)
	st := memory.New()

	h, err := New(cfg, st)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	created, err := h.BootstrapManagedVault(time.Date(2026, 4, 22, 9, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatalf("BootstrapManagedVault() error = %v", err)
	}
	if len(created) < 3 {
		t.Fatalf("created = %d, want at least 3 managed docs", len(created))
	}

	planPath := filepath.Join(cfg.Paths.VaultRoot, "0-\u6392\u671f", "04-\u6267\u884c", "week.md")
	if _, err := vault.WriteFileAtomic(planPath, []byte("# plan update"), cfg.Vault.TempSuffix); err != nil {
		t.Fatalf("WriteFileAtomic(plan) error = %v", err)
	}

	draft, err := h.ObserveDocumentChange(filepath.Join("0-\u6392\u671f", "04-\u6267\u884c", "week.md"), []byte("learn SQL triggers"), time.Date(2026, 4, 22, 10, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatalf("ObserveDocumentChange() error = %v", err)
	}
	if draft.State != model.DraftPendingReview {
		t.Fatalf("draft.State = %q, want %q", draft.State, model.DraftPendingReview)
	}

	approved, err := h.ApproveDraft(draft.ID, time.Date(2026, 4, 22, 10, 5, 0, 0, time.UTC))
	if err != nil {
		t.Fatalf("ApproveDraft() error = %v", err)
	}
	if approved.State != model.DraftApproved {
		t.Fatalf("approved.State = %q, want %q", approved.State, model.DraftApproved)
	}

	applied, err := h.ApplyDraft(draft.ID, time.Date(2026, 4, 22, 10, 6, 0, 0, time.UTC))
	if err != nil {
		t.Fatalf("ApplyDraft() error = %v", err)
	}
	if applied.State != model.DraftApplied {
		t.Fatalf("applied.State = %q, want %q", applied.State, model.DraftApplied)
	}

	progressAbs := filepath.Join(cfg.Paths.VaultRoot, cfg.Vault.ManagedCore.ProgressIndex)
	progressContent, err := os.ReadFile(progressAbs)
	if err != nil {
		t.Fatalf("ReadFile(progress) error = %v", err)
	}
	if !strings.Contains(string(progressContent), "Auto Progress Sync") {
		t.Fatalf("progress index missing auto sync block: %s", string(progressContent))
	}
}

func TestApplyDraftDetectsConflict(t *testing.T) {
	workDir := t.TempDir()
	cfg := config.Default(workDir)
	st := memory.New()

	h, err := New(cfg, st)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	if _, err := h.BootstrapManagedVault(time.Date(2026, 4, 22, 9, 0, 0, 0, time.UTC)); err != nil {
		t.Fatalf("BootstrapManagedVault() error = %v", err)
	}

	draft, err := h.ObserveDocumentChange(filepath.Join("0-\u6392\u671f", "04-\u6267\u884c", "week.md"), []byte("learn SQL triggers"), time.Date(2026, 4, 22, 10, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatalf("ObserveDocumentChange() error = %v", err)
	}
	if _, err := h.ApproveDraft(draft.ID, time.Date(2026, 4, 22, 10, 5, 0, 0, time.UTC)); err != nil {
		t.Fatalf("ApproveDraft() error = %v", err)
	}

	progressAbs := filepath.Join(cfg.Paths.VaultRoot, cfg.Vault.ManagedCore.ProgressIndex)
	if _, err := vault.WriteFileAtomic(progressAbs, []byte("# changed externally"), cfg.Vault.TempSuffix); err != nil {
		t.Fatalf("WriteFileAtomic(progressAbs) error = %v", err)
	}

	if _, err := h.ApplyDraft(draft.ID, time.Date(2026, 4, 22, 10, 6, 0, 0, time.UTC)); !errorsIs(err, store.ErrConflict) {
		t.Fatalf("ApplyDraft() error = %v, want store.ErrConflict", err)
	}

	updated, err := st.Drafts().GetDraft(draft.ID)
	if err != nil {
		t.Fatalf("GetDraft() error = %v", err)
	}
	if updated.State != model.DraftConflicted {
		t.Fatalf("updated.State = %q, want %q", updated.State, model.DraftConflicted)
	}
}

func TestProcessSinkChainWritesCheckpointAndDailyReport(t *testing.T) {
	workDir := t.TempDir()
	cfg := config.Default(workDir)
	st := memory.New()

	h, err := New(cfg, st)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	windowStart := time.Date(2026, 4, 22, 14, 0, 0, 0, time.UTC)
	window := model.SessionWindow{
		AgentID:     "codex",
		SessionID:   "thread-1",
		WindowStart: windowStart,
		WindowEnd:   windowStart.Add(30 * time.Minute),
	}

	checkpoint, err := h.IngestSessionWindow(window, "afternoon checkpoint", "summarized coding work", "raw transcript", window.WindowEnd)
	if err != nil {
		t.Fatalf("IngestSessionWindow() error = %v", err)
	}
	if checkpoint.State != model.CheckpointMaterialized {
		t.Fatalf("checkpoint.State = %q, want %q", checkpoint.State, model.CheckpointMaterialized)
	}

	report, err := h.RollupDaily("codex", windowStart, windowStart.Add(12*time.Hour))
	if err != nil {
		t.Fatalf("RollupDaily() error = %v", err)
	}
	if len(report.WindowKeys) != 1 {
		t.Fatalf("WindowKeys = %d, want 1", len(report.WindowKeys))
	}

	if _, err := os.Stat(checkpoint.Path); err != nil {
		t.Fatalf("checkpoint path not written: %v", err)
	}
	if _, err := os.Stat(report.Path); err != nil {
		t.Fatalf("daily report path not written: %v", err)
	}
}

func errorsIs(err error, target error) bool {
	return err != nil && target != nil && strings.Contains(err.Error(), target.Error())
}
