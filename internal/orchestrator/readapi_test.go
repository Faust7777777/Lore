package orchestrator

import (
	"path/filepath"
	"strings"
	"testing"
	"time"

	"obsidian-harness/internal/config"
	"obsidian-harness/internal/store/memory"
	"obsidian-harness/internal/vault"
)

func TestManagedStatusAndContextPack(t *testing.T) {
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

	targetPath := filepath.Join("0-\u6392\u671f", "04-\u6267\u884c", "demo-week.md")
	if _, err := vault.WriteFileAtomic(filepath.Join(cfg.Paths.VaultRoot, targetPath), []byte("# Demo Week\n\n[[\u4eba\u7269\u753b\u50cf]]\n\n![[assets/chart.png]]\n\nSQL triggers"), cfg.Vault.TempSuffix); err != nil {
		t.Fatalf("WriteFileAtomic(target) error = %v", err)
	}
	sourcePath := filepath.Join("03-notes", "daily-note.md")
	if _, err := vault.WriteFileAtomic(filepath.Join(cfg.Paths.VaultRoot, sourcePath), []byte("[[demo-week]] reference"), cfg.Vault.TempSuffix); err != nil {
		t.Fatalf("WriteFileAtomic(source) error = %v", err)
	}

	status, err := h.ManagedStatus()
	if err != nil {
		t.Fatalf("ManagedStatus() error = %v", err)
	}
	if !status.Ready {
		t.Fatal("ManagedStatus().Ready = false, want true")
	}
	if len(status.CoreDocs) < 5 {
		t.Fatalf("len(status.CoreDocs) = %d, want at least 5", len(status.CoreDocs))
	}

	agentDoc, err := h.SystemDocGet("agent")
	if err != nil {
		t.Fatalf("SystemDocGet(agent) error = %v", err)
	}
	if !strings.Contains(agentDoc.Content, "Workspace Agent Operating Manual") {
		t.Fatalf("agent doc content missing expected heading: %s", agentDoc.Content)
	}

	identityDoc, err := h.SystemDocGet("identity")
	if err != nil {
		t.Fatalf("SystemDocGet(identity) error = %v", err)
	}
	if !strings.Contains(identityDoc.Content, "Lore Identity") {
		t.Fatalf("identity doc content missing expected heading: %s", identityDoc.Content)
	}

	pack, err := h.ContextPack(targetPath, "SQL", 5)
	if err != nil {
		t.Fatalf("ContextPack() error = %v", err)
	}
	if pack.SystemDoc == nil || pack.ProgressDoc == nil {
		t.Fatal("ContextPack() missing system or progress document")
	}
	if pack.CurrentWeek == nil {
		t.Fatal("ContextPack() missing current week document")
	}
	if pack.TargetDoc == nil {
		t.Fatal("ContextPack() missing target document")
	}
	if len(pack.TargetDoc.Attachments) == 0 {
		t.Fatal("ContextPack() missing target document attachments")
	}
	if len(pack.RelatedHits) == 0 {
		t.Fatal("ContextPack() missing related hits")
	}
	if len(pack.Backlinks) == 0 {
		t.Fatal("ContextPack() missing backlinks")
	}
	if len(pack.Attachments) == 0 {
		t.Fatal("ContextPack() missing aggregated attachments")
	}
}

func TestVaultReadToolsRecordAudit(t *testing.T) {
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

	if _, err := h.SystemDocGet("system"); err != nil {
		t.Fatalf("SystemDocGet() error = %v", err)
	}
	if _, err := h.VaultList(""); err != nil {
		t.Fatalf("VaultList() error = %v", err)
	}

	records, err := st.Audit().ListAudit(10)
	if err != nil {
		t.Fatalf("ListAudit() error = %v", err)
	}
	if len(records) < 2 {
		t.Fatalf("audit count = %d, want at least 2", len(records))
	}

	found := false
	for _, record := range records {
		if record.Kind == "mcp_read" && strings.TrimSpace(record.Metadata["tool"]) != "" {
			found = true
			break
		}
	}
	if !found {
		t.Fatal("expected mcp_read audit record")
	}
}

