package cli

import (
	"errors"
	"strings"
	"testing"
	"time"

	"obsidian-harness/internal/config"
)

func TestRenderLLMConfigDiagnosticsOmitsSecretsAndShowsSource(t *testing.T) {
	got := renderLLMConfigDiagnostics([]config.LLMDiagnostic{
		{
			Purpose:   config.LLMPurposeOperator,
			Enabled:   true,
			Source:    config.LLMSourceWorkspace,
			Profile:   "deepseek",
			Provider:  "deepseek",
			BaseURL:   "https://api.deepseek.com/v1",
			Model:     "deepseek-chat",
			Timeout:   45 * time.Second,
			APIKeyEnv: "DEEPSEEK_API_KEY",
		},
	})
	for _, want := range []string{"Model Config", "operator:", "provider=deepseek", "model=deepseek-chat", "source=workspace", "profile=deepseek", "api_key_env=DEEPSEEK_API_KEY"} {
		if !strings.Contains(got, want) {
			t.Fatalf("output missing %q:\n%s", want, got)
		}
	}
	if strings.Contains(got, "secret") {
		t.Fatalf("output leaked secret-looking value:\n%s", got)
	}
}

func TestRenderLLMConfigDiagnosticsShowsErrorsAndDisabled(t *testing.T) {
	got := renderLLMConfigDiagnostics([]config.LLMDiagnostic{
		{Purpose: config.LLMPurposeOperator, Source: config.LLMSourceDefault},
		{Purpose: config.LLMPurposePersonaExtract, Source: config.LLMSourceWorkspace, Profile: "broken", Err: errors.New("api_key_env missing")},
	})
	for _, want := range []string{"operator: disabled source=default", "persona_extract: error source=workspace profile=broken err=api_key_env missing"} {
		if !strings.Contains(got, want) {
			t.Fatalf("output missing %q:\n%s", want, got)
		}
	}
}
