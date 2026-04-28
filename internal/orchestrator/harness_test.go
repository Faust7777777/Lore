package orchestrator

import (
	"encoding/json"
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

	records, err := st.Audit().ListAudit(20)
	if err != nil {
		t.Fatalf("ListAudit() error = %v", err)
	}
	wantKinds := map[model.AuditKind]bool{
		model.AuditDraftCreated:     false,
		model.AuditDraftStateChange: false,
		model.AuditDraftApplied:     false,
	}
	for _, record := range records {
		if record.CorrelationID != draft.ID {
			continue
		}
		if _, ok := wantKinds[record.Kind]; ok {
			wantKinds[record.Kind] = true
		}
	}
	for kind, found := range wantKinds {
		if !found {
			t.Fatalf("missing %s audit record with correlation_id %q in %+v", kind, draft.ID, records)
		}
	}
}

func TestProposePersonaUpdateCreatesPendingDraftWithoutWritingPersona(t *testing.T) {
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
	personaAbs := filepath.Join(cfg.Paths.VaultRoot, cfg.Vault.ManagedCore.Persona)
	before, err := os.ReadFile(personaAbs)
	if err != nil {
		t.Fatalf("ReadFile(persona before) error = %v", err)
	}
	at := time.Date(2026, 4, 28, 10, 30, 0, 0, time.UTC)

	result, err := h.ProposePersonaUpdate(model.PersonaUpdateProposal{
		Field:         "education.major",
		CurrentValue:  "",
		ProposedValue: "电子商务",
		Evidence:      "用户说：我是大连理工大学学生，专业电子商务",
		Reason:        "这是用户长期教育背景事实",
		Confidence:    "high",
		Source:        "external_agent",
		ObservedAt:    at,
	}, at)
	if err != nil {
		t.Fatalf("ProposePersonaUpdate() error = %v", err)
	}
	if result.Status != "draft_created" || result.DraftID == "" || result.Target != cfg.Vault.ManagedCore.Persona || !result.ReviewRequired {
		t.Fatalf("result = %+v, want draft_created review-required persona target", result)
	}
	after, err := os.ReadFile(personaAbs)
	if err != nil {
		t.Fatalf("ReadFile(persona after) error = %v", err)
	}
	if string(after) != string(before) {
		t.Fatalf("persona document changed on proposal creation:\nbefore=%s\nafter=%s", before, after)
	}

	draft, err := st.Drafts().GetDraft(result.DraftID)
	if err != nil {
		t.Fatalf("GetDraft(%s) error = %v", result.DraftID, err)
	}
	if draft.Kind != model.DraftKindPersonaUpdate || draft.State != model.DraftPendingReview {
		t.Fatalf("draft kind/state = %s/%s, want persona_update/pending_review", draft.Kind, draft.State)
	}
	if draft.Target.Path != cfg.Vault.ManagedCore.Persona || draft.Target.Class != model.DocClassPersona || draft.Target.BaseVersion == "" {
		t.Fatalf("draft target = %+v, want persona target with base version", draft.Target)
	}
	if !strings.Contains(draft.Summary, "Proposal creation is not apply") {
		t.Fatalf("draft summary = %q, want no-apply warning", draft.Summary)
	}
	var payload model.PersonaUpdateProposal
	if err := json.Unmarshal([]byte(draft.ProposedContent), &payload); err != nil {
		t.Fatalf("decode ProposedContent error = %v\n%s", err, draft.ProposedContent)
	}
	if payload.Field != "education.major" || payload.ProposedValue != "电子商务" || payload.Confidence != "high" {
		t.Fatalf("payload = %+v", payload)
	}

	records, err := st.Audit().ListAudit(10)
	if err != nil {
		t.Fatalf("ListAudit() error = %v", err)
	}
	found := false
	for _, record := range records {
		if record.Kind == model.AuditDraftCreated && record.CorrelationID == result.DraftID && record.Target == cfg.Vault.ManagedCore.Persona {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("missing draft-created audit record for %s in %+v", result.DraftID, records)
	}
}

func TestApplyPersonaUpdateDraftAppendsReviewedRecord(t *testing.T) {
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
	personaAbs := filepath.Join(cfg.Paths.VaultRoot, cfg.Vault.ManagedCore.Persona)
	before, err := os.ReadFile(personaAbs)
	if err != nil {
		t.Fatalf("ReadFile(persona before) error = %v", err)
	}
	at := time.Date(2026, 4, 28, 10, 30, 0, 0, time.UTC)

	result, err := h.ProposePersonaUpdate(model.PersonaUpdateProposal{
		Field:         "education.major",
		CurrentValue:  "",
		ProposedValue: "电子商务",
		Evidence:      "用户说：我是大连理工大学学生，专业电子商务",
		Reason:        "这是用户长期教育背景事实",
		Confidence:    "high",
		Source:        "external_agent",
		ObservedAt:    at,
	}, at)
	if err != nil {
		t.Fatalf("ProposePersonaUpdate() error = %v", err)
	}
	if _, err := h.ApplyDraft(result.DraftID, at.Add(time.Minute)); !errors.Is(err, ErrDraftNotReady) {
		t.Fatalf("ApplyDraft(pending persona) error = %v, want ErrDraftNotReady", err)
	}
	if _, err := h.ApproveDraft(result.DraftID, at.Add(2*time.Minute)); err != nil {
		t.Fatalf("ApproveDraft(persona) error = %v", err)
	}
	applied, err := h.ApplyDraft(result.DraftID, at.Add(3*time.Minute))
	if err != nil {
		t.Fatalf("ApplyDraft(persona) error = %v", err)
	}
	if applied.State != model.DraftApplied {
		t.Fatalf("applied.State = %q, want applied", applied.State)
	}

	after, err := os.ReadFile(personaAbs)
	if err != nil {
		t.Fatalf("ReadFile(persona after) error = %v", err)
	}
	got := string(after)
	if !strings.HasPrefix(got, string(before)) {
		t.Fatalf("persona apply should append without rewriting existing content:\n%s", got)
	}
	for _, want := range []string{
		"## Applied Persona Updates",
		"field: education.major",
		"proposed_value: 电子商务",
		"evidence: 用户说：我是大连理工大学学生，专业电子商务",
		"reason: 这是用户长期教育背景事实",
		"confidence: high",
		"source: external_agent",
		"observed_at: 2026-04-28T10:30:00Z",
		"draft_id: " + result.DraftID,
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("persona content missing %q:\n%s", want, got)
		}
	}

	records, err := st.Audit().ListAudit(20)
	if err != nil {
		t.Fatalf("ListAudit() error = %v", err)
	}
	foundAppliedAudit := false
	for _, record := range records {
		if record.Kind == model.AuditDraftApplied && record.CorrelationID == result.DraftID && record.Target == cfg.Vault.ManagedCore.Persona {
			foundAppliedAudit = true
			break
		}
	}
	if !foundAppliedAudit {
		t.Fatalf("missing persona draft applied audit for %s in %+v", result.DraftID, records)
	}
}

