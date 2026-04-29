package orchestrator

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"obsidian-harness/internal/config"
	"obsidian-harness/internal/store/memory"
)

func TestBuildCoreContextIncludesPersonaWeaknessSystemProgressAndDrafts(t *testing.T) {
	workDir := t.TempDir()
	cfg := config.Default(workDir)
	st := memory.New()

	h, err := New(cfg, st)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	if _, err := h.BootstrapManagedVault(time.Date(2026, 4, 28, 9, 0, 0, 0, time.UTC)); err != nil {
		t.Fatalf("BootstrapManagedVault() error = %v", err)
	}
	if _, _, err := writeVaultFile(t, cfg, cfg.Vault.ManagedCore.Persona, "# Persona\n\n## Identity / Stable Profile\n\n- School: Dalian University of Technology\n- Major: E-commerce\n\n## Current State\n\n- Focus: coursework and Lore governance\n\n## Weaknesses\n\n- Needs structured review before durable notes\n"); err != nil {
		t.Fatalf("write persona: %v", err)
	}
	if _, _, err := writeVaultFile(t, cfg, cfg.Vault.ManagedCore.SystemDoc, "# System\n\n- Managed docs require draft review apply.\n"); err != nil {
		t.Fatalf("write system: %v", err)
	}
	if _, _, err := writeVaultFile(t, cfg, cfg.Vault.ManagedCore.ProgressIndex, "# Progress\n\n- Current milestone: governed note intake.\n"); err != nil {
		t.Fatalf("write progress: %v", err)
	}
	if _, _, err := writeVaultFile(t, cfg, cfg.Vault.ManagedCore.IdentityDoc, "# Identity\n\nLORE SELF IDENTITY SECRET\n"); err != nil {
		t.Fatalf("write identity: %v", err)
	}

	at := time.Date(2026, 4, 28, 10, 30, 0, 0, time.UTC)
	proposal := validMarkdownNoteProposal("03-notes/inbox/ecommerce.md", at)
	result, err := h.ProposeMarkdownNote(proposal, at)
	if err != nil {
		t.Fatalf("ProposeMarkdownNote() error = %v", err)
	}

	ctx, err := h.BuildCoreContext(4)
	if err != nil {
		t.Fatalf("BuildCoreContext() error = %v", err)
	}
	if !strings.Contains(ctx.PersonaSummary, "Dalian University of Technology") || !strings.Contains(ctx.PersonaSummary, "coursework and Lore governance") {
		t.Fatalf("PersonaSummary = %q, want stable profile and current state", ctx.PersonaSummary)
	}
	if !strings.Contains(ctx.WeaknessSummary, "structured review") {
		t.Fatalf("WeaknessSummary = %q, want weakness excerpt", ctx.WeaknessSummary)
	}
	if !strings.Contains(ctx.SystemRulesSummary, "draft review apply") {
		t.Fatalf("SystemRulesSummary = %q", ctx.SystemRulesSummary)
	}
	if !strings.Contains(ctx.ProgressSummary, "governed note intake") {
		t.Fatalf("ProgressSummary = %q", ctx.ProgressSummary)
	}
	if len(ctx.PendingDrafts) != 1 || !strings.Contains(ctx.PendingDrafts[0], result.DraftID) || !strings.Contains(ctx.PendingDrafts[0], "markdown_note_write") {
		t.Fatalf("PendingDrafts = %+v, want markdown note draft", ctx.PendingDrafts)
	}
	combined := ctx.PersonaSummary + ctx.WeaknessSummary + ctx.SystemRulesSummary + ctx.ProgressSummary + strings.Join(ctx.PendingDrafts, "\n") + strings.Join(ctx.Notes, "\n")
	if strings.Contains(combined, "LORE SELF IDENTITY SECRET") {
		t.Fatalf("BuildCoreContext read identity content unexpectedly: %q", combined)
	}
}

func TestBuildCoreContextNotesMissingPersonaSections(t *testing.T) {
	workDir := t.TempDir()
	cfg := config.Default(workDir)
	st := memory.New()

	h, err := New(cfg, st)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	if _, err := h.BootstrapManagedVault(time.Date(2026, 4, 28, 9, 0, 0, 0, time.UTC)); err != nil {
		t.Fatalf("BootstrapManagedVault() error = %v", err)
	}
	if _, _, err := writeVaultFile(t, cfg, cfg.Vault.ManagedCore.Persona, "# Persona\n\nFreeform only.\n"); err != nil {
		t.Fatalf("write persona: %v", err)
	}

	ctx, err := h.BuildCoreContext(4)
	if err != nil {
		t.Fatalf("BuildCoreContext() error = %v", err)
	}
	if !strings.Contains(ctx.PersonaSummary, "Freeform only") {
		t.Fatalf("PersonaSummary = %q, want fallback excerpt", ctx.PersonaSummary)
	}
	notes := strings.Join(ctx.Notes, "\n")
	for _, want := range []string{"Identity / Stable Profile", "Current State", "Weaknesses"} {
		if !strings.Contains(notes, want) {
			t.Fatalf("Notes = %q, missing %q", notes, want)
		}
	}
}

func TestMarkdownSectionHandlesNestedHeadings(t *testing.T) {
	content := strings.Join([]string{
		"# Persona",
		"",
		"## Weaknesses",
		"",
		"- SQL triggers",
		"",
		"### Evidence",
		"",
		"- missed exercise",
		"",
		"## Freeform Notes",
		"",
		"- stop here",
	}, "\n")
	got := markdownSection(content, "Weaknesses")
	if !strings.Contains(got, "SQL triggers") || !strings.Contains(got, "missed exercise") || strings.Contains(got, "stop here") {
		t.Fatalf("markdownSection() = %q", got)
	}
}

func TestBuildCoreContextDoesNotRequireIdentityDoc(t *testing.T) {
	workDir := t.TempDir()
	cfg := config.Default(workDir)
	st := memory.New()

	h, err := New(cfg, st)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	if _, err := h.BootstrapManagedVault(time.Date(2026, 4, 28, 9, 0, 0, 0, time.UTC)); err != nil {
		t.Fatalf("BootstrapManagedVault() error = %v", err)
	}
	if err := removeManagedDoc(t, cfg, cfg.Vault.ManagedCore.IdentityDoc); err != nil {
		t.Fatalf("remove identity: %v", err)
	}

	if _, err := h.BuildCoreContext(4); err != nil {
		t.Fatalf("BuildCoreContext() error = %v; identity must not be required", err)
	}
}

func removeManagedDoc(t *testing.T, cfg config.Config, relPath string) error {
	t.Helper()
	path := filepath.Join(cfg.Paths.VaultRoot, filepath.FromSlash(relPath))
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}
