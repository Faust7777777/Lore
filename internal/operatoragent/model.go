package operatoragent

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"

	openai "obsidian-harness/internal/llm/openai"
	"obsidian-harness/internal/model"
)

type completionClient interface {
	ChatCompletion(ctx context.Context, req openai.ChatCompletionRequest) (openai.ChatCompletionResponse, error)
}

type ModelAgent struct {
	client completionClient
}

type ErrorAgent struct {
	err error
}

type modelDecision struct {
	Action          string `json:"action"`
	DraftID         string `json:"draft_id"`
	UseFocusedDraft bool   `json:"use_focused_draft"`
	PendingOnly     bool   `json:"pending_only"`
	AgentID         string `json:"agent_id"`
	Day             string `json:"day"`
}

type EnvConfig struct {
	BaseURL string
	APIKey  string
	Model   string
	Timeout time.Duration
}

func NewDefault() Agent {
	agent, err := NewFromEnv()
	if err != nil {
		return ErrorAgent{err: err}
	}
	if agent == nil {
		return NewFallback()
	}
	return agent
}

func NewFromEnv() (Agent, error) {
	cfg, enabled, err := LoadEnvConfig()
	if err != nil {
		return nil, err
	}
	if !enabled {
		return nil, nil
	}

	client, err := openai.NewClient(openai.Config{
		BaseURL: cfg.BaseURL,
		APIKey:  cfg.APIKey,
		Model:   cfg.Model,
		Timeout: cfg.Timeout,
	})
	if err != nil {
		return nil, err
	}
	return NewModelAgent(client), nil
}

func NewModelAgent(client completionClient) Agent {
	return ModelAgent{client: client}
}

func LoadEnvConfig() (EnvConfig, bool, error) {
	cfg := EnvConfig{
		BaseURL: firstNonEmptyEnv("OBSIDIAN_HARNESS_OPERATOR_BASE_URL", "OBSIDIAN_HARNESS_LLM_BASE_URL", "LORE_OPERATOR_BASE_URL", "LORE_LLM_BASE_URL"),
		APIKey:  firstNonEmptyEnv("OBSIDIAN_HARNESS_OPERATOR_API_KEY", "OBSIDIAN_HARNESS_LLM_API_KEY", "LORE_OPERATOR_API_KEY", "LORE_LLM_API_KEY"),
		Model:   firstNonEmptyEnv("OBSIDIAN_HARNESS_OPERATOR_MODEL", "OBSIDIAN_HARNESS_LLM_MODEL", "LORE_OPERATOR_MODEL", "LORE_LLM_MODEL"),
	}

	timeoutValue := firstNonEmptyEnv("OBSIDIAN_HARNESS_OPERATOR_TIMEOUT", "OBSIDIAN_HARNESS_LLM_TIMEOUT", "LORE_OPERATOR_TIMEOUT", "LORE_LLM_TIMEOUT")
	if strings.TrimSpace(timeoutValue) == "" {
		cfg.Timeout = 30 * time.Second
	} else {
		timeout, err := time.ParseDuration(strings.TrimSpace(timeoutValue))
		if err != nil {
			return EnvConfig{}, false, fmt.Errorf("operator agent: invalid timeout: %w", err)
		}
		cfg.Timeout = timeout
	}

	values := []string{strings.TrimSpace(cfg.BaseURL), strings.TrimSpace(cfg.APIKey), strings.TrimSpace(cfg.Model)}
	nonEmpty := 0
	for _, value := range values {
		if value != "" {
			nonEmpty++
		}
	}
	if nonEmpty == 0 {
		return EnvConfig{}, false, nil
	}
	if nonEmpty != len(values) {
		return EnvConfig{}, false, fmt.Errorf("operator agent: OBSIDIAN_HARNESS_LLM_BASE_URL, OBSIDIAN_HARNESS_LLM_API_KEY, and OBSIDIAN_HARNESS_LLM_MODEL must be set together")
	}
	return cfg, true, nil
}

