package persona

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	openai "obsidian-harness/internal/llm/openai"
	"obsidian-harness/internal/model"
)

// scriptedCompletionClient satisfies the package-private
// completionClient interface via Go's structural typing. Tests pass
// it directly to NewLLMExtractor without depending on the openai
// HTTP plumbing.
type scriptedCompletionClient struct {
	resp openai.ChatCompletionResponse
	err  error
	req  openai.ChatCompletionRequest
}

func (s *scriptedCompletionClient) ChatCompletion(_ context.Context, req openai.ChatCompletionRequest) (openai.ChatCompletionResponse, error) {
	s.req = req
	return s.resp, s.err
}

func newInput(userText string) PersonaExtractionInput {
	return PersonaExtractionInput{
		UserText:        userText,
		SourceKind:      SourceConsole,
		SourceSessionID: "lore-session-1",
		SourceAgentID:   "codex",
		ObservedAt:      time.Date(2026, 5, 19, 10, 0, 0, 0, time.UTC),
	}
}

func TestLLMExtractorParsesValidJSON(t *testing.T) {
	client := &scriptedCompletionClient{
		resp: openai.ChatCompletionResponse{
			Content: `{
  "candidates": [
    {
      "field": "major",
      "proposed_value": "economics and management",
      "evidence_quote": "I'm majoring in economics and management",
      "reason": "stable profile fact",
      "confidence": "high",
      "conflict": false
    }
  ],
  "warnings": []
}`,
		},
	}
	extractor := NewLLMExtractor(client, "test", "test-model")
	input := newInput("Hi, I'm majoring in economics and management at DLUT.")

	result, err := extractor.Extract(context.Background(), input)
	if err != nil {
		t.Fatalf("Extract() error = %v", err)
	}
	if len(result.Candidates) != 1 {
		t.Fatalf("len(Candidates) = %d, want 1; warnings=%+v", len(result.Candidates), result.Warnings)
	}
	got := result.Candidates[0]
	if got.Field != "major" || got.ProposedValue != "economics and management" {
		t.Fatalf("candidate fields = %+v", got)
	}
	if got.Confidence != ConfidenceHigh {
		t.Fatalf("confidence = %q, want high", got.Confidence)
	}
	if got.SourceKind != SourceConsole || got.SourceSessionID != "lore-session-1" {
		t.Fatalf("source metadata = kind=%q session=%q, want console/lore-session-1", got.SourceKind, got.SourceSessionID)
	}
	if !got.ObservedAt.Equal(input.ObservedAt) {
		t.Fatalf("ObservedAt = %v, want %v", got.ObservedAt, input.ObservedAt)
	}
}

func TestLLMExtractorRejectsInvalidJSON(t *testing.T) {
	client := &scriptedCompletionClient{
		resp: openai.ChatCompletionResponse{Content: `definitely not json`},
	}
	extractor := NewLLMExtractor(client, "test", "test-model")

	_, err := extractor.Extract(context.Background(), newInput("hello"))
	if err == nil {
		t.Fatal("Extract() error = nil, want parse failure")
	}
	if !strings.Contains(err.Error(), "no valid json object found") {
		t.Fatalf("error message = %q, want no valid json object found", err.Error())
	}
}

func TestLLMExtractorDiscardsParaphrasedEvidenceQuote(t *testing.T) {
	client := &scriptedCompletionClient{
		resp: openai.ChatCompletionResponse{
			Content: `{
  "candidates": [
    {
      "field": "major",
      "proposed_value": "economics",
      "evidence_quote": "user is studying economics",
      "confidence": "high"
    }
  ]
}`,
		},
	}
	extractor := NewLLMExtractor(client, "test", "test-model")
	input := newInput("I'm in the economics program at DLUT.")

	result, err := extractor.Extract(context.Background(), input)
	if err != nil {
		t.Fatalf("Extract() error = %v", err)
	}
	if len(result.Candidates) != 0 {
		t.Fatalf("paraphrased evidence_quote leaked into candidates: %+v", result.Candidates)
	}
	if len(result.Warnings) == 0 || !strings.Contains(strings.Join(result.Warnings, " "), "evidence_quote not a substring") {
		t.Fatalf("warnings missing paraphrase notice; got %+v", result.Warnings)
	}
}

