package config

import (
	"fmt"
	"sort"
	"strings"
)

const (
	LLMProfilePresetDeepSeek           = "deepseek"
	LLMProfilePresetDeepSeekFast       = "deepseek-fast"
	LLMProfilePresetOpenAICompatible   = "openai-compatible"
	deepSeekBaseURL                    = "https://api.deepseek.com/v1"
	deepSeekDefaultModel               = "deepseek-v4-pro"
	deepSeekFastDefaultModel           = "deepseek-v4-flash"
	deepSeekDefaultAPIKeyEnv           = "DEEPSEEK_API_KEY"
	openAICompatibleDefaultProvider    = "openai-compatible"
	openAICompatibleDefaultProfileName = "openai-compatible"
)

type LLMProfilePreset struct {
	Name        string
	Provider    string
	BaseURL     string
	Model       string
	APIKeyEnv   string
	Description string
	Requires    []string
}

func BuiltinLLMProfilePresets() []LLMProfilePreset {
	presets := []LLMProfilePreset{
		{
			Name:        LLMProfilePresetDeepSeek,
			Provider:    "deepseek",
			BaseURL:     deepSeekBaseURL,
			Model:       deepSeekDefaultModel,
			APIKeyEnv:   deepSeekDefaultAPIKeyEnv,
			Description: "DeepSeek official OpenAI-compatible endpoint; use full model name deepseek-v4-pro.",
		},
		{
			Name:        LLMProfilePresetDeepSeekFast,
			Provider:    "deepseek",
			BaseURL:     deepSeekBaseURL,
			Model:       deepSeekFastDefaultModel,
			APIKeyEnv:   deepSeekDefaultAPIKeyEnv,
			Description: "DeepSeek official faster model; use full model name deepseek-v4-flash.",
		},
		{
			Name:        LLMProfilePresetOpenAICompatible,
			Provider:    openAICompatibleDefaultProvider,
			Description: "Generic OpenAI-compatible endpoint; caller must provide base_url, model, and api_key_env.",
			Requires:    []string{"base_url", "model", "api_key_env"},
		},
	}
	return presets
}

func LLMProfileFromPreset(presetName string, overrides LLMProfileConfig) (LLMProfileConfig, error) {
	preset, ok := lookupLLMProfilePreset(presetName)
	if !ok {
		return LLMProfileConfig{}, fmt.Errorf("config: unknown llm profile preset %q; available: %s", strings.TrimSpace(presetName), strings.Join(llmProfilePresetNames(), ", "))
	}
	profile := LLMProfileConfig{
		Provider:  preset.Provider,
		BaseURL:   preset.BaseURL,
		Model:     preset.Model,
		APIKeyEnv: preset.APIKeyEnv,
	}
	profile = mergeLLMProfileOverrides(profile, overrides)
	profile = normalizeLLMProfileConfig(profile)
	if err := validateWorkspaceLLMProfile(preset.Name, profile); err != nil {
		return LLMProfileConfig{}, err
	}
	return profile, nil
}

func UpsertLLMProfileFromPreset(workDir string, profileName string, presetName string, overrides LLMProfileConfig) error {
	profileName = strings.TrimSpace(profileName)
	if profileName == "" {
		profileName = strings.TrimSpace(presetName)
	}
	if profileName == "" {
		profileName = openAICompatibleDefaultProfileName
	}
	profile, err := LLMProfileFromPreset(presetName, overrides)
	if err != nil {
		return err
	}
	return UpsertLLMProfile(workDir, profileName, profile)
}

func lookupLLMProfilePreset(name string) (LLMProfilePreset, bool) {
	name = strings.TrimSpace(name)
	for _, preset := range BuiltinLLMProfilePresets() {
		if preset.Name == name {
			return preset, true
		}
	}
	return LLMProfilePreset{}, false
}

func llmProfilePresetNames() []string {
	presets := BuiltinLLMProfilePresets()
	names := make([]string, 0, len(presets))
	for _, preset := range presets {
		names = append(names, preset.Name)
	}
	sort.Strings(names)
	return names
}

func mergeLLMProfileOverrides(base LLMProfileConfig, overrides LLMProfileConfig) LLMProfileConfig {
	if strings.TrimSpace(overrides.Provider) != "" {
		base.Provider = overrides.Provider
	}
	if strings.TrimSpace(overrides.BaseURL) != "" {
		base.BaseURL = overrides.BaseURL
	}
	if strings.TrimSpace(overrides.Model) != "" {
		base.Model = overrides.Model
	}
	if overrides.Timeout > 0 {
		base.Timeout = overrides.Timeout
	}
	if strings.TrimSpace(overrides.APIKeyEnv) != "" {
		base.APIKeyEnv = overrides.APIKeyEnv
	}
	if strings.TrimSpace(overrides.APIKeyRef) != "" {
		base.APIKeyRef = overrides.APIKeyRef
	}
	return base
}