func TestSystemDocGetAcceptsManagedCoreAliases(t *testing.T) {
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

	tests := map[string]string{
		"\u753b\u50cf":             cfg.Vault.ManagedCore.Persona,
		"\u4eba\u7269\u753b\u50cf": cfg.Vault.ManagedCore.Persona,
		"\u8fdb\u5ea6\u603b\u8868": cfg.Vault.ManagedCore.ProgressIndex,
		"\u7cfb\u7edf\u8bf4\u660e": cfg.Vault.ManagedCore.SystemDoc,
		"\u8eab\u4efd":             cfg.Vault.ManagedCore.IdentityDoc,
	}
	for name, wantPath := range tests {
		doc, err := h.SystemDocGet(name)
		if err != nil {
			t.Fatalf("SystemDocGet(%q) error = %v", name, err)
		}
		if doc.Path != filepath.ToSlash(wantPath) {
			t.Fatalf("SystemDocGet(%q).Path = %q, want %q", name, doc.Path, filepath.ToSlash(wantPath))
		}
	}
}
func TestVaultResolveUniqueAmbiguousAndNotFound(t *testing.T) {
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
	files := map[string]string{
		"notes/python.md":          "# Python",
		"notes/python-diary.md":    "# Python Diary",
		"notes/golang-handbook.md": "# Go",
	}
	for relPath, content := range files {
		if _, err := vault.WriteFileAtomic(filepath.Join(cfg.Paths.VaultRoot, filepath.FromSlash(relPath)), []byte(content), cfg.Vault.TempSuffix); err != nil {
			t.Fatalf("WriteFileAtomic(%s) error = %v", relPath, err)
		}
	}

	unique, err := h.VaultResolve("golang handbook", "", 5)
	if err != nil {
		t.Fatalf("VaultResolve(unique) error = %v", err)
	}
	if unique.Status != "unique" || unique.SelectedPath != "notes/golang-handbook.md" {
		t.Fatalf("unique result = %+v, want golang handbook unique", unique)
	}

	ambiguous, err := h.VaultResolve("python", "", 5)
	if err != nil {
		t.Fatalf("VaultResolve(ambiguous) error = %v", err)
	}
	if ambiguous.Status != "ambiguous" || len(ambiguous.Matches) < 2 {
		t.Fatalf("ambiguous result = %+v, want multiple matches", ambiguous)
	}

	missing, err := h.VaultResolve("does-not-exist", "", 5)
	if err != nil {
		t.Fatalf("VaultResolve(missing) error = %v", err)
	}
	if missing.Status != "not_found" {
		t.Fatalf("missing status = %q, want not_found", missing.Status)
	}
}
func TestVaultReadAuditPrefersLoreAgentIdentity(t *testing.T) {
	workDir := t.TempDir()
	cfg := config.Default(workDir)
	st := memory.New()
	t.Setenv("OBSIDIAN_HARNESS_MCP_AGENT_ID", "legacy-agent")
	t.Setenv("LORE_MCP_AGENT_ID", "lore-agent")

	h, err := New(cfg, st)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	if _, err := h.BootstrapManagedVault(time.Date(2026, 4, 22, 9, 0, 0, 0, time.UTC)); err != nil {
		t.Fatalf("BootstrapManagedVault() error = %v", err)
	}

	if _, err := h.ManagedStatus(); err != nil {
		t.Fatalf("ManagedStatus() error = %v", err)
	}

	records, err := st.Audit().ListAudit(10)
	if err != nil {
		t.Fatalf("ListAudit() error = %v", err)
	}
	if len(records) == 0 {
		t.Fatal("expected at least one audit record")
	}
	if records[0].Actor != "mcp:lore-agent" {
		t.Fatalf("audit actor = %q, want mcp:lore-agent", records[0].Actor)
	}
	if records[0].ActorType != "mcp" {
		t.Fatalf("audit actor_type = %q, want mcp", records[0].ActorType)
	}
	if records[0].ActorID != "lore-agent" {
		t.Fatalf("audit actor_id = %q, want lore-agent", records[0].ActorID)
	}
	if records[0].ResultStatus != "ok" {
		t.Fatalf("audit result_status = %q, want ok", records[0].ResultStatus)
	}
	if records[0].CorrelationID == "" {
		t.Fatal("audit correlation_id is empty")
	}
}

func TestVaultReadRejectsTraversal(t *testing.T) {
	workDir := t.TempDir()
	cfg := config.Default(workDir)
	st := memory.New()

	h, err := New(cfg, st)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	if _, err := h.VaultRead("..\\outside.md"); err == nil {
		t.Fatal("VaultRead() error = nil, want traversal rejection")
	}
}

func TestVaultReadExtractsAttachments(t *testing.T) {
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

	path := filepath.Join("03-notes", "with-attachments.md")
	content := "# Note\n\n![[assets/chart.png]]\n[report](docs/spec.pdf)"
	if _, err := vault.WriteFileAtomic(filepath.Join(cfg.Paths.VaultRoot, path), []byte(content), cfg.Vault.TempSuffix); err != nil {
		t.Fatalf("WriteFileAtomic(path) error = %v", err)
	}

	doc, err := h.VaultRead(path)
	if err != nil {
		t.Fatalf("VaultRead() error = %v", err)
	}
	if len(doc.Attachments) != 2 {
		t.Fatalf("len(doc.Attachments) = %d, want 2", len(doc.Attachments))
	}
}

func TestVaultReadRejectsNonMarkdown(t *testing.T) {
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

	assetPath := filepath.Join("03-notes", "raw.txt")
	if _, err := vault.WriteFileAtomic(filepath.Join(cfg.Paths.VaultRoot, assetPath), []byte("not markdown"), cfg.Vault.TempSuffix); err != nil {
		t.Fatalf("WriteFileAtomic(asset) error = %v", err)
	}

	if _, err := h.VaultRead(assetPath); err == nil {
		t.Fatal("VaultRead() error = nil, want non-markdown rejection")
	}
}
