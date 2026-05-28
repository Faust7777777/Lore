package app

import (
	"fmt"
	"strings"

	"obsidian-harness/internal/config"
	"obsidian-harness/internal/model"
	"obsidian-harness/internal/persona"
)

type PersonaExtractorBinding struct {
	Extractor persona.PersonaCandidateExtractor
	Provider  string
	Model     string
	BaseURL   string
}

// defaultPersonaExtractor returns a persona.PersonaCandidateExtractor
// built from the runtime-resolved LLM config. Workspace/user-global
// profiles take precedence over legacy env-only config; the resolved
// profile still reads the actual API key from api_key_env so secrets
// stay out of repo, vault, and sessionlog. A disabled config returns
// (nil, nil), preserving the fire-and-forget call site's nil guard.
func defaultPersonaExtractor(cfg config.ResolvedLLMConfig, resolveErr error) (persona.PersonaCandidateExtractor, error) {
	if resolveErr != nil {
		return nil, resolveErr
	}
	if !cfg.Enabled {
		return nil, nil
	}
	client, modelName, err := openAIClientFromResolvedLLMConfig(cfg)
	if err != nil {
		return nil, err
	}
	return persona.NewLLMExtractor(client, llmProvider(cfg), modelName), nil
}

// attachPersonaExtractorUsageSink wires a usage sink into a
// LLMExtractor so emitted UsageRecord values (Purpose=persona_extract)
// flow into runtime.RecordUsage. Mirrors attachUsageSink in
// processsink_summarizer.go: the helper is no-op for non-LLM
// implementations (e.g. fakes in tests) so the test surface stays
// unchanged.
func attachPersonaExtractorUsageSink(extractor persona.PersonaCandidateExtractor, sink func(model.UsageRecord) error) {
	if e, ok := extractor.(*persona.LLMExtractor); ok {
		e.AttachUsageSink(sink)
	}
}

// BuildPersonaExtractorForOperatorModel builds a session-scoped persona
// extractor for an interactive /model switch. The endpoint, provider, and
// api_key_env still come from the resolved operator LLM config; only the model
// name is overridden by the user's in-session model choice. This keeps chat and
// fire-and-forget persona extraction aligned without teaching the CLI how to
// construct persona extractors or handle usage attribution.
func (r *Runtime) BuildPersonaExtractorForOperatorModel(modelName string) (PersonaExtractorBinding, error) {
	return r.BuildPersonaExtractorForModel("", modelName)
}

// BuildPersonaExtractorForModel mirrors BuildOperatorAgentForModel for
// interactive /model switches: the optional profile selects the resolved
// endpoint/provider/api_key_env, while modelName overrides only the session's
// active model. The returned binding carries key-free identity metadata for
// persona-extract logs.
func (r *Runtime) BuildPersonaExtractorForModel(profileName string, modelName string) (PersonaExtractorBinding, error) {
	modelName = strings.TrimSpace(modelName)
	if modelName == "" {
		return PersonaExtractorBinding{}, fmt.Errorf("persona extractor: model name is required")
	}
	if r == nil {
		return PersonaExtractorBinding{}, fmt.Errorf("persona extractor: runtime is nil")
	}
	cfg, err := r.resolveOperatorProfile(profileName)
	if err != nil {
		return PersonaExtractorBinding{}, err
	}
	cfg.Model = modelName
	extractor, err := defaultPersonaExtractor(cfg, nil)
	if err != nil {
		return PersonaExtractorBinding{}, err
	}
	attachPersonaExtractorUsageSink(extractor, func(rec model.UsageRecord) error {
		return r.RecordUsage([]model.UsageRecord{rec})
	})
	return PersonaExtractorBinding{
		Extractor: extractor,
		Provider:  llmProvider(cfg),
		Model:     modelName,
		BaseURL:   config.SanitizeLLMBaseURL(cfg.BaseURL),
	}, nil
}

// personaExtractIdentity returns one of the resolved-config identity
// strings (provider, model, base URL) when the persona-extract LLM
// resolved without error. resolveErr non-nil or Enabled false yields
// "" so the console wiring layer leaves Session.PersonaExtractModelInfo
// fields empty rather than logging stale values from a failed resolve.
// The selector indirection lets one helper cover provider / model /
// base URL without duplicating the gating logic.
func personaExtractIdentity(cfg config.ResolvedLLMConfig, resolveErr error, selector func(config.ResolvedLLMConfig) string) string {
	if resolveErr != nil || !cfg.Enabled {
		return ""
	}
	return selector(cfg)
}