func TestLLMExtractorDiscardsLowConfidenceCandidate(t *testing.T) {
	client := &scriptedCompletionClient{
		resp: openai.ChatCompletionResponse{
			Content: `{
  "candidates": [
    {
      "field": "hobby",
      "proposed_value": "running",
      "evidence_quote": "running",
      "confidence": "low"
    },
    {
      "field": "city",
      "proposed_value": "Dalian",
      "evidence_quote": "Dalian",
      "confidence": "medium"
    }
  ]
}`,
		},
	}
	extractor := NewLLMExtractor(client, "test", "test-model")
	input := newInput("I started running while living in Dalian.")

	result, err := extractor.Extract(context.Background(), input)
	if err != nil {
		t.Fatalf("Extract() error = %v", err)
	}
	if len(result.Candidates) != 1 || result.Candidates[0].Field != "city" {
		t.Fatalf("expected only the medium-confidence city candidate; got %+v", result.Candidates)
	}
	joined := strings.Join(result.Warnings, " ")
	if !strings.Contains(joined, "confidence=") || !strings.Contains(joined, "low") {
		t.Fatalf("warnings missing low-confidence notice; got %+v", result.Warnings)
	}
}

func TestLLMExtractorParserOverridesConflictWhenCurrentDiffers(t *testing.T) {
	// LLM self-reported conflict=false but current_value contradicts
	// proposed_value. Parser must still flag conflict=true.
	client := &scriptedCompletionClient{
		resp: openai.ChatCompletionResponse{
			Content: `{
  "candidates": [
    {
      "field": "city",
      "proposed_value": "Beijing",
      "current_value": "Dalian",
      "evidence_quote": "Beijing now",
      "confidence": "high",
      "conflict": false
    }
  ]
}`,
		},
	}
	extractor := NewLLMExtractor(client, "test", "test-model")
	input := newInput("Actually I moved, I live in Beijing now.")

	result, err := extractor.Extract(context.Background(), input)
	if err != nil {
		t.Fatalf("Extract() error = %v", err)
	}
	if len(result.Candidates) != 1 || !result.Candidates[0].Conflict {
		t.Fatalf("parser must promote Conflict=true; got %+v", result.Candidates)
	}
}

func TestLLMExtractorPreservesLLMReportedConflict(t *testing.T) {
	client := &scriptedCompletionClient{
		resp: openai.ChatCompletionResponse{
			Content: `{
  "candidates": [
    {
      "field": "role",
      "proposed_value": "product manager",
      "current_value": "",
      "evidence_quote": "product manager",
      "confidence": "high",
      "conflict": true
    }
  ]
}`,
		},
	}
	extractor := NewLLMExtractor(client, "test", "test-model")
	input := newInput("I'm a product manager actually, not a developer.")

	result, err := extractor.Extract(context.Background(), input)
	if err != nil {
		t.Fatalf("Extract() error = %v", err)
	}
	if len(result.Candidates) != 1 || !result.Candidates[0].Conflict {
		t.Fatalf("LLM-reported conflict must be preserved; got %+v", result.Candidates)
	}
}

func TestLLMExtractorSkipsAssistantOnlyInput(t *testing.T) {
	client := &scriptedCompletionClient{
		// Even if the LLM hypothetically returned candidates here,
		// the extractor must not call it at all because UserText is
		// empty.
		resp: openai.ChatCompletionResponse{Content: `{"candidates":[{"field":"x","proposed_value":"y","evidence_quote":"y","confidence":"high"}]}`},
	}
	extractor := NewLLMExtractor(client, "test", "test-model")
	input := PersonaExtractionInput{
		UserText:         "",
		AssistantContext: "Tell me about yourself.",
		SourceKind:       SourceConsole,
		ObservedAt:       time.Now().UTC(),
	}

	result, err := extractor.Extract(context.Background(), input)
	if err != nil {
		t.Fatalf("Extract(empty) error = %v", err)
	}
	if len(result.Candidates) != 0 || len(result.Warnings) != 0 {
		t.Fatalf("expected empty result for assistant-only input; got %+v / warnings=%+v", result.Candidates, result.Warnings)
	}
	// Sanity: client must not have seen any request.
	if client.req.Messages != nil {
		t.Fatalf("client.req populated despite empty user_text: %+v", client.req)
	}
}

