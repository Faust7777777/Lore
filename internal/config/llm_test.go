package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func clearLLMEnv(t *testing.T) {
	t.Helper()
	for _, name := range []string{
		"LORE_OPERATOR_BASE_URL", "LORE_OPERATOR_API_KEY", "LORE_OPERATOR_MODEL", "LORE_OPERATOR_PROVIDER", "LORE_OPERATOR_TIMEOUT",
		"LORE_PERSONA_EXTRACT_BASE_URL", "LORE_PERSONA_EXTRACT_API_KEY", "LORE_PERSONA_EXTRACT_MODEL", "LORE_PERSONA_EXTRACT_PROVIDER", "LORE_PERSONA_EXTRACT_TIMEOUT",
		"LORE_PROCESS_SINK_BASE_URL", "LORE_PROCESS_SINK_API_KEY", "LORE_PROCESS_SINK_MODEL", "LORE_PROCESS_SINK_PROVIDER", "LORE_PROCESS_SINK_TIMEOUT",
		"LORE_LLM_BASE_URL", "LORE_LLM_API_KEY", "LORE_LLM_MODEL", "LORE_LLM_PROVIDER", "LORE_LLM_TIMEOUT",
		"OBSIDIAN_HARNESS_OPERATOR_BASE_URL", "OBSIDIAN_HARNESS_OPERATOR_API_KEY", "OBSIDIAN_HARNESS_OPERATOR_MODEL", "OBSIDIAN_HARNESS_OPERATOR_PROVIDER", "OBSIDIAN_HARNESS_OPERATOR_TIMEOUT",
		"OBSIDIAN_HARNESS_PERSONA_EXTRACT_BASE_URL", "OBSIDIAN_HARNESS_PERSONA_EXTRACT_API_KEY", "OBSIDIAN_HARNESS_PERSONA_EXTRACT_MODEL", "OBSIDIAN_HARNESS_PERSONA_EXTRACT_PROVIDER", "OBSIDIAN_HARNESS_PERSONA_EXTRACT_TIMEOUT",
		"OBSIDIAN_HARNESS_PROCESS_SINK_BASE_URL", "OBSIDIAN_HARNESS_PROCESS_SINK_API_KEY", "OBSIDIAN_HARNESS_PROCESS_SINK_MODEL", "OBSIDIAN_HARNESS_PROCESS_SINK_PROVIDER", "OBSIDIAN_HARNESS_PROCESS_SINK_TIMEOUT",
		"OBSIDIAN_HARNESS_LLM_BASE_URL", "OBSIDIAN_HARNESS_LLM_API_KEY", "OBSIDIAN_HARNESS_LLM_MODEL", "OBSIDIAN_HARNESS_LLM_PROVIDER", "OBSIDIAN_HARNESS_LLM_TIMEOUT",
	} {
		t.Setenv(name, "")
	}
}

func TestResolveLLMConfigUsesWorkspaceProfileOverEnv(t *testing.T) {
	clearLLMEnv(t)
	workDir := t.TempDir()
	workspacePath := filepath.Join(workDir, ".lore", "config.json")
	if err := os.MkdirAll(filepath.Dir(workspacePath), 0o755); err != nil {
		t.Fatalf("mkdir workspace config: %v", err)
	}
	if err := os.WriteFile(workspacePath, []byte(`{
		"llm": {
			"active_profile": "deepseek",
			"profiles": {
				"deepseek": {
					"provider": "deepseek",
					"base_url": "https://api.deepseek.com/v1",
					"model": "deepseek-chat",
					"timeout": 45000000000,
					"api_key_env": "DEEPSEEK_API_KEY"
				}
			}
		}
	}`), 0o600); err != nil {
		t.Fatalf("write workspace config: %v", err)
	}
	t.Setenv("LORE_LLM_BASE_URL", "https://api.ikuncode.cc/v1")
	t.Setenv("LORE_LLM_MODEL", "gpt-5.4")
	t.Setenv("LORE_LLM_API_KEY", "ikuncode-secret")
	t.Setenv("DEEPSEEK_API_KEY", "deepseek-secret")

	resolved, diagnostics, err := ResolveLLMConfigWithOptions(workDir, LLMPurposeOperator, LoadOptions{
		UserGlobalPath: absentPath(t),
	})
	if err != nil {
		t.Fatalf("ResolveLLMConfigWithOptions returned error: %v", err)
	}
	if !resolved.Enabled {
		t.Fatal("resolved.Enabled = false, want true")
	}
	if resolved.Source != LLMSourceWorkspace {
		t.Fatalf("resolved.Source = %q, want workspace; diagnostics=%+v", resolved.Source, diagnostics)
	}
	if resolved.Provider != "deepseek" || resolved.BaseURL != "https://api.deepseek.com/v1" || resolved.Model != "deepseek-chat" {
		t.Fatalf("resolved provider/base/model = %q/%q/%q", resolved.Provider, resolved.BaseURL, resolved.Model)
	}
	if resolved.APIKey != "deepseek-secret" || resolved.APIKeyEnv != "DEEPSEEK_API_KEY" {
		t.Fatalf("resolved key/env = %q/%q, want deepseek env", resolved.APIKey, resolved.APIKeyEnv)
	}
	if resolved.Timeout != 45*time.Second {
		t.Fatalf("resolved.Timeout = %s, want 45s", resolved.Timeout)
	}
}

