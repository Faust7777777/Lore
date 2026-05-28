package config

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestUpsertLLMProfilePreservesWorkspaceConfigAndOmitsSecretValue(t *testing.T) {
	clearLLMEnv(t)
	workDir := t.TempDir()
	workspacePath := filepath.Join(workDir, ".lore", "config.json")
	if err := os.MkdirAll(filepath.Dir(workspacePath), 0o755); err != nil {
		t.Fatalf("mkdir workspace config: %v", err)
	}
	initial := `{
  "runtime": {"proactive_mode": "quiet"},
  "process_sink": {"retention_days": 21}
}`
	if err := os.WriteFile(workspacePath, []byte(initial), 0o600); err != nil {
		t.Fatalf("write workspace config: %v", err)
	}
	t.Setenv("DEEPSEEK_API_KEY", "secret-value-must-not-be-written")

	profile := LLMProfileConfig{
		Provider:  "deepseek",
		BaseURL:   "https://api.deepseek.com/v1",
		Model:     "deepseek-chat",
		Timeout:   45 * time.Second,
		APIKeyEnv: "DEEPSEEK_API_KEY",
	}
	if err := UpsertLLMProfile(workDir, "deepseek", profile); err != nil {
		t.Fatalf("UpsertLLMProfile() error = %v", err)
	}

	firstWrite, err := os.ReadFile(workspacePath)
	if err != nil {
		t.Fatalf("read workspace config: %v", err)
	}
	if !strings.Contains(string(firstWrite), `"runtime"`) || !strings.Contains(string(firstWrite), `"process_sink"`) {
		t.Fatalf("workspace config lost existing sections: %s", firstWrite)
	}
	if !strings.Contains(string(firstWrite), "DEEPSEEK_API_KEY") {
		t.Fatalf("workspace config missing api_key_env name: %s", firstWrite)
	}
	if strings.Contains(string(firstWrite), "secret-value-must-not-be-written") {
		t.Fatalf("workspace config leaked API key value: %s", firstWrite)
	}

	if err := UpsertLLMProfile(workDir, "deepseek", profile); err != nil {
		t.Fatalf("second UpsertLLMProfile() error = %v", err)
	}
	secondWrite, err := os.ReadFile(workspacePath)
	if err != nil {
		t.Fatalf("read second workspace config: %v", err)
	}
	if string(secondWrite) != string(firstWrite) {
		t.Fatalf("second upsert is not idempotent\nfirst:\n%s\nsecond:\n%s", firstWrite, secondWrite)
	}

	cfg, _, err := LoadWithOptions(workDir, LoadOptions{UserGlobalPath: absentPath(t)})
	if err != nil {
		t.Fatalf("LoadWithOptions() error = %v", err)
	}
	if cfg.Runtime.ProactiveMode != "quiet" {
		t.Fatalf("Runtime.ProactiveMode = %q, want preserved quiet", cfg.Runtime.ProactiveMode)
	}
	if cfg.ProcessSink.RetentionDays != 21 {
		t.Fatalf("ProcessSink.RetentionDays = %d, want preserved 21", cfg.ProcessSink.RetentionDays)
	}
}

func TestSetActiveLLMProfileSwitchesResolver(t *testing.T) {
	clearLLMEnv(t)
	workDir := t.TempDir()
	t.Setenv("DEEPSEEK_API_KEY", "deepseek-secret")
	t.Setenv("KIMI_API_KEY", "kimi-secret")
	if err := UpsertLLMProfile(workDir, "deepseek", LLMProfileConfig{
		Provider:  "deepseek",
		BaseURL:   "https://api.deepseek.com/v1",
		Model:     "deepseek-v4-pro",
		APIKeyEnv: "DEEPSEEK_API_KEY",
	}); err != nil {
		t.Fatalf("upsert deepseek: %v", err)
	}
	if err := UpsertLLMProfile(workDir, "kimi", LLMProfileConfig{
		Provider:  "kimi",
		BaseURL:   "https://api.moonshot.cn/v1",
		Model:     "kimi-k2",
		APIKeyEnv: "KIMI_API_KEY",
	}); err != nil {
		t.Fatalf("upsert kimi: %v", err)
	}
	if err := SetActiveLLMProfile(workDir, "kimi"); err != nil {
		t.Fatalf("SetActiveLLMProfile(kimi) error = %v", err)
	}

	resolved, _, err := ResolveLLMConfigWithOptions(workDir, LLMPurposeOperator, LoadOptions{UserGlobalPath: absentPath(t)})
	if err != nil {
		t.Fatalf("ResolveLLMConfigWithOptions(kimi) error = %v", err)
	}
	if resolved.Profile != "kimi" || resolved.Model != "kimi-k2" || resolved.APIKey != "kimi-secret" {
		t.Fatalf("resolved after kimi active = %+v, want kimi profile", resolved)
	}

	if err := SetActiveLLMProfile(workDir, "deepseek"); err != nil {
		t.Fatalf("SetActiveLLMProfile(deepseek) error = %v", err)
	}
	resolved, _, err = ResolveLLMConfigWithOptions(workDir, LLMPurposeOperator, LoadOptions{UserGlobalPath: absentPath(t)})
	if err != nil {
		t.Fatalf("ResolveLLMConfigWithOptions(deepseek) error = %v", err)
	}
	if resolved.Profile != "deepseek" || resolved.Model != "deepseek-v4-pro" || resolved.APIKey != "deepseek-secret" {
		t.Fatalf("resolved after deepseek active = %+v, want deepseek profile", resolved)
	}
}