func TestLLMExtractorAcceptsChineseEvidenceQuote(t *testing.T) {
	client := &scriptedCompletionClient{
		resp: openai.ChatCompletionResponse{
			Content: `{
  "candidates": [
    {
      "field": "major",
      "proposed_value": "经济管理",
      "evidence_quote": "我在读经济管理",
      "confidence": "high",
      "conflict": false
    }
  ]
}`,
		},
	}
	extractor := NewLLMExtractor(client, "test", "test-model")
	input := newInput("我在读经济管理,大三在校生。")

	result, err := extractor.Extract(context.Background(), input)
	if err != nil {
		t.Fatalf("Extract() error = %v", err)
	}
	if len(result.Candidates) != 1 {
		t.Fatalf("Chinese substring match failed: %+v / warnings=%+v", result.Candidates, result.Warnings)
	}
	got := result.Candidates[0]
	if got.ProposedValue != "经济管理" || got.EvidenceQuote != "我在读经济管理" {
		t.Fatalf("Chinese candidate fields = %+v", got)
	}
}

func TestBuildExtractionPromptForbidsAssistantAndTransientExtraction(t *testing.T) {
	system, user := buildExtractionPrompt(PersonaExtractionInput{
		UserText:              "I live in Dalian.",
		AssistantContext:      "Where do you live?",
		CurrentPersonaExcerpt: "Major: economics",
		SystemRulesExcerpt:    "Lore governs persona updates through draft review.",
	})
	for _, want := range []string{
		"verbatim substring",
		"NEVER extract",
		"transient moods",
		"one-shot task context",
		"assistant inferences",
	} {
		if !strings.Contains(system, want) {
			t.Fatalf("system prompt missing %q:\n%s", want, system)
		}
	}
	if !strings.Contains(user, "Previous assistant message") || !strings.Contains(user, "do NOT extract from this") {
		t.Fatalf("user prompt missing assistant-context disclaimer:\n%s", user)
	}
	if !strings.Contains(user, "User message") || !strings.Contains(user, "I live in Dalian.") {
		t.Fatalf("user prompt missing user_text section:\n%s", user)
	}
	if !strings.Contains(user, "Major: economics") {
		t.Fatalf("user prompt missing current persona excerpt:\n%s", user)
	}
}

func TestLLMExtractorEmitsUsageWithPersonaExtractPurpose(t *testing.T) {
	client := &scriptedCompletionClient{
		resp: openai.ChatCompletionResponse{
			Content:          `{"candidates":[], "warnings":[]}`,
			PromptTokens:     30,
			CompletionTokens: 4,
		},
	}
	extractor := NewLLMExtractor(client, "openai-compatible", "fake-model")

	var sunk []model.UsageRecord
	extractor.AttachUsageSink(func(rec model.UsageRecord) error {
		sunk = append(sunk, rec)
		return nil
	})

	input := newInput("just a message")
	if _, err := extractor.Extract(context.Background(), input); err != nil {
		t.Fatalf("Extract() error = %v", err)
	}
	if len(sunk) != 1 {
		t.Fatalf("usage records = %d, want 1", len(sunk))
	}
	rec := sunk[0]
	if rec.Purpose != model.UsagePurposePersonaExtract {
		t.Fatalf("Purpose = %q, want %q", rec.Purpose, model.UsagePurposePersonaExtract)
	}
	if rec.Provider != "openai-compatible" || rec.Model != "fake-model" {
		t.Fatalf("provider/model = %q/%q", rec.Provider, rec.Model)
	}
	if rec.PromptTokens != 30 || rec.CompletionTokens != 4 {
		t.Fatalf("tokens = %d/%d, want 30/4", rec.PromptTokens, rec.CompletionTokens)
	}
	if rec.AgentID != "codex" || rec.SessionID != "lore-session-1" {
		t.Fatalf("agent/session = %q/%q", rec.AgentID, rec.SessionID)
	}
}

