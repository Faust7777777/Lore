package app

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"obsidian-harness/internal/adapter/codexjsonl"
	"obsidian-harness/internal/config"
	openai "obsidian-harness/internal/llm/openai"
	"obsidian-harness/internal/model"
)

type ProcessSinkSummarizer interface {
	SummarizeCheckpoint(window codexjsonl.WindowSummary) (string, string, error)
	SummarizeDaily(agentID string, day time.Time, checkpoints []model.CheckpointDoc) (string, string, error)
	// Context variants thread a caller's context.Context to the model
	// call so a long-running summarization (e.g. a large `lore import`)
	// can be cancelled mid-flight. The non-context methods above delegate
	// to these with context.Background() for callers that do not yet
	// thread one.
	SummarizeCheckpointContext(ctx context.Context, window codexjsonl.WindowSummary) (string, string, error)
	SummarizeDailyContext(ctx context.Context, agentID string, day time.Time, checkpoints []model.CheckpointDoc) (string, string, error)
}

type processSinkChatClient interface {
	ChatCompletion(ctx context.Context, req openai.ChatCompletionRequest) (openai.ChatCompletionResponse, error)
}

type modelProcessSinkSummarizer struct {
	client   processSinkChatClient
	provider string
	model    string
	// usageSink, when non-nil, receives one model.UsageRecord per
	// successful ChatCompletion. Wired in by OpenRuntime so that
	// daemon/import paths persist process-sink LLM cost without the
	// public ProcessSinkSummarizer interface having to know about it.
	// Sink failures are deliberately swallowed: usage accounting must
	// not break summarization on the daemon path. See attachUsageSink.
	usageSink func(model.UsageRecord) error
}

type processSinkSummaryPayload struct {
	Title   string `json:"title"`
	Content string `json:"content"`
}

func defaultProcessSinkSummarizer(cfg config.ResolvedLLMConfig, resolveErr error) (ProcessSinkSummarizer, error) {
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
	return &modelProcessSinkSummarizer{
		client:   client,
		provider: llmProvider(cfg),
		model:    modelName,
	}, nil
}

// attachUsageSink injects a usage sink into a process-sink summarizer
// if and only if the underlying implementation is the model-backed one.
// Fakes used in tests are no-ops, which keeps the broader test surface
// unchanged. This is the seam OpenRuntime uses to wire summarizer cost
// records into runtime.RecordUsage without widening the public
// ProcessSinkSummarizer interface.
func attachUsageSink(summarizer ProcessSinkSummarizer, sink func(model.UsageRecord) error) {
	if s, ok := summarizer.(*modelProcessSinkSummarizer); ok {
		s.usageSink = sink
	}
}

func (s *modelProcessSinkSummarizer) SummarizeCheckpoint(window codexjsonl.WindowSummary) (string, string, error) {
	return s.SummarizeCheckpointContext(context.Background(), window)
}

func (s *modelProcessSinkSummarizer) SummarizeCheckpointContext(ctx context.Context, window codexjsonl.WindowSummary) (string, string, error) {
	if strings.TrimSpace(window.RawTranscript) == "" {
		return "", "", nil
	}

	prompt := fmt.Sprintf(
		"Summarize this external agent checkpoint window.\n\nWindow:\n- agent: %s\n- session: %s\n- start: %s\n- end: %s\n- event_count: %d\n\nStructured extract:\n%s\n\nRaw transcript:\n%s\n",
		window.Window.AgentID,
		window.Window.SessionID,
		window.Window.WindowStart.Format(time.RFC3339),
		window.Window.WindowEnd.Format(time.RFC3339),
		window.EventCount,
		truncateForSummary(window.Content, 2400),
		truncateForSummary(window.RawTranscript, 6000),
	)
	return s.runSummaryPrompt(ctx, checkpointSummarySystemPrompt(), prompt, window.Window.AgentID, window.Window.SessionID)
}

func (s *modelProcessSinkSummarizer) SummarizeDaily(agentID string, day time.Time, checkpoints []model.CheckpointDoc) (string, string, error) {
	return s.SummarizeDailyContext(context.Background(), agentID, day, checkpoints)
}