func TestSetActiveLLMProfileRejectsUnknownProfile(t *testing.T) {
	workDir := t.TempDir()
	if err := UpsertLLMProfile(workDir, "deepseek", LLMProfileConfig{
		BaseURL:   "https://api.deepseek.com/v1",
		Model:     "deepseek-v4-pro",
		APIKeyEnv: "DEEPSEEK_API_KEY",
	}); err != nil {
		t.Fatalf("UpsertLLMProfile() error = %v", err)
	}
	err := SetActiveLLMProfile(workDir, "missing")
	if err == nil {
		t.Fatal("SetActiveLLMProfile(missing) error = nil, want error")
	}
	if !strings.Contains(err.Error(), `profile "missing" does not exist`) {
		t.Fatalf("error = %q, want unknown profile", err.Error())
	}
}

func TestSaveWorkspaceConfigPreservesUnknownTopLevelFields(t *testing.T) {
	workDir := t.TempDir()
	workspacePath := filepath.Join(workDir, ".lore", "config.json")
	if err := os.MkdirAll(filepath.Dir(workspacePath), 0o755); err != nil {
		t.Fatalf("mkdir workspace config: %v", err)
	}
	if err := os.WriteFile(workspacePath, []byte(`{"custom_section":{"enabled":true},"runtime":{"proactive_mode":"off"}}`), 0o600); err != nil {
		t.Fatalf("write workspace config: %v", err)
	}
	llm := LLMConfig{
		ActiveProfile: "deepseek",
		Profiles: map[string]LLMProfileConfig{
			"deepseek": {
				Provider:  "deepseek",
				BaseURL:   "https://api.deepseek.com/v1",
				Model:     "deepseek-v4-pro",
				APIKeyEnv: "DEEPSEEK_API_KEY",
			},
		},
	}
	if err := SaveWorkspaceConfig(workDir, WorkspaceConfigPatch{LLM: &llm}); err != nil {
		t.Fatalf("SaveWorkspaceConfig() error = %v", err)
	}
	data, err := os.ReadFile(workspacePath)
	if err != nil {
		t.Fatalf("read workspace config: %v", err)
	}
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		t.Fatalf("workspace config invalid JSON: %v\n%s", err, data)
	}
	if _, ok := raw["custom_section"]; !ok {
		t.Fatalf("custom_section was not preserved: %s", data)
	}
	if _, ok := raw["runtime"]; !ok {
		t.Fatalf("runtime was not preserved: %s", data)
	}
	if _, ok := raw["llm"]; !ok {
		t.Fatalf("llm was not written: %s", data)
	}
}

func TestLoadEditableConfigRejectsNonObjectWorkspaceConfig(t *testing.T) {
	workDir := t.TempDir()
	workspacePath := filepath.Join(workDir, ".lore", "config.json")
	if err := os.MkdirAll(filepath.Dir(workspacePath), 0o755); err != nil {
		t.Fatalf("mkdir workspace config: %v", err)
	}
	if err := os.WriteFile(workspacePath, []byte(`[]`), 0o600); err != nil {
		t.Fatalf("write workspace config: %v", err)
	}
	_, err := LoadEditableConfig(workDir)
	if err == nil {
		t.Fatal("LoadEditableConfig() error = nil, want non-object error")
	}
	if !strings.Contains(err.Error(), "must be a JSON object") {
		t.Fatalf("error = %q, want JSON object", err.Error())
	}
}
