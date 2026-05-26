package config

import (
	"fmt"
	"os"
	"sort"
	"strings"
	"time"
)

const (
	LLMPurposeOperator       = "operator"
	LLMPurposePersonaExtract = "persona_extract"
	LLMPurposeProcessSink    = "process_sink"
)

type LLMConfigSource string

const (
	LLMSourceDefault    LLMConfigSource = "default"
	LLMSourceUserGlobal LLMConfigSource = "user_global"
	LLMSourceWorkspace  LLMConfigSource = "workspace"
	LLMSourceEnv        LLMConfigSource = "env"
)

type ResolvedLLMConfig struct {
	Enabled   bool
	Purpose   string
	Source    LLMConfigSource
	Profile   string
	Provider  string
	BaseURL   string
	Model     string
	Timeout   time.Duration
	APIKey    string
	APIKeyEnv string
	APIKeyRef string
}

type LLMDiagnostic struct {
	Purpose   string
	Enabled   bool
	Source    LLMConfigSource
	Profile   string
	Provider  string
	BaseURL   string
	Model     string
	Timeout   time.Duration
	APIKeyEnv string
	APIKeyRef string
	Err       error
}

func (cfg ResolvedLLMConfig) Diagnostic(err error) LLMDiagnostic {
	return LLMDiagnostic{
		Purpose:   cfg.Purpose,
		Enabled:   cfg.Enabled,
		Source:    cfg.Source,
		Profile:   cfg.Profile,
		Provider:  cfg.Provider,
		BaseURL:   cfg.BaseURL,
		Model:     cfg.Model,
		Timeout:   cfg.Timeout,
		APIKeyEnv: cfg.APIKeyEnv,
		APIKeyRef: cfg.APIKeyRef,
		Err:       err,
	}
}

func ResolveLLMConfig(workDir string, purpose string) (ResolvedLLMConfig, error) {
	resolved, _, err := ResolveLLMConfigWithOptions(workDir, purpose, LoadOptions{})
	return resolved, err
}

func ResolveLLMConfigWithOptions(workDir string, purpose string, opts LoadOptions) (ResolvedLLMConfig, []LoadDiagnostic, error) {
	cfg, diagnostics, err := LoadWithOptions(workDir, opts)
	if err != nil {
		return ResolvedLLMConfig{}, diagnostics, err
	}
	resolved, err := ResolveLLMConfigFromLoaded(cfg, diagnostics, purpose)
	return resolved, diagnostics, err
}

func ResolveLLMConfigFromLoaded(cfg Config, diagnostics []LoadDiagnostic, purpose string) (ResolvedLLMConfig, error) {
	purpose = normalizeLLMPurpose(purpose)
	if hasConfiguredLLM(cfg) {
		return resolveConfiguredLLM(cfg, diagnostics, purpose)
	}
	return resolveEnvLLM(purpose)
}

func normalizeLLMPurpose(purpose string) string {
	purpose = strings.TrimSpace(purpose)
	if purpose == "" {
		return LLMPurposeOperator
	}
	return purpose
}

func hasConfiguredLLM(cfg Config) bool {
	return strings.TrimSpace(cfg.LLM.ActiveProfile) != "" || len(cfg.LLM.Profiles) > 0
}

func resolveConfiguredLLM(cfg Config, diagnostics []LoadDiagnostic, purpose string) (ResolvedLLMConfig, error) {
	profileName, profile, err := selectLLMProfile(cfg.LLM)
	resolved := ResolvedLLMConfig{
		Enabled:   true,
		Purpose:   purpose,
		Source:    llmSourceFromDiagnostics(diagnostics),
		Profile:   profileName,
		Provider:  withLLMDefault(strings.TrimSpace(profile.Provider), "openai-compatible"),
		BaseURL:   strings.TrimSpace(profile.BaseURL),
		Model:     strings.TrimSpace(profile.Model),
		Timeout:   profile.Timeout,
		APIKeyEnv: strings.TrimSpace(profile.APIKeyEnv),
		APIKeyRef: strings.TrimSpace(profile.APIKeyRef),
	}
	if resolved.Timeout <= 0 {
		resolved.Timeout = 30 * time.Second
	}
	if err != nil {
		return resolved, err
	}
	if resolved.BaseURL == "" {
		return resolved, fmt.Errorf("llm: profile %q base_url must not be empty", profileName)
	}
	if resolved.APIKeyEnv == "" {
		if resolved.APIKeyRef != "" {
			return resolved, fmt.Errorf("llm: profile %q api_key_ref is not supported yet; use api_key_env", profileName)
		}
		return resolved, fmt.Errorf("llm: profile %q api_key_env must be set; API keys must stay outside repo, vault, and sessionlog", profileName)
	}
	apiKey := strings.TrimSpace(os.Getenv(resolved.APIKeyEnv))
	if apiKey == "" {
		return resolved, fmt.Errorf("llm: profile %q api_key_env %s is not set", profileName, resolved.APIKeyEnv)
	}
	resolved.APIKey = apiKey
	return resolved, nil
}