func (a ModelAgent) Decide(input string, ctx Context) (Decision, error) {
	raw := strings.TrimSpace(input)
	if raw == "" {
		return Decision{}, fmt.Errorf("empty input")
	}
	if asksForBackgroundRuntime(strings.ToLower(raw)) {
		return Decision{}, fmt.Errorf("timed and background runtime tasks stay outside the operator console")
	}

	resp, err := a.client.ChatCompletion(context.Background(), openai.ChatCompletionRequest{
		Messages: []openai.Message{
			{Role: "system", Content: systemPrompt()},
			{Role: "user", Content: buildUserPrompt(raw, ctx)},
		},
		Temperature: 0,
		MaxTokens:   220,
	})
	if err != nil {
		return Decision{}, fmt.Errorf("operator agent: model request failed: %w", err)
	}

	return parseModelDecision(resp.Content, ctx)
}

func (a ErrorAgent) Decide(_ string, _ Context) (Decision, error) {
	return Decision{}, a.err
}

func parseModelDecision(content string, ctx Context) (Decision, error) {
	jsonPayload, err := extractJSONObject(content)
	if err != nil {
		return Decision{}, fmt.Errorf("operator agent: invalid model response: %w", err)
	}

	var parsed modelDecision
	if err := json.Unmarshal([]byte(jsonPayload), &parsed); err != nil {
		return Decision{}, fmt.Errorf("operator agent: decode model decision: %w", err)
	}

	action := Action(strings.TrimSpace(parsed.Action))
	switch action {
	case ActionHelp, ActionShowStatus, ActionListDrafts, ActionReviewDraft, ActionApproveDraft, ActionRejectDraft, ActionRequestDraftRevision, ActionApplyDraft, ActionShowProcessSinkDay:
	default:
		return Decision{}, fmt.Errorf("operator agent: unsupported action %q", parsed.Action)
	}

	day := resolveDay(parsed.Day, ctx.Now)
	if strings.TrimSpace(parsed.Day) != "" {
		parsedDay, err := time.ParseInLocation("2006-01-02", strings.TrimSpace(parsed.Day), time.Local)
		if err != nil {
			return Decision{}, fmt.Errorf("operator agent: invalid day %q", parsed.Day)
		}
		day = model.NormalizeDay(parsedDay)
	}

	return Decision{
		Action:          action,
		DraftID:         strings.TrimSpace(parsed.DraftID),
		UseFocusedDraft: parsed.UseFocusedDraft,
		PendingOnly:     parsed.PendingOnly,
		AgentID:         withFallback(strings.TrimSpace(parsed.AgentID), withFallback(strings.TrimSpace(ctx.DefaultAgentID), "codex")),
		Day:             day,
	}, nil
}

func extractJSONObject(content string) (string, error) {
	trimmed := strings.TrimSpace(content)
	if trimmed == "" {
		return "", fmt.Errorf("empty content")
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
	return "", fmt.Errorf("no valid json object found")
}

func systemPrompt() string {
	return strings.TrimSpace(`
You are the operator agent for Obsidian Harness.
Return exactly one JSON object and nothing else.

Allowed actions:
- help
- show_status
- list_drafts
- review_draft
- approve_draft
- reject_draft
- request_draft_revision
- apply_draft
- show_process_sink_day

Rules:
- choose exactly one action
- never invent new actions
- never schedule, poll, sync, import, attach, or run background jobs
- never attempt direct runtime or tool execution; only choose one operator action
- prefer review_draft over list_drafts when the user clearly wants to inspect a draft
- if the user refers to the current/focused draft, set use_focused_draft=true
- if the request is unsupported, return {"action":"help"}

Return schema:
{
  "action": "help|show_status|list_drafts|review_draft|approve_draft|reject_draft|request_draft_revision|apply_draft|show_process_sink_day",
  "draft_id": "",
  "use_focused_draft": false,
  "pending_only": false,
  "agent_id": "codex",
  "day": "YYYY-MM-DD"
}
`)
}

func buildUserPrompt(input string, ctx Context) string {
	today := time.Now()
	if !ctx.Now.IsZero() {
		today = ctx.Now
	}

	return fmt.Sprintf(
		"Operator request:\n%s\n\nSession context:\n- current_draft_id: %s\n- default_agent_id: %s\n- today: %s\n",
		strings.TrimSpace(input),
		strings.TrimSpace(ctx.CurrentDraftID),
		withFallback(strings.TrimSpace(ctx.DefaultAgentID), "codex"),
		model.NormalizeDay(today).Format("2006-01-02"),
	)
}

func firstNonEmptyEnv(names ...string) string {
	for _, name := range names {
		if value := strings.TrimSpace(os.Getenv(name)); value != "" {
			return value
		}
	}
	return ""
}