func TestLLMExtractorSkipsUsageWhenChatCompletionFails(t *testing.T) {
	client := &scriptedCompletionClient{err: errors.New("upstream timeout")}
	extractor := NewLLMExtractor(client, "test", "test-model")

	var sunk []model.UsageRecord
	extractor.AttachUsageSink(func(rec model.UsageRecord) error {
		sunk = append(sunk, rec)
		return nil
	})

	if _, err := extractor.Extract(context.Background(), newInput("hello")); err == nil {
		t.Fatal("Extract() error = nil, want upstream timeout")
	}
	if len(sunk) != 0 {
		t.Fatalf("usage emitted on failed ChatCompletion: %+v", sunk)
	}
}

func TestLLMExtractorDropsCandidateWithEmptyEvidence(t *testing.T) {
	client := &scriptedCompletionClient{
		resp: openai.ChatCompletionResponse{
			Content: `{
  "candidates": [
    {"field":"goal","proposed_value":"compete in CTF","evidence_quote":"","confidence":"high"}
  ]
}`,
		},
	}
	extractor := NewLLMExtractor(client, "test", "test-model")
	input := newInput("I want to compete in CTF this semester.")

	result, err := extractor.Extract(context.Background(), input)
	if err != nil {
		t.Fatalf("Extract() error = %v", err)
	}
	if len(result.Candidates) != 0 {
		t.Fatalf("expected discard of empty-evidence candidate; got %+v", result.Candidates)
	}
	if len(result.Warnings) == 0 || !strings.Contains(strings.Join(result.Warnings, " "), "missing evidence_quote") {
		t.Fatalf("warnings missing diagnostic; got %+v", result.Warnings)
	}
}

func TestNormalizeForSubstringCollapsesAndLowercases(t *testing.T) {
	got := normalizeForSubstring("  I  live  in  DALIAN \t now\n")
	want := "i live in dalian now"
	if got != want {
		t.Fatalf("normalizeForSubstring = %q, want %q", got, want)
	}
}

func TestExtractJSONObjectToleratesProviderOutputVariants(t *testing.T) {
	// extractJSONObject is the lenience layer that lets the extractor
	// survive provider output quirks: bare JSON, ```json / ``` code
	// fences, and a JSON object embedded in surrounding prose must all
	// yield the object; empty or object-free responses must error so the
	// caller loses the candidates (tokens already billed) rather than
	// decoding garbage. Only the bare-valid path was exercised via
	// Extract; the fence and prose branches were not.
	object := `{"candidates":[]}`
	okCases := []struct {
		name string
		in   string
		want string
	}{
		{"bare valid json", object, object},
		{"json code fence", "```json\n" + object + "\n```", object},
		{"bare code fence", "```\n" + object + "\n```", object},
		{"prose around object", "Sure, here you go:\n" + object + "\nHope that helps!", object},
		{"surrounding whitespace", "  \n" + object + "\t ", object},
	}
	for _, tc := range okCases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := extractJSONObject(tc.in)
			if err != nil {
				t.Fatalf("extractJSONObject(%q) error = %v, want nil", tc.in, err)
			}
			if got != tc.want {
				t.Fatalf("extractJSONObject(%q) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}

	errCases := []struct {
		name string
		in   string
	}{
		{"empty", ""},
		{"whitespace only", "   \n\t"},
		{"no json object", "I could not find anything worth recording."},
		{"braces but invalid json", "{ this is not: valid json }"},
	}
	for _, tc := range errCases {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := extractJSONObject(tc.in); err == nil {
				t.Fatalf("extractJSONObject(%q) error = nil, want an error", tc.in)
			}
		})
	}
}