func selectLLMProfile(llm LLMConfig) (string, LLMProfileConfig, error) {
	profiles := llm.Profiles
	active := strings.TrimSpace(llm.ActiveProfile)
	if len(profiles) == 0 {
		return active, LLMProfileConfig{}, fmt.Errorf("llm: active_profile %q has no profiles", active)
	}
	if active == "" {
		if profile, ok := profiles["default"]; ok {
			return "default", profile, nil
		}
		if len(profiles) == 1 {
			for name, profile := range profiles {
				return name, profile, nil
			}
		}
		names := make([]string, 0, len(profiles))
		for name := range profiles {
			names = append(names, name)
		}
		sort.Strings(names)
		return "", LLMProfileConfig{}, fmt.Errorf("llm: active_profile is required when multiple profiles exist: %s", strings.Join(names, ", "))
	}
	profile, ok := profiles[active]
	if !ok {
		return active, LLMProfileConfig{}, fmt.Errorf("llm: active_profile %q not found", active)
	}
	return active, profile, nil
}

func llmSourceFromDiagnostics(diagnostics []LoadDiagnostic) LLMConfigSource {
	for i := len(diagnostics) - 1; i >= 0; i-- {
		diag := diagnostics[i]
		if diag.Status != LayerStatusLoaded || !diag.HasLLM {
			continue
		}
		switch diag.Source {
		case LayerWorkspace:
			return LLMSourceWorkspace
		case LayerUserGlobal:
			return LLMSourceUserGlobal
		case LayerDefault:
			return LLMSourceDefault
		}
	}
	return LLMSourceDefault
}

func resolveEnvLLM(purpose string) (ResolvedLLMConfig, error) {
	base := firstLLMEnv(llmEnvNames(purpose, "BASE_URL")...)
	key := firstLLMEnv(llmEnvNames(purpose, "API_KEY")...)
	modelName := firstLLMEnv(llmEnvNames(purpose, "MODEL")...)
	provider := firstLLMEnv(llmEnvNames(purpose, "PROVIDER")...)
	timeoutRaw := firstLLMEnv(llmEnvNames(purpose, "TIMEOUT")...)

	if base.Value == "" && key.Value == "" && modelName.Value == "" && provider.Value == "" && timeoutRaw.Value == "" {
		return ResolvedLLMConfig{Purpose: purpose, Source: LLMSourceDefault}, nil
	}
	resolved := ResolvedLLMConfig{
		Enabled:   true,
		Purpose:   purpose,
		Source:    LLMSourceEnv,
		Provider:  withLLMDefault(provider.Value, "openai-compatible"),
		BaseURL:   base.Value,
		Model:     modelName.Value,
		Timeout:   30 * time.Second,
		APIKey:    key.Value,
		APIKeyEnv: key.Name,
	}
	if timeoutRaw.Value != "" {
		timeout, err := time.ParseDuration(timeoutRaw.Value)
		if err != nil {
			return resolved, fmt.Errorf("llm: invalid timeout %q from %s: %w", timeoutRaw.Value, timeoutRaw.Name, err)
		}
		resolved.Timeout = timeout
	}
	if resolved.BaseURL == "" || resolved.APIKey == "" {
		return resolved, fmt.Errorf("llm: base_url and api key must be set together for %s (legacy LORE_LLM_* and OBSIDIAN_HARNESS_LLM_* env are supported)", purpose)
	}
	return resolved, nil
}

type envValue struct {
	Name  string
	Value string
}

func firstLLMEnv(names ...string) envValue {
	for _, name := range names {
		value := strings.TrimSpace(os.Getenv(name))
		if value != "" {
			return envValue{Name: name, Value: value}
		}
	}
	return envValue{}
}

func llmEnvNames(purpose string, suffix string) []string {
	suffix = strings.TrimSpace(suffix)
	names := make([]string, 0, 8)
	switch purpose {
	case LLMPurposeOperator:
		names = append(names, "LORE_OPERATOR_"+suffix, "OBSIDIAN_HARNESS_OPERATOR_"+suffix)
	case LLMPurposePersonaExtract:
		names = append(names, "LORE_PERSONA_EXTRACT_"+suffix, "OBSIDIAN_HARNESS_PERSONA_EXTRACT_"+suffix)
	case LLMPurposeProcessSink:
		names = append(names, "LORE_PROCESS_SINK_"+suffix, "OBSIDIAN_HARNESS_PROCESS_SINK_"+suffix)
	}
	names = append(names, "LORE_LLM_"+suffix, "OBSIDIAN_HARNESS_LLM_"+suffix)
	return names
}

func withLLMDefault(value string, fallback string) string {
	value = strings.TrimSpace(value)
	if value != "" {
		return value
	}
	return fallback
}
