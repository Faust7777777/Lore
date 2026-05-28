package app

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	"obsidian-harness/internal/config"
	openai "obsidian-harness/internal/llm/openai"
	"obsidian-harness/internal/model"
	"obsidian-harness/internal/operatoragent"
)

type LLMProfileView struct {
	Name      string
	Active    bool
	Provider  string
	BaseURL   string
	Model     string
	Timeout   time.Duration
	APIKeyEnv string
	APIKeyRef string
}

type runtimeLLMResolution struct {
	byPurpose     map[string]config.ResolvedLLMConfig
	errors        map[string]error
	diagnostics   []config.LLMDiagnostic
	operatorAgent operatoragent.Agent
}

func resolveRuntimeLLM(cfg config.Config, diagnostics []config.LoadDiagnostic) runtimeLLMResolution {
	purposes := []string{
		config.LLMPurposeOperator,
		config.LLMPurposePersonaExtract,
		config.LLMPurposeProcessSink,
	}
	resolved := runtimeLLMResolution{
		byPurpose: make(map[string]config.ResolvedLLMConfig, len(purposes)),
		errors:    make(map[string]error, len(purposes)),
	}
	for _, purpose := range purposes {
		llmCfg, err := config.ResolveLLMConfigFromLoaded(cfg, diagnostics, purpose)
		resolved.byPurpose[purpose] = llmCfg
		if err != nil {
			resolved.errors[purpose] = err
		}
		resolved.diagnostics = append(resolved.diagnostics, llmCfg.Diagnostic(err))
	}

	operatorCfg := resolved.byPurpose[config.LLMPurposeOperator]
	if err := resolved.errors[config.LLMPurposeOperator]; err != nil {
		resolved.operatorAgent = operatoragent.NewUnavailable(err)
	} else {
		agent, err := operatoragent.NewFromResolvedLLMConfig(operatorCfg)
		if err != nil {
			resolved.errors[config.LLMPurposeOperator] = err
			resolved.diagnostics = replaceLLMDiagnostic(resolved.diagnostics, config.LLMPurposeOperator, operatorCfg.Diagnostic(err))
			resolved.operatorAgent = operatoragent.NewUnavailable(err)
		} else if agent != nil {
			resolved.operatorAgent = agent
		} else {
			resolved.operatorAgent = operatoragent.NewUnavailable(nil)
		}
	}
	return resolved
}

func (r *Runtime) ResolveLLMConfig(purpose string) (config.ResolvedLLMConfig, error) {
	if r == nil {
		return config.ResolvedLLMConfig{}, fmt.Errorf("runtime is nil")
	}
	return config.ResolveLLMConfigFromLoaded(r.Config, r.ConfigDiagnostics, purpose)
}

// LLMIdentity is the key-free projection of a resolved per-purpose LLM
// config that the TUI (and other read-only surfaces) can render
// without ever holding the API key value. Mirrors
// config.ResolvedLLMConfig minus the APIKey field; the surrounding
// APIKeyEnv / APIKeyRef name strings are safe to display because they
// identify WHERE the secret lives, not what it is. BaseURL flows
// through SanitizeLLMBaseURL so URL-embedded credentials, if any, do
// not leak into the dashboard.
type LLMIdentity struct {
	Purpose   string
	Enabled   bool
	Source    config.LLMConfigSource
	Profile   string
	Provider  string
	BaseURL   string
	Model     string
	Timeout   time.Duration
	APIKeyEnv string
	APIKeyRef string
}

// LLMIdentity returns the safe display projection for a resolved
// per-purpose LLM config. Use this from TUI / dashboard surfaces;
// reach for ResolveLLMConfig only when the caller actually needs the
// APIKey to build an LLM client. Errors propagate verbatim from
// ResolveLLMConfig so the caller can distinguish "purpose not
// configured" (returned identity has Enabled=false) from "config
// resolution failed" (error returned).
func (r *Runtime) LLMIdentity(purpose string) (LLMIdentity, error) {
	cfg, err := r.ResolveLLMConfig(purpose)
	if err != nil {
		return LLMIdentity{}, err
	}
	return LLMIdentity{
		Purpose:   cfg.Purpose,
		Enabled:   cfg.Enabled,
		Source:    cfg.Source,
		Profile:   cfg.Profile,
		Provider:  cfg.Provider,
		BaseURL:   config.SanitizeLLMBaseURL(cfg.BaseURL),
		Model:     cfg.Model,
		Timeout:   cfg.Timeout,
		APIKeyEnv: cfg.APIKeyEnv,
		APIKeyRef: cfg.APIKeyRef,
	}, nil
}

func (r *Runtime) ListLLMProfiles() ([]LLMProfileView, error) {
	if r == nil {
		return nil, fmt.Errorf("runtime is nil")
	}
	active := strings.TrimSpace(r.Config.LLM.ActiveProfile)
	profiles := r.Config.LLM.Profiles
	names := make([]string, 0, len(profiles))
	for name := range profiles {
		names = append(names, name)
	}
	sort.Strings(names)
	views := make([]LLMProfileView, 0, len(names))
	for _, name := range names {
		profile := profiles[name]
		views = append(views, LLMProfileView{
			Name:      name,
			Active:    name == active,
			Provider:  strings.TrimSpace(profile.Provider),
			BaseURL:   config.SanitizeLLMBaseURL(profile.BaseURL),
			Model:     strings.TrimSpace(profile.Model),
			Timeout:   profile.Timeout,
			APIKeyEnv: strings.TrimSpace(profile.APIKeyEnv),
			APIKeyRef: strings.TrimSpace(profile.APIKeyRef),
		})
	}
	return views, nil
}

