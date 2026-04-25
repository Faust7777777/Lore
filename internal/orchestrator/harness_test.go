package orchestrator

import (
	"errors"
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

func TestWriteLowRiskNoteWritesNoteAndAudits(t *testing.T) {
	workDir := t.TempDir()
	cfg := config.Default(workDir)
	st := memory.New()

	h, err := New(cfg, st)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	doc, err := h.WriteLowRiskNote("03-notes/diary.md", []byte("# Diary\n\nToday I shipped Lore."), false, time.Date(2026, 4, 22, 21, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatalf("WriteLowRiskNote() error = %v", err)
	}
	if doc.Path != "03-notes/diary.md" || doc.DocClass != model.DocClassNote || doc.BaseVersion == "" {
		t.Fatalf("doc = %+v, want note with hash", doc)
	}

	data, err := os.ReadFile(filepath.Join(cfg.Paths.VaultRoot, "03-notes", "diary.md"))
	if err != nil {
		t.Fatalf("ReadFile(note) error = %v", err)
	}
	if string(data) != "# Diary\n\nToday I shipped Lore." {
		t.Fatalf("note content = %q", string(data))
	}

	records, err := st.Audit().ListAudit(10)
	if err != nil {
		t.Fatalf("ListAuditRecords() error = %v", err)
	}
	found := false
	for _, record := range records {
		if record.Kind == model.AuditLowRiskVaultWrite && record.Target == "03-notes/diary.md" {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected low-risk write audit record, got %+v", records)
	}
}

func TestAuditFailureMarksHealthButDoesNotBlockWrite(t *testing.T) {
	workDir := t.TempDir()
	cfg := config.Default(workDir)
	st := failingAuditStateStore{Store: memory.New()}

	h, err := New(cfg, st)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	doc, err := h.WriteLowRiskNote("03-notes/audit-failure.md", []byte("# Audit Failure"), false, time.Date(2026, 4, 25, 9, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatalf("WriteLowRiskNote() error = %v", err)
	}
	if doc.Path != "03-notes/audit-failure.md" {
		t.Fatalf("doc.Path = %q", doc.Path)
	}

	snapshot := h.StatusSnapshot()
	if snapshot.Outcome.Status != model.StatusError {
		t.Fatalf("health status = %q, want error", snapshot.Outcome.Status)
	}
	if !strings.Contains(snapshot.Message, "audit record failed") {
		t.Fatalf("health message = %q, want audit failure", snapshot.Message)
	}
}

type failingAuditStateStore struct {
	*memory.Store
}

func (s failingAuditStateStore) Audit() store.AuditStore {
	return failingAuditStore{}
}

type failingAuditStore struct{}

func (failingAuditStore) AppendAudit(model.AuditRecord) error {
	return errors.New("forced audit failure")
}

func (failingAuditStore) ListAudit(int) ([]model.AuditRecord, error) {
	return nil, errors.New("forced audit failure")
}
func TestWriteLowRiskNoteRejectsGovernedPaths(t *testing.T) {
	workDir := t.TempDir()
	cfg := config.Default(workDir)
	st := memory.New()

	h, err := New(cfg, st)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	cases := []string{
		cfg.Vault.ManagedCore.SystemDoc,
		cfg.Vault.ManagedCore.ProgressIndex,
		cfg.Vault.ManagedCore.Persona,
		cfg.Vault.ManagedCore.AgentDoc,
		cfg.Vault.ManagedCore.IdentityDoc,
		filepath.Join("0-\u6392\u671f", "04-\u6267\u884c", "week.md"),
		filepath.ToSlash(filepath.Join("09-\u8fc7\u7a0b\u6c89\u6dc0", "codex", "2026-04-22.md")),
		".obsidian/private.md",
		"../escape.md",
		"notes/raw.txt",
	}
	for _, path := range cases {
		if _, err := h.WriteLowRiskNote(path, []byte("blocked"), true, time.Now()); !errors.Is(err, ErrDirectWriteDenied) {
			t.Fatalf("WriteLowRiskNote(%q) error = %v, want ErrDirectWriteDenied", path, err)
		}
	}
}

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
	if len(created) < 5 {
		t.Fatalf("created = %d, want at least 5 managed docs", len(created))
	}
	for _, relPath := range []string{
		cfg.Vault.ManagedCore.SystemDoc,
		cfg.Vault.ManagedCore.ProgressIndex,
		cfg.Vault.ManagedCore.Persona,
		cfg.Vault.ManagedCore.AgentDoc,
		cfg.Vault.ManagedCore.IdentityDoc,
	} {
		if _, err := os.Stat(filepath.Join(cfg.Paths.VaultRoot, relPath)); err != nil {
			t.Fatalf("expected managed doc %q to exist: %v", relPath, err)
		}
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
	if !strings.Contains(string(progressContent), "| 0-\u6392\u671f/04-\u6267\u884c/week.md | \u5468\u6267\u884c | \u5df2\u540c\u6b65 | 2026-04-22 10:00 |") {
		t.Fatalf("progress index missing structured progress row: %s", string(progressContent))
	}
}

func TestApplyDraftUpsertsProgressRowInsteadOfAppendingDuplicateBlock(t *testing.T) {
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

	relPath := filepath.Join("0-\u6392\u671f", "04-\u6267\u884c", "week.md")
	firstDraft, err := h.ObserveDocumentChange(relPath, []byte("first pass"), time.Date(2026, 4, 22, 10, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatalf("ObserveDocumentChange(first) error = %v", err)
	}
	if _, err := h.ApproveDraft(firstDraft.ID, time.Date(2026, 4, 22, 10, 5, 0, 0, time.UTC)); err != nil {
		t.Fatalf("ApproveDraft(first) error = %v", err)
	}
	if _, err := h.ApplyDraft(firstDraft.ID, time.Date(2026, 4, 22, 10, 6, 0, 0, time.UTC)); err != nil {
		t.Fatalf("ApplyDraft(first) error = %v", err)
	}

	secondDraft, err := h.ObserveDocumentChange(relPath, []byte("second pass"), time.Date(2026, 4, 22, 12, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatalf("ObserveDocumentChange(second) error = %v", err)
	}
	if _, err := h.ApproveDraft(secondDraft.ID, time.Date(2026, 4, 22, 12, 5, 0, 0, time.UTC)); err != nil {
		t.Fatalf("ApproveDraft(second) error = %v", err)
	}
	if _, err := h.ApplyDraft(secondDraft.ID, time.Date(2026, 4, 22, 12, 6, 0, 0, time.UTC)); err != nil {
		t.Fatalf("ApplyDraft(second) error = %v", err)
	}

	progressAbs := filepath.Join(cfg.Paths.VaultRoot, cfg.Vault.ManagedCore.ProgressIndex)
	progressContent, err := os.ReadFile(progressAbs)
	if err != nil {
		t.Fatalf("ReadFile(progress) error = %v", err)
	}
	got := string(progressContent)
	if strings.Count(got, "0-\u6392\u671f/04-\u6267\u884c/week.md") != 1 {
		t.Fatalf("expected one upserted progress row, got %s", got)
	}
	if !strings.Contains(got, "| 0-\u6392\u671f/04-\u6267\u884c/week.md | \u5468\u6267\u884c | \u5df2\u540c\u6b65 | 2026-04-22 12:00 |") {
		t.Fatalf("progress row not updated to latest timestamp: %s", got)
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

func TestRejectDraftTransitionsPendingReviewToRejected(t *testing.T) {
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

	draft, err := h.ObserveDocumentChange(filepath.Join("0-\u6392\u671f", "04-\u6267\u884c", "week.md"), []byte("reject me"), time.Date(2026, 4, 22, 10, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatalf("ObserveDocumentChange() error = %v", err)
	}

	rejected, err := h.RejectDraft(draft.ID, time.Date(2026, 4, 22, 10, 5, 0, 0, time.UTC))
	if err != nil {
		t.Fatalf("RejectDraft() error = %v", err)
	}
	if rejected.State != model.DraftRejected {
		t.Fatalf("rejected.State = %q, want %q", rejected.State, model.DraftRejected)
	}
}

func TestRequestDraftRevisionTransitionsPendingReviewToRevisionRequested(t *testing.T) {
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

	draft, err := h.ObserveDocumentChange(filepath.Join("0-\u6392\u671f", "04-\u6267\u884c", "week.md"), []byte("revise me"), time.Date(2026, 4, 22, 10, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatalf("ObserveDocumentChange() error = %v", err)
	}

	revised, err := h.RequestDraftRevision(draft.ID, time.Date(2026, 4, 22, 10, 5, 0, 0, time.UTC))
	if err != nil {
		t.Fatalf("RequestDraftRevision() error = %v", err)
	}
	if revised.State != model.DraftRevisionRequested {
		t.Fatalf("revised.State = %q, want %q", revised.State, model.DraftRevisionRequested)
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
