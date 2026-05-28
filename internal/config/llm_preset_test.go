package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLLMProfileFromPresetDeepSeekUsesOfficialFullModelNames(t *testing.T) {
	deepseek, err := LLMProfileFromPreset(LLMProfilePresetDeepSeek, LLMProfileConfig{})
	if err != nil {
		t.Fatalf("LLMProfileFromPreset(deepseek) error = %v", err)
	}
	if deepseek.Model != "deepseek-v4-pro" {
		t.Fatalf("deepseek model = %q, want full official name deepseek-v4-pro", deepseek.Model)
	}
	if deepseek.Model == "v4-pro" {
		t.Fatal("deepseek preset used rejected short model name v4-pro")
	}
	if deepseek.BaseURL != "https://api.deepseek.com/v1" || deepseek.APIKeyEnv != "DEEPSEEK_API_KEY" {
		t.Fatalf("deepseek preset = %+v", deepseek)
	}

	fast, err := LLMProfileFromPreset(LLMProfilePresetDeepSeekFast, LLMProfileConfig{})
	if err != nil {
		t.Fatalf("LLMProfileFromPreset(deepseek-fast) error = %v", err)
	}
	if fast.Model != "deepseek-v4-flash" {
		t.Fatalf("deepseek-fast model = %q, want full official name deepseek-v4-flash", fast.Model)
	}
	if fast.Model == "v4-flash" {
		t.Fatal("deepseek-fast preset used rejected short model name v4-flash")
	}
}

func TestLLMProfileFromPresetOpenAICompatibleRequiresUserFields(t *testing.T) {
	_, err := LLMProfileFromPreset(LLMProfilePresetOpenAICompatible, LLMProfileConfig{})
	if err == nil {
		t.Fatal("LLMProfileFromPreset(openai-compatible) error = nil, want missing field error")
	}
	if !strings.Contains(err.Error(), "base_url") {
		t.Fatalf("error = %q, want base_url guidance", err.Error())
	}

	profile, err := LLMProfileFromPreset(LLMProfilePresetOpenAICompatible, LLMProfileConfig{
		BaseURL:   "https://example.test/v1",
		Model:     "custom-model",
		APIKeyEnv: "CUSTOM_API_KEY",
	})
	if err != nil {
		t.Fatalf("LLMProfileFromPreset(openai-compatible with overrides) error = %v", err)
	}
	if profile.Provider != "openai-compatible" || profile.BaseURL != "https://example.test/v1" || profile.Model != "custom-model" || profile.APIKeyEnv != "CUSTOM_API_KEY" {
		t.Fatalf("profile = %+v, want override fields", profile)
	}
}

func TestUpsertLLMProfileFromPresetWritesCompleteProfileWithoutEnvMigration(t *testing.T) {
	clearLLMEnv(t)
	workDir := t.TempDir()
	t.Setenv("LORE_LLM_BASE_URL", "https://api.ikuncode.cc/v1")
	t.Setenv("LORE_LLM_MODEL", "gpt-5.4")
	t.Setenv("LORE_LLM_API_KEY", "ikuncode-secret")
	t.Setenv("DEEPSEEK_API_KEY", "deepseek-secret")

	if err := UpsertLLMProfileFromPreset(workDir, "deepseek", LLMProfilePresetDeepSeek, LLMProfileConfig{}); err != nil {
		t.Fatalf("UpsertLLMProfileFromPreset() error = %v", err)
	}
	if err := SetActiveLLMProfile(workDir, "deepseek"); err != nil {
		t.Fatalf("SetActiveLLMProfile() error = %v", err)
	}
	data, err := os.ReadFile(filepath.Join(workDir, ".lore", "config.json"))
	if err != nil {
		t.Fatalf("read workspace config: %v", err)
	}
	text := string(data)
	for _, want := range []string{"deepseek-v4-pro", "https://api.deepseek.com/v1", "DEEPSEEK_API_KEY"} {
		if !strings.Contains(text, want) {
			t.Fatalf("workspace config missing %q:\n%s", want, text)
		}
	}
	for _, forbidden := range []string{"gpt-5.4", "https://api.ikuncode.cc/v1", "ikuncode-secret", "deepseek-secret"} {
		if strings.Contains(text, forbidden) {
			t.Fatalf("workspace config unexpectedly contains %q:\n%s", forbidden, text)
		}
	}

	resolved, _, err := ResolveLLMConfigWithOptions(workDir, LLMPurposeOperator, LoadOptions{UserGlobalPath: absentPath(t)})
	if err != nil {
		t.Fatalf("ResolveLLMConfigWithOptions() error = %v", err)
	}
	if resolved.Profile != "deepseek" || resolved.Model != "deepseek-v4-pro" || resolved.APIKey != "deepseek-secret" {
		t.Fatalf("resolved = %+v, want deepseek preset with env key", resolved)
	}
}

func TestLLMProfileFromPresetRejectsUnknownPreset(t *testing.T) {
	_, err := LLMProfileFromPreset("missing", LLMProfileConfig{})
	if err == nil {
		t.Fatal("LLMProfileFromPreset(missing) error = nil, want error")
	}
	for _, want := range []string{"missing", LLMProfilePresetDeepSeek, LLMProfilePresetDeepSeekFast, LLMProfilePresetOpenAICompatible} {
		if !strings.Contains(err.Error(), want) {
			t.Fatalf("error = %q, want %q", err.Error(), want)
		}
	}
}