func (r *Runtime) UpsertLLMProfile(profileName string, profile config.LLMProfileConfig) error {
	if r == nil {
		return fmt.Errorf("runtime is nil")
	}
	if err := config.UpsertLLMProfile(r.Config.Paths.WorkDir, profileName, profile); err != nil {
		return err
	}
	return r.reloadLLMConfig()
}

func (r *Runtime) SetActiveLLMProfile(profileName string) error {
	if r == nil {
		return fmt.Errorf("runtime is nil")
	}
	if err := config.SetActiveLLMProfile(r.Config.Paths.WorkDir, profileName); err != nil {
		return err
	}
	return r.reloadLLMConfig()
}

func (r *Runtime) BuildOperatorAgentForModel(profileName string, modelName string) (operatoragent.Agent, error) {
	cfg, err := r.resolveOperatorProfile(profileName)
	if err != nil {
		return nil, err
	}
	modelName = strings.TrimSpace(modelName)
	if modelName == "" {
		modelName = cfg.Model
	}
	if modelName == "" {
		return nil, fmt.Errorf("operator agent: model name is required")
	}
	cfg.Model = modelName
	return operatoragent.NewFromResolvedLLMConfig(cfg)
}

func (r *Runtime) resolveOperatorProfile(profileName string) (config.ResolvedLLMConfig, error) {
	if r == nil {
		return config.ResolvedLLMConfig{}, fmt.Errorf("runtime is nil")
	}
	cfg, err := config.ResolveLLMProfileConfigFromLoaded(r.Config, r.ConfigDiagnostics, config.LLMPurposeOperator, profileName)
	if err != nil {
		return cfg, err
	}
	if !cfg.Enabled {
		return cfg, fmt.Errorf("operator LLM config is disabled")
	}
	return cfg, nil
}

func (r *Runtime) reloadLLMConfig() error {
	if r == nil {
		return fmt.Errorf("runtime is nil")
	}
	opts := r.configLoadOptions
	// Runtime profile edits always write <workdir>/.lore/config.json. Preserve
	// user-global isolation from OpenRuntimeWithConfigOptions, but reload the
	// canonical workspace layer rather than any test-only WorkspacePath override.
	opts.WorkspacePath = ""
	cfg, diagnostics, err := config.LoadWithOptions(r.Config.Paths.WorkDir, opts)
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}
	r.Config.LLM = cfg.LLM
	r.ConfigDiagnostics = diagnostics

	llmRuntime := resolveRuntimeLLM(r.Config, diagnostics)
	processSinkSummarizer, processSinkErr := defaultProcessSinkSummarizer(llmRuntime.byPurpose[config.LLMPurposeProcessSink], llmRuntime.errors[config.LLMPurposeProcessSink])
	personaExtractor, personaErr := defaultPersonaExtractor(llmRuntime.byPurpose[config.LLMPurposePersonaExtract], llmRuntime.errors[config.LLMPurposePersonaExtract])
	personaCfg := llmRuntime.byPurpose[config.LLMPurposePersonaExtract]

	attachUsageSink(processSinkSummarizer, func(rec model.UsageRecord) error {
		return r.RecordUsage([]model.UsageRecord{rec})
	})
	attachPersonaExtractorUsageSink(personaExtractor, func(rec model.UsageRecord) error {
		return r.RecordUsage([]model.UsageRecord{rec})
	})

	r.LLMDiagnostics = llmRuntime.diagnostics
	r.OperatorAgent = llmRuntime.operatorAgent
	r.ProcessSinkSummarizer = processSinkSummarizer
	r.processSinkSummarizerErr = processSinkErr
	r.PersonaExtractor = personaExtractor
	r.personaExtractorErr = personaErr
	r.PersonaExtractProvider = personaExtractIdentity(personaCfg, personaErr, llmProvider)
	r.PersonaExtractModel = personaExtractIdentity(personaCfg, personaErr, func(c config.ResolvedLLMConfig) string { return c.Model })
	r.PersonaExtractBaseURL = personaExtractIdentity(personaCfg, personaErr, func(c config.ResolvedLLMConfig) string { return config.SanitizeLLMBaseURL(c.BaseURL) })
	return nil
}

func replaceLLMDiagnostic(diagnostics []config.LLMDiagnostic, purpose string, replacement config.LLMDiagnostic) []config.LLMDiagnostic {
	for i := range diagnostics {
		if diagnostics[i].Purpose == purpose {
			diagnostics[i] = replacement
			return diagnostics
		}
	}
	return append(diagnostics, replacement)
}

func openAIClientFromResolvedLLMConfig(cfg config.ResolvedLLMConfig) (*openai.Client, string, error) {
	modelName, err := operatoragent.ResolveModel(context.Background(), operatoragent.EnvConfig{
		BaseURL: cfg.BaseURL,
		APIKey:  cfg.APIKey,
		Model:   cfg.Model,
		Timeout: cfg.Timeout,
	})
	if err != nil {
		return nil, "", err
	}
	client, err := openai.NewClient(openai.Config{
		BaseURL: cfg.BaseURL,
		APIKey:  cfg.APIKey,
		Model:   modelName,
		Timeout: cfg.Timeout,
	})
	if err != nil {
		return nil, "", err
	}
	return client, modelName, nil
}

func llmProvider(cfg config.ResolvedLLMConfig) string {
	if strings.TrimSpace(cfg.Provider) == "" {
		return "openai-compatible"
	}
	return strings.TrimSpace(cfg.Provider)
}