func (s *modelProcessSinkSummarizer) SummarizeDailyContext(ctx context.Context, agentID string, day time.Time, checkpoints []model.CheckpointDoc) (string, string, error) {
	if len(checkpoints) == 0 {
		return "", "", nil
	}

	var builder strings.Builder
	for _, checkpoint := range checkpoints {
		builder.WriteString(fmt.Sprintf(
			"- %s-%s | %s | %s\n%s\n\n",
			checkpoint.Window.WindowStart.Format("15:04"),
			checkpoint.Window.WindowEnd.Format("15:04"),
			checkpoint.State,
			checkpoint.Title,
			truncateForSummary(checkpoint.Content, 1200),
		))
	}

	prompt := fmt.Sprintf(
		"Summarize this agent day into a daily report.\n\nDay:\n- agent: %s\n- day: %s\n- checkpoints: %d\n\nCheckpoint materials:\n%s",
		agentID,
		model.NormalizeDay(day).Format("2006-01-02"),
		len(checkpoints),
		builder.String(),
	)
	// Daily summaries have no natural session identifier; leave SessionID empty
	// rather than synthesizing one.
	return s.runSummaryPrompt(ctx, dailySummarySystemPrompt(), prompt, agentID, "")
}

func (s *modelProcessSinkSummarizer) runSummaryPrompt(ctx context.Context, system string, user string, agentID string, sessionID string) (string, string, error) {
	startedAt := time.Now().UTC()
	resp, err := s.client.ChatCompletion(ctx, openai.ChatCompletionRequest{
		Messages: []openai.Message{
			{Role: "system", Content: system},
			{Role: "user", Content: user},
		},
		Temperature: 0,
	})
	if err != nil {
		return "", "", err
	}

	// Emit usage before JSON parse so that a malformed-summary failure
	// does not lose the cost record for an already-billed model call.
	// Sink errors are intentionally swallowed: usage accounting must
	// not bubble up and break daemon/import summarization.
	if s.usageSink != nil {
		_ = s.usageSink(model.UsageRecord{
			Provider:         s.provider,
			Model:            s.model,
			AgentID:          agentID,
			SessionID:        sessionID,
			PromptTokens:     resp.PromptTokens,
			CompletionTokens: resp.CompletionTokens,
			RecordedAt:       startedAt,
			Purpose:          model.UsagePurposeProcessSink,
		})
	}

	payload, err := parseProcessSinkSummary(resp.Content)
	if err != nil {
		return "", "", err
	}
	return strings.TrimSpace(payload.Title), strings.TrimSpace(payload.Content), nil
}

func checkpointSummarySystemPrompt() string {
	return strings.TrimSpace(`
You summarize one external coding-agent checkpoint window for Lore.
Return exactly one JSON object and nothing else.

Schema:
{
  "title": "short checkpoint title",
  "content": "markdown summary"
}

Rules:
- if the window has no meaningful transcript content, return {"title":"","content":""}
- title should be concise and specific
- content should be markdown with short sections or bullets
- capture completed work, decisions, blockers, and next actions when present
- do not invent facts not present in the materials
`)
}

func dailySummarySystemPrompt() string {
	return strings.TrimSpace(`
You summarize one day of external coding-agent checkpoints for Lore.
Return exactly one JSON object and nothing else.

Schema:
{
  "title": "daily report title",
  "content": "markdown daily summary"
}

Rules:
- content should summarize the day across all checkpoints
- include what progressed, open issues, and immediate next steps if present
- mention empty/placeholder windows only when they affect the day's continuity
- do not invent facts not present in the provided checkpoint materials
`)
}

func parseProcessSinkSummary(content string) (processSinkSummaryPayload, error) {
	jsonPayload, err := extractJSONObject(content)
	if err != nil {
		return processSinkSummaryPayload{}, err
	}

	var payload processSinkSummaryPayload
	if err := json.Unmarshal([]byte(jsonPayload), &payload); err != nil {
		return processSinkSummaryPayload{}, fmt.Errorf("process sink summarizer: decode response: %w", err)
	}
	return payload, nil
}

func extractJSONObject(content string) (string, error) {
	trimmed := strings.TrimSpace(content)
	if trimmed == "" {
		return "", fmt.Errorf("process sink summarizer: empty response")
	}
	if strings.HasPrefix(trimmed, "```") {
		trimmed = strings.TrimPrefix(trimmed, "```json")
		trimmed = strings.TrimPrefix(trimmed, "```")
		trimmed = strings.TrimSuffix(trimmed, "```")
		trimmed = strings.TrimSpace(trimmed)
	}
	if json.Valid([]byte(trimmed)) {
		return trimmed, nil
	}

	start := strings.Index(trimmed, "{")
	end := strings.LastIndex(trimmed, "}")
	if start >= 0 && end > start {
		candidate := strings.TrimSpace(trimmed[start : end+1])
		if json.Valid([]byte(candidate)) {
			return candidate, nil
		}
	}
	return "", fmt.Errorf("process sink summarizer: no valid json object found")
}

func truncateForSummary(value string, limit int) string {
	value = strings.TrimSpace(value)
	if limit <= 0 || len(value) <= limit {
		return value
	}
	return value[:limit] + "..."
}