func TestResolveLLMConfigFallsBackToEnvWhenNoProfile(t *testing.T) {
	clearLLMEnv(t)
	workDir := t.TempDir()
	t.Setenv("LORE_LLM_BASE_URL", "https://api.ikuncode.cc/v1")
	t.Setenv("LORE_LLM_MODEL", "gpt-5.4")
	t.Setenv("LORE_LLM_API_KEY", "ikuncode-secret")

	resolved, _, err := ResolveLLMConfigWithOptions(workDir, LLMPurposeProcessSink, LoadOptions{
		UserGlobalPath: absentPath(t),
	})
	if err != nil {
		t.Fatalf("ResolveLLMConfigWithOptions returned error: %v", err)
	}
	if resolved.Source != LLMSourceEnv || resolved.BaseURL != "https://api.ikuncode.cc/v1" || resolved.Model != "gpt-5.4" {
		t.Fatalf("resolved = %+v, want env ikuncode/gpt-5.4", resolved)
	}
	if resolved.APIKey != "ikuncode-secret" || resolved.APIKeyEnv != "LORE_LLM_API_KEY" {
		t.Fatalf("key/env = %q/%q, want generic env key", resolved.APIKey, resolved.APIKeyEnv)
	}
}

func TestResolveLLMConfigRequiresProfileAPIKeyEnv(t *testing.T) {
	clearLLMEnv(t)
	workDir := t.TempDir()
	workspacePath := filepath.Join(workDir, ".lore", "config.json")
	if err := os.MkdirAll(filepath.Dir(workspacePath), 0o755); err != nil {
		t.Fatalf("mkdir workspace config: %v", err)
	}
	if err := os.WriteFile(workspacePath, []byte(`{
		"llm": {
			"active_profile": "deepseek",
			"profiles": {
				"deepseek": {"base_url":"https://api.deepseek.com/v1","model":"deepseek-chat"}
			}
		}
	}`), 0o600); err != nil {
		t.Fatalf("write workspace config: %v", err)
	}

	_, _, err := ResolveLLMConfigWithOptions(workDir, LLMPurposeOperator, LoadOptions{UserGlobalPath: absentPath(t)})
	if err == nil {
		t.Fatal("ResolveLLMConfigWithOptions error = nil, want missing api_key_env")
	}
	if !strings.Contains(err.Error(), "api_key_env") {
		t.Fatalf("error = %q, want api_key_env", err.Error())
	}
}

func TestResolveLLMConfigReportsDisabledDefault(t *testing.T) {
	clearLLMEnv(t)
	resolved, _, err := ResolveLLMConfigWithOptions(t.TempDir(), LLMPurposeOperator, LoadOptions{UserGlobalPath: absentPath(t)})
	if err != nil {
		t.Fatalf("ResolveLLMConfigWithOptions returned error: %v", err)
	}
	if resolved.Enabled || resolved.Source != LLMSourceDefault {
		t.Fatalf("resolved = %+v, want disabled default", resolved)
	}
}
