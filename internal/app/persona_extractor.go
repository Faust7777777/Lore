package app

import (
	"context"

	openai "obsidian-harness/internal/llm/openai"
	"obsidian-harness/internal/model"
	"obsidian-harness/internal/operatoragent"
	"obsidian-harness/internal/persona"
)

// defaultPersonaExtractor returns a persona.PersonaCandidateExtractor
// built from the same LORE_LLM_* env config the operator agent uses,
// or (nil, nil) when those env vars are not set so the runtime can
// still operate without an LLM (the console fire-and-forget call site
// guards on a nil extractor). Errors from env parsing or model
// resolution bubble up so OpenRuntime can stash them on
// Runtime.personaExtractorErr for diagnostic surfacing.
//
// Reuses operatoragent.LoadEnvConfig + ResolveModel rather than
// growing a parallel env contract, mirroring how
// defaultProcessSinkSummarizer composes its client. The shared env
// keeps "no LLM configured" a single decision rather than three.
func defaultPersonaExtractor() (persona.PersonaCandidateExtractor, error) {
	cfg, enabled, err := operatoragent.LoadEnvConfig()
	if err != nil {
		return nil, err
	}
	if !enabled {
		return nil, nil
	}
	modelName, err := operatoragent.ResolveModel(context.Background(), cfg)
	if err != nil {
		return nil, err
	}
	client, err := openai.NewClient(openai.Config{
		BaseURL: cfg.BaseURL,
		APIKey:  cfg.APIKey,
		Model:   modelName,
		Timeout: cfg.Timeout,
	})
	if err != nil {
		return nil, err
	}
	return persona.NewLLMExtractor(client, "openai-compatible", modelName), nil
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
