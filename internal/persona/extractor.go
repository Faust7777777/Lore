package persona

import (
	"context"
	"fmt"
	"strings"

	openai "obsidian-harness/internal/llm/openai"
	"obsidian-harness/internal/model"
)

// PersonaCandidateExtractor is the contract every Lore subsystem
// consumes to mine persona candidates from a user message. The single
// method shape is small on purpose: every produced candidate is
// independently reviewable, so callers do not need streaming, batch,
// or partial-failure semantics here. Failures return a non-nil error
// and an empty result; callers (P4+) are expected to log and skip,
// never to retry blindly.
type PersonaCandidateExtractor interface {
	Extract(ctx context.Context, input PersonaExtractionInput) (PersonaExtractionResult, error)
}

// completionClient is the minimal slice of openai.Client the extractor
// needs. Mirroring this interface lets tests inject scripted fakes
// without dragging in HTTP plumbing.
type completionClient interface {
	ChatCompletion(ctx context.Context, req openai.ChatCompletionRequest) (openai.ChatCompletionResponse, error)
}

// LLMExtractor is the production implementation backed by an
// OpenAI-compatible chat completion client. provider/model are stored
// so emitted UsageRecords can be attributed in `lore usage`.
//
// usageSink mirrors the pattern used by app.modelProcessSinkSummarizer:
// callers (P4) inject a sink that funnels into runtime.RecordUsage.
// When unset (P1+P2 today, or in tests), usage is silently dropped --
// the extractor's job is to produce candidates, not to fail because
// no one is listening to its accounting events.
type LLMExtractor struct {
	client    completionClient
	provider  string
	model     string
	usageSink func(model.UsageRecord) error
}

// NewLLMExtractor constructs an extractor with the given client and
// provider/model metadata. provider should be a short identifier such
// as "openai-compatible" or "test"; model is the resolved model name
// that ultimately serves the request.
func NewLLMExtractor(client completionClient, provider, model string) *LLMExtractor {
	return &LLMExtractor{
		client:   client,
		provider: strings.TrimSpace(provider),
		model:    strings.TrimSpace(model),
	}
}

// AttachUsageSink wires a usage sink into the extractor. Idempotent
// replace -- callers (P4 OpenRuntime two-phase wiring) install the
// real sink after construction so the model+client can be assembled
// without knowing about the runtime layer. Nil is a valid value and
// disables usage emission.
func (e *LLMExtractor) AttachUsageSink(sink func(model.UsageRecord) error) {
	if e == nil {
		return
	}
	e.usageSink = sink
}

// Extract issues one ChatCompletion against the LLM, parses the JSON
// response, and returns vetted candidates. Empty UserText short
// circuits with an empty result (no LLM call) so assistant-only
// turns or pure-pending-confirmation turns do not consume tokens.
//
// Error semantics:
//   - ChatCompletion failures bubble up unchanged; usage is NOT
//     emitted because no tokens were billed.
//   - Parse failures of an otherwise-billed response still emit usage
//     (mirrors process-sink summarizer policy: tokens were spent;
//     don't lose the cost record because the response was malformed).
func (e *LLMExtractor) Extract(ctx context.Context, input PersonaExtractionInput) (PersonaExtractionResult, error) {
	if e == nil {
		return PersonaExtractionResult{}, fmt.Errorf("persona extractor: nil receiver")
	}
	if strings.TrimSpace(input.UserText) == "" {
		// Assistant-only turn or empty input: no candidates, no LLM
		// call, no warnings. P4 will use the same guard at its call
		// site but defensive duplication here keeps the contract
		// crisp for direct callers.
		return PersonaExtractionResult{}, nil
	}

	system, user := buildExtractionPrompt(input)
	resp, err := e.client.ChatCompletion(ctx, openai.ChatCompletionRequest{
		Messages: []openai.Message{
			{Role: "system", Content: system},
			{Role: "user", Content: user},
		},
		Temperature: 0,
	})
	if err != nil {
		return PersonaExtractionResult{}, fmt.Errorf("persona extractor: model request failed: %w", err)
	}

	if e.usageSink != nil {
		_ = e.usageSink(model.UsageRecord{
			Provider:         e.provider,
			Model:            e.model,
			AgentID:          input.SourceAgentID,
			SessionID:        input.SourceSessionID,
			PromptTokens:     resp.PromptTokens,
			CompletionTokens: resp.CompletionTokens,
			RecordedAt:       input.ObservedAt,
			Purpose:          model.UsagePurposePersonaExtract,
		})
	}

	return parseExtractionResponse(resp.Content, input)
}
