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
	for _, tc := range []struct {
		name   string
		err    string
		banned []string
	}{
		{
			name:   "authorization bearer",
			err:    "upstream rejected Authorization: Bearer test-secret-value",
			banned: []string{"test-secret-value", "Bearer", "Authorization"},
		},
		{
			name:   "api key echo",
			err:    "Your api key: sk-live-abc123 is invalid",
			banned: []string{"sk-live-abc123", "api key"},
		},
		{
			name:   "api_key parameter echo",
			err:    "provider rejected api_key=plain-text-key",
			banned: []string{"plain-text-key", "api_key"},
		},
		{
			name:   "token echo",
			err:    "provider said token tok_abc123 expired",
			banned: []string{"tok_abc123", "token"},
		},
		{
			name:   "secret echo",
			err:    "request failed with client secret client-secret-abc",
			banned: []string{"client-secret-abc", "secret"},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := renderLLMConfigDiagnostics([]config.LLMDiagnostic{
				{
					Purpose:   config.LLMPurposeOperator,
					Source:    config.LLMSourceWorkspace,
					Profile:   "deepseek",
					APIKeyEnv: "DEEPSEEK_API_KEY",
					KeyStatus: "present",
					Err:       errors.New(tc.err),
				},
			})
			for _, banned := range tc.banned {
				if strings.Contains(got, banned) {
					t.Fatalf("diagnostic leaked %q:\n%s", banned, got)
				}
			}
			if !strings.Contains(got, "[redacted credential-bearing error]") {
				t.Fatalf("diagnostic missing redaction marker:\n%s", got)
			}
		})
	}
}

func TestRenderLLMConfigDiagnosticsKeepsMissingEnvVarName(t *testing.T) {
	got := renderLLMConfigDiagnostics([]config.LLMDiagnostic{
		{
			Purpose:   config.LLMPurposeOperator,
			Source:    config.LLMSourceWorkspace,
			Profile:   "deepseek",
			APIKeyEnv: "DEEPSEEK_API_KEY",
			KeyStatus: "missing",
			Err:       errors.New("llm: profile \"deepseek\" api_key_env DEEPSEEK_API_KEY is not set"),
		},
	})
	if !strings.Contains(got, "DEEPSEEK_API_KEY is not set") {
		t.Fatalf("diagnostic should keep actionable missing env var name:\n%s", got)
	}
	if strings.Contains(got, "[redacted credential-bearing error]") {
		t.Fatalf("missing env var diagnostic should not be redacted:\n%s", got)
	}
}