func TestApplyPersonaUpdateDraftDetectsConflict(t *testing.T) {
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
	at := time.Date(2026, 4, 28, 10, 30, 0, 0, time.UTC)
	result, err := h.ProposePersonaUpdate(model.PersonaUpdateProposal{
		Field:         "education.major",
		ProposedValue: "电子商务",
		Evidence:      "用户说：我是大连理工大学学生，专业电子商务",
		Reason:        "这是用户长期教育背景事实",
		Confidence:    "high",
		Source:        "external_agent",
		ObservedAt:    at,
	}, at)
	if err != nil {
		t.Fatalf("ProposePersonaUpdate() error = %v", err)
	}
	if _, err := h.ApproveDraft(result.DraftID, at.Add(time.Minute)); err != nil {
		t.Fatalf("ApproveDraft(persona) error = %v", err)
	}
	personaAbs := filepath.Join(cfg.Paths.VaultRoot, cfg.Vault.ManagedCore.Persona)
	if _, err := vault.WriteFileAtomic(personaAbs, []byte("# changed externally"), cfg.Vault.TempSuffix); err != nil {
		t.Fatalf("WriteFileAtomic(personaAbs) error = %v", err)
	}

	if _, err := h.ApplyDraft(result.DraftID, at.Add(2*time.Minute)); !errorsIs(err, store.ErrConflict) {
		t.Fatalf("ApplyDraft(persona) error = %v, want store.ErrConflict", err)
	}
	updated, err := st.Drafts().GetDraft(result.DraftID)
	if err != nil {
		t.Fatalf("GetDraft() error = %v", err)
	}
	if updated.State != model.DraftConflicted {
		t.Fatalf("updated.State = %q, want conflicted", updated.State)
	}
}

