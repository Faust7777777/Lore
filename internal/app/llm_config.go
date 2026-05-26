package app

import (
	"context"
	"strings"

	"obsidian-harness/internal/config"
	openai "obsidian-harness/internal/llm/openai"
	"obsidian-harness/internal/operatoragent"
)

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
