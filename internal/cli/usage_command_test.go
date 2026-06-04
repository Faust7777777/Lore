package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"obsidian-harness/internal/app"
	"obsidian-harness/internal/model"
)

func TestRunUsageCommandSurfacesSoftBudgetWarning(t *testing.T) {
	// End-to-end proof that usage.soft_warning_tokens now takes effect: a
	// workspace config sets a low soft budget, today's usage exceeds it,
	// and `lore usage` must print the soft-budget line. Previously the
	// config was parsed/validated but never surfaced anywhere.
	workDir := t.TempDir()
	cfgDir := filepath.Join(workDir, ".lore")
	if err := os.MkdirAll(cfgDir, 0o755); err != nil {
		t.Fatalf("mkdir .lore: %v", err)
	}
	// Only soft_warning_tokens is overridden; track_usage stays the
	// default true, so the seeded usage below is actually recorded.
	if err := os.WriteFile(filepath.Join(cfgDir, "config.json"), []byte(`{"usage":{"soft_warning_tokens":100}}`), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	// Seed today's usage at 120 tokens, over the 100 budget.
	seed, err := app.OpenRuntime(workDir)
	if err != nil {
		t.Fatalf("OpenRuntime(seed): %v", err)
	}
	if err := seed.RecordUsage([]model.UsageRecord{{
		Provider: "openai-compatible", Model: "gpt-x", AgentID: "codex", SessionID: "s1",
		PromptTokens: 90, CompletionTokens: 30, RecordedAt: time.Now(),
	}}); err != nil {
		t.Fatalf("RecordUsage: %v", err)
	}
	if err := seed.Close(); err != nil {
		t.Fatalf("seed Close: %v", err)
	}

	var stdout, stderr bytes.Buffer
	if code := runUsageCommand([]string{workDir}, &stdout, &stderr); code != 0 {
		t.Fatalf("runUsageCommand exit = %d, stderr=%q", code, stderr.String())
	}
	out := stdout.String()
	if !strings.Contains(out, "Soft budget") || !strings.Contains(out, "100") {
		t.Fatalf("expected a soft-budget warning naming the 100 threshold:\n%s", out)
	}
}