func TestApplyPersonaUpdateDraftRejectsInvalidPayload(t *testing.T) {
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
	personaAbs := filepath.Join(cfg.Paths.VaultRoot, cfg.Vault.ManagedCore.Persona)
	before, err := os.ReadFile(personaAbs)
	if err != nil {
		t.Fatalf("ReadFile(persona before) error = %v", err)
	}
	_, baseVersion, err := vault.ReadFileWithHash(personaAbs)
	if err != nil {
		t.Fatalf("ReadFileWithHash(persona) error = %v", err)
	}
	at := time.Date(2026, 4, 28, 10, 30, 0, 0, time.UTC)
	draft := model.Draft{
		ID:    "draft-invalid-persona",
		Kind:  model.DraftKindPersonaUpdate,
		State: model.DraftApproved,
		Target: model.DocumentRef{
			Path:        cfg.Vault.ManagedCore.Persona,
			Class:       model.DocClassPersona,
			BaseVersion: baseVersion,
		},
		Title:           "Invalid persona update",
		Summary:         "invalid payload",
		ProposedContent: `{"field": "education.major"}`,
		CreatedAt:       at,
		UpdatedAt:       at,
	}
	if err := st.Drafts().SaveDraft(draft); err != nil {
		t.Fatalf("SaveDraft() error = %v", err)
	}

	if _, err := h.ApplyDraft(draft.ID, at.Add(time.Minute)); !errors.Is(err, ErrInvalidDraftPatch) {
		t.Fatalf("ApplyDraft(invalid persona) error = %v, want ErrInvalidDraftPatch", err)
	}
	after, err := os.ReadFile(personaAbs)
	if err != nil {
		t.Fatalf("ReadFile(persona after) error = %v", err)
	}
	if string(after) != string(before) {
		t.Fatalf("invalid persona apply changed document:\nbefore=%s\nafter=%s", before, after)
	}
	updated, err := st.Drafts().GetDraft(draft.ID)
	if err != nil {
		t.Fatalf("GetDraft() error = %v", err)
	}
	if updated.State != model.DraftApproved {
		t.Fatalf("updated.State = %q, want approved after invalid patch", updated.State)
	}
}

func TestApplyPersonaUpdateDraftRejectsNonPersonaTarget(t *testing.T) {
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
	noteRel := filepath.Join("03-notes", "forged.md")
	noteAbs := filepath.Join(cfg.Paths.VaultRoot, noteRel)
	before := []byte("# forged note\n")
	if _, err := vault.WriteFileAtomic(noteAbs, before, cfg.Vault.TempSuffix); err != nil {
		t.Fatalf("WriteFileAtomic(note) error = %v", err)
	}
	_, baseVersion, err := vault.ReadFileWithHash(noteAbs)
	if err != nil {
		t.Fatalf("ReadFileWithHash(note) error = %v", err)
	}
	at := time.Date(2026, 4, 28, 10, 30, 0, 0, time.UTC)
	payload, err := json.Marshal(model.PersonaUpdateProposal{
		Field:         "education.major",
		ProposedValue: "电子商务",
		Evidence:      "用户说：我是大连理工大学学生，专业电子商务",
		Reason:        "这是用户长期教育背景事实",
		Confidence:    "high",
		Source:        "external_agent",
		ObservedAt:    at,
	})
	if err != nil {
		t.Fatalf("Marshal(persona proposal) error = %v", err)
	}
	draft := model.Draft{
		ID:    "draft-forged-persona-target",
		Kind:  model.DraftKindPersonaUpdate,
		State: model.DraftApproved,
		Target: model.DocumentRef{
			Path:        noteRel,
			Class:       model.DocClassNote,
			BaseVersion: baseVersion,
		},
		Title:           "Forged persona target",
		Summary:         "must not write note",
		ProposedContent: string(payload),
		CreatedAt:       at,
		UpdatedAt:       at,
	}
	if err := st.Drafts().SaveDraft(draft); err != nil {
		t.Fatalf("SaveDraft() error = %v", err)
	}

	if _, err := h.ApplyDraft(draft.ID, at.Add(time.Minute)); !errors.Is(err, ErrInvalidDraftPatch) {
		t.Fatalf("ApplyDraft(forged persona target) error = %v, want ErrInvalidDraftPatch", err)
	}
	after, err := os.ReadFile(noteAbs)
	if err != nil {
		t.Fatalf("ReadFile(note after) error = %v", err)
	}
	if string(after) != string(before) {
		t.Fatalf("forged persona apply changed note:\nbefore=%s\nafter=%s", before, after)
	}
	updated, err := st.Drafts().GetDraft(draft.ID)
	if err != nil {
		t.Fatalf("GetDraft() error = %v", err)
	}
	if updated.State != model.DraftApproved {
		t.Fatalf("updated.State = %q, want approved after invalid target", updated.State)
	}
}

