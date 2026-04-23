package app

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"obsidian-harness/internal/adapter/codexjsonl"
	openai "obsidian-harness/internal/llm/openai"
	"obsidian-harness/internal/model"
	"obsidian-harness/internal/operatoragent"
)

type ProcessSinkSummarizer interface {
	SummarizeCheckpoint(window codexjsonl.WindowSummary) (string, string, error)
	SummarizeDaily(agentID string, day time.Time, checkpoints []model.CheckpointDoc) (string, string, error)
}

type processSinkChatClient interface {
	ChatCompletion(ctx context.Context, req openai.ChatCompletionRequest) (openai.ChatCompletionResponse, error)
}

type modelProcessSinkSummarizer struct {
	client processSinkChatClient
}

type processSinkSummaryPayload struct {
	Title   string `json:"title"`
	Content string `json:"content"`
}

func defaultProcessSinkSummarizer() (ProcessSinkSummarizer, error) {
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
	return &modelProcessSinkSummarizer{client: client}, nil
}

func (s *modelProcessSinkSummarizer) SummarizeCheckpoint(window codexjsonl.WindowSummary) (string, string, error) {
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
	return s.runSummaryPrompt(checkpointSummarySystemPrompt(), prompt)
}

func (s *modelProcessSinkSummarizer) SummarizeDaily(agentID string, day time.Time, checkpoints []model.CheckpointDoc) (string, string, error) {
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
	return s.runSummaryPrompt(dailySummarySystemPrompt(), prompt)
}

func (s *modelProcessSinkSummarizer) runSummaryPrompt(system string, user string) (string, string, error) {
	resp, err := s.client.ChatCompletion(context.Background(), openai.ChatCompletionRequest{
		Messages: []openai.Message{
			{Role: "system", Content: system},
			{Role: "user", Content: user},
		},
		Temperature: 0,
		MaxTokens:   700,
	})
	if err != nil {
		return "", "", err
	}

	payload, err := parseProcessSinkSummary(resp.Content)
	if err != nil {
		return "", "", err
	}
	return strings.TrimSpace(payload.Title), strings.TrimSpace(payload.Content), nil
}

func checkpointSummarySystemPrompt() string {
	return strings.TrimSpace(`
You summarize one external coding-agent checkpoint window for Obsidian Harness.
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
You summarize one day of external coding-agent checkpoints for Obsidian Harness.
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
