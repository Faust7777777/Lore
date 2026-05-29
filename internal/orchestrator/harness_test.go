package orchestrator

import (
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
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

func requireSymlink(t *testing.T, target string, link string) {
	t.Helper()
	if err := os.Symlink(target, link); err != nil {
		t.Skipf("symlink creation unavailable in this environment: %v", err)
	}
}

func requireDirectoryLink(t *testing.T, target string, link string) {
	t.Helper()
	if err := os.Symlink(target, link); err == nil {
		return
	}
	if runtime.GOOS == "windows" {
		cmd := exec.Command("cmd", "/c", "mklink", "/J", link, target)
		if output, err := cmd.CombinedOutput(); err == nil {
			return
		} else {
			t.Skipf("directory symlink/junction creation unavailable in this environment: %v: %s", err, string(output))
		}
	}
	t.Skipf("directory symlink creation unavailable in this environment")
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

func TestWriteLowRiskNoteRejectsParentSymlinkOutsideRoot(t *testing.T) {
	workDir := t.TempDir()
	cfg := config.Default(workDir)
	st := memory.New()

	h, err := New(cfg, st)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	if err := os.MkdirAll(cfg.Paths.VaultRoot, 0o755); err != nil {
		t.Fatalf("MkdirAll(vault) error = %v", err)
	}
	outsideDir := t.TempDir()
	requireDirectoryLink(t, outsideDir, filepath.Join(cfg.Paths.VaultRoot, "03-notes"))

	if _, err := h.WriteLowRiskNote("03-notes/leak.md", []byte("# Leak"), false, time.Now()); !errors.Is(err, fs.ErrPermission) {
		t.Fatalf("WriteLowRiskNote(parent symlink) error = %v, want fs.ErrPermission", err)
	}
	if _, err := os.Stat(filepath.Join(outsideDir, "leak.md")); !os.IsNotExist(err) {
		t.Fatalf("outside file stat error = %v, want not exist", err)
	}
}

func TestApplyDraftRejectsParentSymlinkOutsideRoot(t *testing.T) {
	workDir := t.TempDir()
	cfg := config.Default(workDir)
	st := memory.New()

	h, err := New(cfg, st)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	if err := os.MkdirAll(cfg.Paths.VaultRoot, 0o755); err != nil {
		t.Fatalf("MkdirAll(vault) error = %v", err)
	}
	outsideDir := t.TempDir()
	requireDirectoryLink(t, outsideDir, filepath.Join(cfg.Paths.VaultRoot, "03-notes"))

	at := time.Date(2026, 5, 25, 10, 0, 0, 0, time.UTC)
	proposal := model.MarkdownNoteProposal{
		TargetPath: "03-notes/leak.md",
		Title:      "Leak",
		Content:    "# Leak\n",
		SourceKind: "other",
		Evidence:   "security test",
		Reason:     "prove parent symlink rejection",
		Source:     "test",
		ObservedAt: at,
	}
	payload, err := json.MarshalIndent(proposal, "", "  ")
	if err != nil {
		t.Fatalf("MarshalIndent(proposal) error = %v", err)
	}
	draft := model.Draft{
		ID:    "draft-symlink-apply",
		Kind:  model.DraftKindMarkdownNoteWrite,
		State: model.DraftPendingReview,
		Target: model.DocumentRef{
			Path:        proposal.TargetPath,
			Class:       model.DocClassNote,
			BaseVersion: model.DraftBaseVersionNewFile,
		},
		Title:           "Markdown note proposal: Leak",
		Summary:         "security test",
		ProposedContent: string(payload),
		CreatedAt:       at,
		UpdatedAt:       at,
	}
	if err := st.Drafts().SaveDraft(draft); err != nil {
		t.Fatalf("SaveDraft() error = %v", err)
	}
	if _, err := h.ApproveDraft(draft.ID, at.Add(time.Minute)); err != nil {
		t.Fatalf("ApproveDraft() error = %v", err)
	}
	if _, err := h.ApplyDraft(draft.ID, at.Add(2*time.Minute)); !errors.Is(err, fs.ErrPermission) {
		t.Fatalf("ApplyDraft(parent symlink) error = %v, want fs.ErrPermission", err)
	}
	if _, err := os.Stat(filepath.Join(outsideDir, "leak.md")); !os.IsNotExist(err) {
		t.Fatalf("outside file stat error = %v, want not exist", err)
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

func TestProposeMarkdownNoteCreatesPendingDraftWithoutWritingNote(t *testing.T) {
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
	target := "03-notes/inbox/ecommerce-platforms.md"
	result, err := h.ProposeMarkdownNote(validMarkdownNoteProposal(target, at), at)
	if err != nil {
		t.Fatalf("ProposeMarkdownNote() error = %v", err)
	}
	if result.Status != "draft_created" || result.DraftID == "" || result.Target != target || !result.ReviewRequired {
		t.Fatalf("result = %+v, want draft_created review-required note target", result)
	}
	if _, err := os.Stat(filepath.Join(cfg.Paths.VaultRoot, filepath.FromSlash(target))); !os.IsNotExist(err) {
		t.Fatalf("target note exists after proposal creation: %v", err)
	}

	draft, err := st.Drafts().GetDraft(result.DraftID)
	if err != nil {
		t.Fatalf("GetDraft(%s) error = %v", result.DraftID, err)
	}
	if draft.Kind != model.DraftKindMarkdownNoteWrite || draft.State != model.DraftPendingReview {
		t.Fatalf("draft kind/state = %s/%s, want markdown_note_write/pending_review", draft.Kind, draft.State)
	}
	if draft.Target.Path != target || draft.Target.Class != model.DocClassNote || draft.Target.BaseVersion != model.DraftBaseVersionNewFile {
		t.Fatalf("draft target = %+v, want new note target", draft.Target)
	}
	if !strings.Contains(draft.Summary, "Proposal creation is not apply") || !strings.Contains(draft.Summary, "target note is unchanged") {
		t.Fatalf("draft summary missing no-apply warning: %q", draft.Summary)
	}
	var payload model.MarkdownNoteProposal
	if err := json.Unmarshal([]byte(draft.ProposedContent), &payload); err != nil {
		t.Fatalf("json.Unmarshal(ProposedContent) error = %v", err)
	}
	if payload.TargetPath != target || payload.Title != "E-commerce Platforms" || payload.SourceKind != "class" {
		t.Fatalf("proposal payload = %+v, want normalized proposal", payload)
	}
	if len(draft.EvidenceRefs) != 1 || draft.EvidenceRefs[0] != payload.Evidence {
		t.Fatalf("EvidenceRefs = %+v, want evidence", draft.EvidenceRefs)
	}

	records, err := st.Audit().ListAudit(20)
	if err != nil {
		t.Fatalf("ListAudit() error = %v", err)
	}
	found := false
	for _, record := range records {
		if record.Kind == model.AuditDraftCreated && record.CorrelationID == draft.ID && record.Target == target {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("missing draft-created audit for markdown note proposal in %+v", records)
	}
}

func TestProposeMarkdownNoteUsesExistingNoteBaseVersion(t *testing.T) {
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

	target := "03-notes/existing.md"
	_, baseVersion, err := writeVaultFile(t, cfg, target, "# Existing\n")
	if err != nil {
		t.Fatalf("writeVaultFile(existing) error = %v", err)
	}
	at := time.Date(2026, 4, 28, 10, 30, 0, 0, time.UTC)
	result, err := h.ProposeMarkdownNote(validMarkdownNoteProposal(target, at), at)
	if err != nil {
		t.Fatalf("ProposeMarkdownNote(existing) error = %v", err)
	}
	draft, err := st.Drafts().GetDraft(result.DraftID)
	if err != nil {
		t.Fatalf("GetDraft(%s) error = %v", result.DraftID, err)
	}
	if draft.Target.BaseVersion != baseVersion {
		t.Fatalf("BaseVersion = %q, want existing hash %q", draft.Target.BaseVersion, baseVersion)
	}
}

func TestProposeMarkdownNoteRejectsGovernedOrUnsafeTargets(t *testing.T) {
	at := time.Date(2026, 4, 28, 10, 30, 0, 0, time.UTC)
	longShortField := strings.Repeat("x", 513)
	longMediumField := strings.Repeat("x", 16*1024+1)
	longContent := strings.Repeat("x", 256*1024+1)

	cases := []struct {
		name   string
		mutate func(*model.MarkdownNoteProposal, config.Config)
	}{
		{name: "path escape", mutate: func(p *model.MarkdownNoteProposal, _ config.Config) { p.TargetPath = "../escape.md" }},
		{name: "drive path", mutate: func(p *model.MarkdownNoteProposal, _ config.Config) { p.TargetPath = `C:\temp\note.md` }},
		{name: "hidden path", mutate: func(p *model.MarkdownNoteProposal, _ config.Config) { p.TargetPath = ".hidden/note.md" }},
		{name: "agent doc", mutate: func(p *model.MarkdownNoteProposal, cfg config.Config) { p.TargetPath = cfg.Vault.ManagedCore.AgentDoc }},
		{name: "identity doc", mutate: func(p *model.MarkdownNoteProposal, cfg config.Config) {
			p.TargetPath = cfg.Vault.ManagedCore.IdentityDoc
		}},
		{name: "persona doc", mutate: func(p *model.MarkdownNoteProposal, cfg config.Config) { p.TargetPath = cfg.Vault.ManagedCore.Persona }},
		{name: "system doc", mutate: func(p *model.MarkdownNoteProposal, cfg config.Config) { p.TargetPath = cfg.Vault.ManagedCore.SystemDoc }},
		{name: "progress doc", mutate: func(p *model.MarkdownNoteProposal, cfg config.Config) {
			p.TargetPath = cfg.Vault.ManagedCore.ProgressIndex
		}},
		{name: "process sink path", mutate: func(p *model.MarkdownNoteProposal, _ config.Config) {
			p.TargetPath = "09-\u8fc7\u7a0b\u6c89\u6dc0/codex/daily/2026-04-28.md"
		}},
		{name: "plan path", mutate: func(p *model.MarkdownNoteProposal, _ config.Config) { p.TargetPath = "plans/week.md" }},
		{name: "non markdown", mutate: func(p *model.MarkdownNoteProposal, _ config.Config) { p.TargetPath = "03-notes/inbox/raw.txt" }},
		{name: "empty content", mutate: func(p *model.MarkdownNoteProposal, _ config.Config) { p.Content = " " }},
		{name: "empty title", mutate: func(p *model.MarkdownNoteProposal, _ config.Config) { p.Title = " " }},
		{name: "empty evidence", mutate: func(p *model.MarkdownNoteProposal, _ config.Config) { p.Evidence = " " }},
		{name: "empty reason", mutate: func(p *model.MarkdownNoteProposal, _ config.Config) { p.Reason = " " }},
		{name: "empty source", mutate: func(p *model.MarkdownNoteProposal, _ config.Config) { p.Source = " " }},
		{name: "invalid source kind", mutate: func(p *model.MarkdownNoteProposal, _ config.Config) { p.SourceKind = "shell" }},
		{name: "content too large", mutate: func(p *model.MarkdownNoteProposal, _ config.Config) { p.Content = longContent }},
		{name: "evidence too large", mutate: func(p *model.MarkdownNoteProposal, _ config.Config) { p.Evidence = longMediumField }},
		{name: "reason too large", mutate: func(p *model.MarkdownNoteProposal, _ config.Config) { p.Reason = longMediumField }},
		{name: "task context too large", mutate: func(p *model.MarkdownNoteProposal, _ config.Config) { p.TaskContext = longMediumField }},
		{name: "title too large", mutate: func(p *model.MarkdownNoteProposal, _ config.Config) { p.Title = longShortField }},
		{name: "course too large", mutate: func(p *model.MarkdownNoteProposal, _ config.Config) { p.Course = longShortField }},
		{name: "topic too large", mutate: func(p *model.MarkdownNoteProposal, _ config.Config) { p.Topic = longShortField }},
		{name: "source too large", mutate: func(p *model.MarkdownNoteProposal, _ config.Config) { p.Source = longShortField }},
		{name: "dedupe key too large", mutate: func(p *model.MarkdownNoteProposal, _ config.Config) { p.DedupeKey = longShortField }},
		{name: "missing observed at", mutate: func(p *model.MarkdownNoteProposal, _ config.Config) { p.ObservedAt = time.Time{} }},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
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

			proposal := validMarkdownNoteProposal("03-notes/inbox/ecommerce-platforms.md", at)
			tc.mutate(&proposal, cfg)
			if _, err := h.ProposeMarkdownNote(proposal, at); err == nil {
				t.Fatal("ProposeMarkdownNote() error = nil, want rejection")
			}
			drafts, err := st.Drafts().ListDrafts()
			if err != nil {
				t.Fatalf("ListDrafts() error = %v", err)
			}
			if len(drafts) != 0 {
				t.Fatalf("drafts = %+v, want no draft for invalid proposal", drafts)
			}
			defaultTarget := filepath.Join(cfg.Paths.VaultRoot, "03-notes", "inbox", "ecommerce-platforms.md")
			if _, err := os.Stat(defaultTarget); !os.IsNotExist(err) {
				t.Fatalf("default target exists after invalid proposal: %v", err)
			}
		})
	}
}

func TestApplyMarkdownNoteDraftRequiresApproval(t *testing.T) {
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
	target := "03-notes/inbox/ecommerce-platforms.md"
	result, err := h.ProposeMarkdownNote(validMarkdownNoteProposal(target, at), at)
	if err != nil {
		t.Fatalf("ProposeMarkdownNote() error = %v", err)
	}
	if _, err := h.ApplyDraft(result.DraftID, at.Add(time.Minute)); !errors.Is(err, ErrDraftNotReady) {
		t.Fatalf("ApplyDraft(pending markdown note) error = %v, want ErrDraftNotReady", err)
	}
	if _, err := os.Stat(filepath.Join(cfg.Paths.VaultRoot, filepath.FromSlash(target))); !os.IsNotExist(err) {
		t.Fatalf("target note exists after pending apply attempt: %v", err)
	}
}

func TestApplyMarkdownNoteDraftWritesNewNote(t *testing.T) {
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
	target := "03-notes/inbox/ecommerce-platforms.md"
	proposal := validMarkdownNoteProposal(target, at)
	proposal.Content = "# E-commerce Platforms\n\n- Marketplaces coordinate buyers and sellers."
	result, err := h.ProposeMarkdownNote(proposal, at)
	if err != nil {
		t.Fatalf("ProposeMarkdownNote() error = %v", err)
	}
	if _, err := h.ApproveDraft(result.DraftID, at.Add(time.Minute)); err != nil {
		t.Fatalf("ApproveDraft(markdown note) error = %v", err)
	}
	applied, err := h.ApplyDraft(result.DraftID, at.Add(2*time.Minute))
	if err != nil {
		t.Fatalf("ApplyDraft(markdown note) error = %v", err)
	}
	if applied.State != model.DraftApplied {
		t.Fatalf("applied.State = %q, want applied", applied.State)
	}

	data, err := os.ReadFile(filepath.Join(cfg.Paths.VaultRoot, filepath.FromSlash(target)))
	if err != nil {
		t.Fatalf("ReadFile(applied note) error = %v", err)
	}
	if string(data) != proposal.Content+"\n" {
		t.Fatalf("applied note content = %q, want reviewed content with trailing newline", string(data))
	}

	records, err := st.Audit().ListAudit(20)
	if err != nil {
		t.Fatalf("ListAudit() error = %v", err)
	}
	found := false
	for _, record := range records {
		if record.Kind == model.AuditDraftApplied && record.CorrelationID == result.DraftID && record.Target == target {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("missing draft-applied audit for markdown note in %+v", records)
	}
}

func TestSupersedeMarkdownNoteDraftCreatesRevisedDraft(t *testing.T) {
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
	originalTarget := "03-notes/inbox/ecommerce-platforms.md"
	result, err := h.ProposeMarkdownNote(validMarkdownNoteProposal(originalTarget, at), at)
	if err != nil {
		t.Fatalf("ProposeMarkdownNote() error = %v", err)
	}
	originalBefore, err := st.Drafts().GetDraft(result.DraftID)
	if err != nil {
		t.Fatalf("GetDraft(original) error = %v", err)
	}

	newTarget := "03-notes/class/ecommerce-platforms.md"
	revisedContent := "# E-commerce Platforms\n\n- Revised with persona-aware study framing."
	reason := "Aligned class note with learner weakness and clearer target path"
	revised, err := h.SupersedeDraft(result.DraftID, model.DraftSupersedeUpdate{
		TargetPath:      newTarget,
		ProposedContent: revisedContent,
		Summary:         "Move from inbox to class notes and tighten explanation",
		Reason:          reason,
	}, at.Add(time.Minute))
	if err != nil {
		t.Fatalf("SupersedeDraft() error = %v", err)
	}
	if revised.ID == result.DraftID || revised.State != model.DraftPendingReview || revised.Supersedes != result.DraftID {
		t.Fatalf("revised draft = %+v, want new pending draft superseding original", revised)
	}
	if revised.Target.Path != newTarget || revised.Target.Class != model.DocClassNote || revised.Target.BaseVersion != model.DraftBaseVersionNewFile {
		t.Fatalf("revised target = %+v, want fresh new-file note target", revised.Target)
	}

	originalAfter, err := st.Drafts().GetDraft(result.DraftID)
	if err != nil {
		t.Fatalf("GetDraft(original after) error = %v", err)
	}
	if originalAfter.State != model.DraftSuperseded {
		t.Fatalf("original state = %q, want superseded", originalAfter.State)
	}
	if originalAfter.ProposedContent != originalBefore.ProposedContent {
		t.Fatalf("original ProposedContent changed\nbefore: %s\nafter: %s", originalBefore.ProposedContent, originalAfter.ProposedContent)
	}

	var payload model.MarkdownNoteProposal
	if err := json.Unmarshal([]byte(revised.ProposedContent), &payload); err != nil {
		t.Fatalf("decode revised ProposedContent error = %v\n%s", err, revised.ProposedContent)
	}
	if payload.TargetPath != newTarget || payload.Content != revisedContent || payload.Title != "E-commerce Platforms" {
		t.Fatalf("revised payload = %+v, want updated path/content and preserved metadata", payload)
	}
	if _, err := os.Stat(filepath.Join(cfg.Paths.VaultRoot, filepath.FromSlash(newTarget))); !os.IsNotExist(err) {
		t.Fatalf("revised target exists after supersede: %v", err)
	}

	records, err := st.Audit().ListAudit(20)
	if err != nil {
		t.Fatalf("ListAudit() error = %v", err)
	}
	foundOld := false
	foundNew := false
	for _, record := range records {
		if record.Kind == model.AuditDraftStateChange && record.CorrelationID == result.DraftID && record.Target == originalTarget {
			foundOld = record.Metadata["state"] == string(model.DraftSuperseded) &&
				record.Metadata["superseded_by"] == revised.ID &&
				record.Metadata["reason"] == reason
		}
		if record.Kind == model.AuditDraftCreated && record.CorrelationID == revised.ID && record.Target == newTarget {
			foundNew = record.Metadata["supersedes"] == result.DraftID &&
				record.Metadata["reason"] == reason
		}
	}
	if !foundOld || !foundNew {
		t.Fatalf("missing supersede audit records old=%v new=%v in %+v", foundOld, foundNew, records)
	}
}

func TestSupersedeMarkdownNoteDraftRejectsUnsafeTarget(t *testing.T) {
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
	result, err := h.ProposeMarkdownNote(validMarkdownNoteProposal("03-notes/inbox/ecommerce-platforms.md", at), at)
	if err != nil {
		t.Fatalf("ProposeMarkdownNote() error = %v", err)
	}
	if _, err := h.SupersedeDraft(result.DraftID, model.DraftSupersedeUpdate{
		TargetPath:      cfg.Vault.ManagedCore.Persona,
		ProposedContent: "# Bad target\n",
		Reason:          "attempted managed core target",
	}, at.Add(time.Minute)); !errors.Is(err, ErrDirectWriteDenied) {
		t.Fatalf("SupersedeDraft(unsafe target) error = %v, want ErrDirectWriteDenied", err)
	}

	original, err := st.Drafts().GetDraft(result.DraftID)
	if err != nil {
		t.Fatalf("GetDraft(original) error = %v", err)
	}
	if original.State != model.DraftPendingReview {
		t.Fatalf("original state = %q, want pending_review after failed supersede", original.State)
	}
	drafts, err := st.Drafts().ListDrafts()
	if err != nil {
		t.Fatalf("ListDrafts() error = %v", err)
	}
	if len(drafts) != 1 {
		t.Fatalf("draft count after failed supersede = %d, want 1", len(drafts))
	}
}

func TestSupersedeDraftRejectsUnsupportedKind(t *testing.T) {
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
		Evidence:      "user stated major",
		Reason:        "long-term education profile",
		Confidence:    "high",
		Source:        "external_agent",
		ObservedAt:    at,
	}, at)
	if err != nil {
		t.Fatalf("ProposePersonaUpdate() error = %v", err)
	}
	if _, err := h.SupersedeDraft(result.DraftID, model.DraftSupersedeUpdate{
		ProposedContent: "# Not persona\n",
		Reason:          "unsupported kind",
	}, at.Add(time.Minute)); !errors.Is(err, ErrUnsupportedDraft) {
		t.Fatalf("SupersedeDraft(persona) error = %v, want ErrUnsupportedDraft", err)
	}
}

func TestApplyMarkdownNoteDraftDetectsNewFileConflict(t *testing.T) {
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
	target := "03-notes/inbox/ecommerce-platforms.md"
	result, err := h.ProposeMarkdownNote(validMarkdownNoteProposal(target, at), at)
	if err != nil {
		t.Fatalf("ProposeMarkdownNote() error = %v", err)
	}
	if _, err := h.ApproveDraft(result.DraftID, at.Add(time.Minute)); err != nil {
		t.Fatalf("ApproveDraft(markdown note) error = %v", err)
	}
	if _, _, err := writeVaultFile(t, cfg, target, "# Existing external file\n"); err != nil {
		t.Fatalf("writeVaultFile(conflict) error = %v", err)
	}
	if _, err := h.ApplyDraft(result.DraftID, at.Add(2*time.Minute)); !errorsIs(err, store.ErrConflict) {
		t.Fatalf("ApplyDraft(new file conflict) error = %v, want store.ErrConflict", err)
	}
	data, err := os.ReadFile(filepath.Join(cfg.Paths.VaultRoot, filepath.FromSlash(target)))
	if err != nil {
		t.Fatalf("ReadFile(conflict note) error = %v", err)
	}
	if string(data) != "# Existing external file\n" {
		t.Fatalf("conflict apply overwrote note: %q", string(data))
	}
	updated, err := st.Drafts().GetDraft(result.DraftID)
	if err != nil {
		t.Fatalf("GetDraft() error = %v", err)
	}
	if updated.State != model.DraftConflicted {
		t.Fatalf("updated.State = %q, want conflicted", updated.State)
	}
}

func TestApplyMarkdownNoteDraftDetectsExistingFileConflict(t *testing.T) {
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

	target := "03-notes/existing.md"
	if _, _, err := writeVaultFile(t, cfg, target, "# Original\n"); err != nil {
		t.Fatalf("writeVaultFile(original) error = %v", err)
	}
	at := time.Date(2026, 4, 28, 10, 30, 0, 0, time.UTC)
	result, err := h.ProposeMarkdownNote(validMarkdownNoteProposal(target, at), at)
	if err != nil {
		t.Fatalf("ProposeMarkdownNote(existing) error = %v", err)
	}
	if _, err := h.ApproveDraft(result.DraftID, at.Add(time.Minute)); err != nil {
		t.Fatalf("ApproveDraft(markdown note) error = %v", err)
	}
	if _, _, err := writeVaultFile(t, cfg, target, "# Changed externally\n"); err != nil {
		t.Fatalf("writeVaultFile(changed) error = %v", err)
	}
	if _, err := h.ApplyDraft(result.DraftID, at.Add(2*time.Minute)); !errorsIs(err, store.ErrConflict) {
		t.Fatalf("ApplyDraft(existing file conflict) error = %v, want store.ErrConflict", err)
	}
	data, err := os.ReadFile(filepath.Join(cfg.Paths.VaultRoot, filepath.FromSlash(target)))
	if err != nil {
		t.Fatalf("ReadFile(conflict note) error = %v", err)
	}
	if string(data) != "# Changed externally\n" {
		t.Fatalf("conflict apply overwrote note: %q", string(data))
	}
}

func TestApplyMarkdownNoteDraftRejectsUnsafeForgedTarget(t *testing.T) {
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

	personaAbs := filepath.Join(cfg.Paths.VaultRoot, filepath.FromSlash(cfg.Vault.ManagedCore.Persona))
	before, err := os.ReadFile(personaAbs)
	if err != nil {
		t.Fatalf("ReadFile(persona before) error = %v", err)
	}
	_, baseVersion, err := vault.ReadFileWithHash(personaAbs)
	if err != nil {
		t.Fatalf("ReadFileWithHash(persona) error = %v", err)
	}
	at := time.Date(2026, 4, 28, 10, 30, 0, 0, time.UTC)
	proposal := validMarkdownNoteProposal(cfg.Vault.ManagedCore.Persona, at)
	payload, err := json.Marshal(proposal)
	if err != nil {
		t.Fatalf("Marshal(markdown proposal) error = %v", err)
	}
	draft := model.Draft{
		ID:    "draft-forged-markdown-target",
		Kind:  model.DraftKindMarkdownNoteWrite,
		State: model.DraftApproved,
		Target: model.DocumentRef{
			Path:        cfg.Vault.ManagedCore.Persona,
			Class:       model.DocClassNote,
			BaseVersion: baseVersion,
		},
		Title:           "Forged markdown target",
		Summary:         "must not write persona",
		ProposedContent: string(payload),
		CreatedAt:       at,
		UpdatedAt:       at,
	}
	if err := st.Drafts().SaveDraft(draft); err != nil {
		t.Fatalf("SaveDraft() error = %v", err)
	}
	if _, err := h.ApplyDraft(draft.ID, at.Add(time.Minute)); !errors.Is(err, ErrDirectWriteDenied) && !errors.Is(err, ErrInvalidDraftPatch) {
		t.Fatalf("ApplyDraft(forged markdown target) error = %v, want governance rejection", err)
	}
	after, err := os.ReadFile(personaAbs)
	if err != nil {
		t.Fatalf("ReadFile(persona after) error = %v", err)
	}
	if string(after) != string(before) {
		t.Fatalf("forged markdown apply changed persona:\nbefore=%s\nafter=%s", before, after)
	}
	updated, err := st.Drafts().GetDraft(draft.ID)
	if err != nil {
		t.Fatalf("GetDraft() error = %v", err)
	}
	if updated.State != model.DraftApproved {
		t.Fatalf("updated.State = %q, want approved after invalid target", updated.State)
	}
}

func TestApplyMarkdownNoteDraftRejectsInvalidPayload(t *testing.T) {
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
	target := "03-notes/inbox/ecommerce-platforms.md"
	draft := model.Draft{
		ID:    "draft-invalid-markdown-payload",
		Kind:  model.DraftKindMarkdownNoteWrite,
		State: model.DraftApproved,
		Target: model.DocumentRef{
			Path:        target,
			Class:       model.DocClassNote,
			BaseVersion: model.DraftBaseVersionNewFile,
		},
		Title:           "Invalid markdown payload",
		Summary:         "must not write note",
		ProposedContent: `{"target_path":"03-notes/inbox/other.md","title":"Mismatch"}`,
		CreatedAt:       at,
		UpdatedAt:       at,
	}
	if err := st.Drafts().SaveDraft(draft); err != nil {
		t.Fatalf("SaveDraft() error = %v", err)
	}
	if _, err := h.ApplyDraft(draft.ID, at.Add(time.Minute)); !errors.Is(err, ErrInvalidDraftPatch) {
		t.Fatalf("ApplyDraft(invalid markdown payload) error = %v, want ErrInvalidDraftPatch", err)
	}
	if _, err := os.Stat(filepath.Join(cfg.Paths.VaultRoot, filepath.FromSlash(target))); !os.IsNotExist(err) {
		t.Fatalf("target note exists after invalid payload apply: %v", err)
	}
	updated, err := st.Drafts().GetDraft(draft.ID)
	if err != nil {
		t.Fatalf("GetDraft() error = %v", err)
	}
	if updated.State != model.DraftApproved {
		t.Fatalf("updated.State = %q, want approved after invalid payload", updated.State)
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

func validMarkdownNoteProposal(target string, observedAt time.Time) model.MarkdownNoteProposal {
	return model.MarkdownNoteProposal{
		TargetPath:  target,
		Title:       "E-commerce Platforms",
		Content:     "# E-commerce Platforms\n\n- Marketplaces coordinate buyers and sellers.",
		SourceKind:  "class",
		Evidence:    "class transcript discussed marketplace coordination",
		Reason:      "durable class note for later review",
		Source:      "external_agent",
		ObservedAt:  observedAt,
		TaskContext: "class note extraction",
		Course:      "E-commerce",
		Topic:       "platforms",
		DedupeKey:   "ecommerce-platforms-2026-04-28",
	}
}

func writeVaultFile(t *testing.T, cfg config.Config, relPath string, content string) ([]byte, string, error) {
	t.Helper()
	abs := filepath.Join(cfg.Paths.VaultRoot, filepath.FromSlash(relPath))
	hash, err := vault.WriteFileAtomic(abs, []byte(content), cfg.Vault.TempSuffix)
	if err != nil {
		return nil, "", err
	}
	data, _, err := vault.ReadFileWithHash(abs)
	if err != nil {
		return nil, "", err
	}
	return data, hash, nil
}

func TestProgressIndexTableBlockHeaderIsRecognized(t *testing.T) {
	// Regression: progressIndexTableBlock must emit a header that
	// isProgressHeaderLine accepts. If it does not, a freshly-created
	// progress table can never be found again, so later upserts append
	// duplicate tables instead of updating rows. progressIndexTableBlock
	// had 0% coverage, so a corrupt header literal slipped through.
	block := progressIndexTableBlock("| notes/db.md | system | synced | 2026-05-29 10:00 |")
	header := strings.SplitN(block, "\n", 2)[0]
	if !isProgressHeaderLine(header) {
		t.Fatalf("generated progress-index header not recognized by isProgressHeaderLine:\n%q", header)
	}
}

func TestUpsertProgressIndexRowReplacesSameDocInsteadOfDuplicating(t *testing.T) {
	// Creating a table then upserting another row for the same first
	// column must update that row in place and keep a single table -- the
	// downstream payoff of the header round-trip. Guards the duplicate-
	// table regression end to end.
	first, err := upsertProgressIndexRow(nil, "| notes/db.md | system | synced | 2026-05-29 10:00 |")
	if err != nil {
		t.Fatalf("first upsert: %v", err)
	}
	second, err := upsertProgressIndexRow(first, "| notes/db.md | system | synced | 2026-05-29 11:00 |")
	if err != nil {
		t.Fatalf("second upsert: %v", err)
	}
	out := string(second)
	if n := strings.Count(out, "| --- | --- | --- | --- |"); n != 1 {
		t.Fatalf("expected exactly one table (one separator row), got %d:\n%s", n, out)
	}
	if c := strings.Count(out, "notes/db.md"); c != 1 {
		t.Fatalf("the doc row should appear once (replaced in place), got %d:\n%s", c, out)
	}
	if !strings.Contains(out, "2026-05-29 11:00") {
		t.Fatalf("second upsert should update the row to the new timestamp:\n%s", out)
	}
}
