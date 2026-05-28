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
			KeyStatus: "present",
		},
	})
	for _, want := range []string{"Model Config", "operator:", "provider=deepseek", "model=deepseek-chat", "source=workspace", "profile=deepseek", "api_key_env=DEEPSEEK_API_KEY", "key=present"} {
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
		{Purpose: config.LLMPurposePersonaExtract, Source: config.LLMSourceWorkspace, Profile: "broken", APIKeyEnv: "DEEPSEEK_API_KEY", KeyStatus: "missing", Err: errors.New("DEEPSEEK_API_KEY is not set")},
	})
	for _, want := range []string{"operator: disabled source=default", "persona_extract: error source=workspace profile=broken key=missing err=DEEPSEEK_API_KEY is not set"} {
		if !strings.Contains(got, want) {
			t.Fatalf("output missing %q:\n%s", want, got)
		}
	}
}

func TestRenderLLMConfigDiagnosticsRedactsCredentialBearingErrors(t *testing.T) {
	got := renderLLMConfigDiagnostics([]config.LLMDiagnostic{
		{
			Purpose:   config.LLMPurposeOperator,
			Source:    config.LLMSourceWorkspace,
			Profile:   "deepseek",
			APIKeyEnv: "DEEPSEEK_API_KEY",
			KeyStatus: "present",
			Err:       errors.New("upstream rejected Authorization: Bearer test-secret-value"),
		},
	})
	if strings.Contains(got, "test-secret-value") || strings.Contains(got, "Bearer") || strings.Contains(got, "Authorization") {
		t.Fatalf("diagnostic leaked credential-bearing error:\n%s", got)
	}
	if !strings.Contains(got, "[redacted credential-bearing error]") {
		t.Fatalf("diagnostic missing redaction marker:\n%s", got)
	}
}