func TestApplyPersonaUpdateDraftRejectsPersonaPathWithWrongClass(t *testing.T) {
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
	personaAbs := filepath.Join(cfg.Paths.VaultRoot, cfg.Vault.ManagedCore.Persona)
	before, err := os.ReadFile(personaAbs)
	if err != nil {
		t.Fatalf("ReadFile(persona before) error = %v", err)
	}
	_, baseVersion, err := vault.ReadFileWithHash(personaAbs)
	if err != nil {
		t.Fatalf("ReadFileWithHash(persona) error = %v", err)
	}
	at := time.Date(2026, 4, 28, 10, 30, 0, 0, time.UTC)
	payload, err := json.Marshal(model.PersonaUpdateProposal{
		Field:         "education.major",
		ProposedValue: "电子商务",
		Evidence:      "用户说：我是大连理工大学学生，专业电子商务",
		Reason:        "这是用户长期教育背景事实",
		Confidence:    "high",
		Source:        "external_agent",
		ObservedAt:    at,
	})
	if err != nil {
		t.Fatalf("Marshal(persona proposal) error = %v", err)
	}
	draft := model.Draft{
		ID:    "draft-wrong-persona-class",
		Kind:  model.DraftKindPersonaUpdate,
		State: model.DraftApproved,
		Target: model.DocumentRef{
			Path:        cfg.Vault.ManagedCore.Persona,
			Class:       model.DocClassNote,
			BaseVersion: baseVersion,
		},
		Title:           "Wrong persona class",
		Summary:         "must not write persona",
		ProposedContent: string(payload),
		CreatedAt:       at,
		UpdatedAt:       at,
	}
	if err := st.Drafts().SaveDraft(draft); err != nil {
		t.Fatalf("SaveDraft() error = %v", err)
	}

	if _, err := h.ApplyDraft(draft.ID, at.Add(time.Minute)); !errors.Is(err, ErrInvalidDraftPatch) {
		t.Fatalf("ApplyDraft(wrong class) error = %v, want ErrInvalidDraftPatch", err)
	}
	after, err := os.ReadFile(personaAbs)
	if err != nil {
		t.Fatalf("ReadFile(persona after) error = %v", err)
	}
	if string(after) != string(before) {
		t.Fatalf("wrong-class persona apply changed persona:\nbefore=%s\nafter=%s", before, after)
	}
	updated, err := st.Drafts().GetDraft(draft.ID)
	if err != nil {
		t.Fatalf("GetDraft() error = %v", err)
	}
	if updated.State != model.DraftApproved {
		t.Fatalf("updated.State = %q, want approved after invalid target class", updated.State)
	}
}

func TestApplyPersonaUpdateDraftRejectsInvalidConfidence(t *testing.T) {
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
	personaAbs := filepath.Join(cfg.Paths.VaultRoot, cfg.Vault.ManagedCore.Persona)
	before, err := os.ReadFile(personaAbs)
	if err != nil {
		t.Fatalf("ReadFile(persona before) error = %v", err)
	}
	_, baseVersion, err := vault.ReadFileWithHash(personaAbs)
	if err != nil {
		t.Fatalf("ReadFileWithHash(persona) error = %v", err)
	}
	at := time.Date(2026, 4, 28, 10, 30, 0, 0, time.UTC)
	payload, err := json.Marshal(model.PersonaUpdateProposal{
		Field:         "education.major",
		ProposedValue: "电子商务",
		Evidence:      "用户说：我是大连理工大学学生，专业电子商务",
		Reason:        "这是用户长期教育背景事实",
		Confidence:    "certain",
		Source:        "external_agent",
		ObservedAt:    at,
	})
	if err != nil {
		t.Fatalf("Marshal(persona proposal) error = %v", err)
	}
	draft := model.Draft{
		ID:    "draft-invalid-confidence",
		Kind:  model.DraftKindPersonaUpdate,
		State: model.DraftApproved,
		Target: model.DocumentRef{
			Path:        cfg.Vault.ManagedCore.Persona,
			Class:       model.DocClassPersona,
			BaseVersion: baseVersion,
		},
		Title:           "Invalid confidence",
		Summary:         "must not write persona",
		ProposedContent: string(payload),
		CreatedAt:       at,
		UpdatedAt:       at,
	}
	if err := st.Drafts().SaveDraft(draft); err != nil {
		t.Fatalf("SaveDraft() error = %v", err)
	}

	if _, err := h.ApplyDraft(draft.ID, at.Add(time.Minute)); !errors.Is(err, ErrInvalidDraftPatch) {
		t.Fatalf("ApplyDraft(invalid confidence) error = %v, want ErrInvalidDraftPatch", err)
	}
	after, err := os.ReadFile(personaAbs)
	if err != nil {
		t.Fatalf("ReadFile(persona after) error = %v", err)
	}
	if string(after) != string(before) {
		t.Fatalf("invalid-confidence persona apply changed persona:\nbefore=%s\nafter=%s", before, after)
	}
	updated, err := st.Drafts().GetDraft(draft.ID)
	if err != nil {
		t.Fatalf("GetDraft() error = %v", err)
	}
	if updated.State != model.DraftApproved {
		t.Fatalf("updated.State = %q, want approved after invalid confidence", updated.State)
	}
}

func TestApplyPersonaUpdateDraftInsertsIntoExistingSection(t *testing.T) {
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
	personaAbs := filepath.Join(cfg.Paths.VaultRoot, cfg.Vault.ManagedCore.Persona)
	persona := "# 人物画像\n\n## Applied Persona Updates\n\n- field: existing\n  proposed_value: old\n\n## Freeform Notes\n\n- keep this section last\n"
	if _, err := vault.WriteFileAtomic(personaAbs, []byte(persona), cfg.Vault.TempSuffix); err != nil {
		t.Fatalf("WriteFileAtomic(persona) error = %v", err)
	}
	at := time.Date(2026, 4, 28, 10, 30, 0, 0, time.UTC)
	result, err := h.ProposePersonaUpdate(model.PersonaUpdateProposal{
		Field:         "education.major",
		ProposedValue: "电子商务",
		Evidence:      "用户说：我是大连理工大学学生，专业电子商务",
		Reason:        "这是用户长期教育背景事实",
		Confidence:    "high",
		Source:        "external_agent",
		ObservedAt:    at,
	}, at)
	if err != nil {
		t.Fatalf("ProposePersonaUpdate() error = %v", err)
	}
	if _, err := h.ApproveDraft(result.DraftID, at.Add(time.Minute)); err != nil {
		t.Fatalf("ApproveDraft(persona) error = %v", err)
	}
	if _, err := h.ApplyDraft(result.DraftID, at.Add(2*time.Minute)); err != nil {
		t.Fatalf("ApplyDraft(persona) error = %v", err)
	}

	after, err := os.ReadFile(personaAbs)
	if err != nil {
		t.Fatalf("ReadFile(persona after) error = %v", err)
	}
	got := string(after)
	if strings.Count(got, "## Applied Persona Updates") != 1 {
		t.Fatalf("persona content should keep one Applied Persona Updates heading:\n%s", got)
	}
	if strings.Index(got, "proposed_value: 电子商务") > strings.Index(got, "## Freeform Notes") {
		t.Fatalf("new persona update should be inserted before next section:\n%s", got)
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
