package operatoragent

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"sort"
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

type loopEnvelope struct {
	Type      string         `json:"type"`
	Tool      string         `json:"tool"`
	Arguments map[string]any `json:"arguments"`
	Message   string         `json:"message"`
}

type EnvConfig struct {
	BaseURL string
	APIKey  string
	Model   string
	Timeout time.Duration
}

type ModelCatalog struct {
	Models      []string
	Recommended string
}

var ErrUnavailable = errors.New("operator agent: model-backed operator agent is required")

const (
	maxLoopSteps                   = 8
	repeatedToolCallAbortThreshold = 3
)

func NewDefault() Agent {
	agent, err := NewFromEnv()
	if err != nil {
		return ErrorAgent{err: err}
	}
	if agent == nil {
		return NewUnavailable(nil)
	}
	return agent
}

func NewUnavailable(err error) Agent {
	if err == nil {
		err = fmt.Errorf("%w; configure LORE_LLM_BASE_URL and LORE_LLM_API_KEY (legacy OBSIDIAN_HARNESS_LLM_* also supported)", ErrUnavailable)
	}
	return ErrorAgent{err: err}
}

func NewFromEnv() (Agent, error) {
	cfg, enabled, err := LoadEnvConfig()
	if err != nil {
		return nil, err
	}
	if !enabled {
		return nil, nil
	}

	modelName, err := ResolveModel(context.Background(), cfg)
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
	return NewModelAgent(client), nil
}

func NewModelAgent(client completionClient) Agent {
	return ModelAgent{client: client}
}

func LoadEnvConfig() (EnvConfig, bool, error) {
	cfg := EnvConfig{
		BaseURL: firstNonEmptyEnv("LORE_OPERATOR_BASE_URL", "LORE_LLM_BASE_URL", "OBSIDIAN_HARNESS_OPERATOR_BASE_URL", "OBSIDIAN_HARNESS_LLM_BASE_URL"),
		APIKey:  firstNonEmptyEnv("LORE_OPERATOR_API_KEY", "LORE_LLM_API_KEY", "OBSIDIAN_HARNESS_OPERATOR_API_KEY", "OBSIDIAN_HARNESS_LLM_API_KEY"),
		Model:   firstNonEmptyEnv("LORE_OPERATOR_MODEL", "LORE_LLM_MODEL", "OBSIDIAN_HARNESS_OPERATOR_MODEL", "OBSIDIAN_HARNESS_LLM_MODEL"),
	}

	timeoutValue := firstNonEmptyEnv("LORE_OPERATOR_TIMEOUT", "LORE_LLM_TIMEOUT", "OBSIDIAN_HARNESS_OPERATOR_TIMEOUT", "OBSIDIAN_HARNESS_LLM_TIMEOUT")
	if strings.TrimSpace(timeoutValue) == "" {
		cfg.Timeout = 30 * time.Second
	} else {
		timeout, err := time.ParseDuration(strings.TrimSpace(timeoutValue))
		if err != nil {
			return EnvConfig{}, false, fmt.Errorf("operator agent: invalid timeout: %w", err)
		}
		cfg.Timeout = timeout
	}

	baseURL := strings.TrimSpace(cfg.BaseURL)
	apiKey := strings.TrimSpace(cfg.APIKey)
	modelName := strings.TrimSpace(cfg.Model)

	if baseURL == "" && apiKey == "" && modelName == "" {
		return EnvConfig{}, false, nil
	}
	if baseURL == "" || apiKey == "" {
		return EnvConfig{}, false, fmt.Errorf("operator agent: LORE_LLM_BASE_URL and LORE_LLM_API_KEY must be set together (legacy OBSIDIAN_HARNESS_LLM_* also supported)")
	}
	return cfg, true, nil
}

func ResolveModel(ctx context.Context, cfg EnvConfig) (string, error) {
	if modelName := strings.TrimSpace(cfg.Model); modelName != "" {
		return modelName, nil
	}

	catalog, err := DiscoverModels(ctx, cfg)
	if err != nil {
		return "", err
	}
	if catalog.Recommended == "" {
		return "", fmt.Errorf("operator agent: models were discovered but none matched the operator allowlist: %s", strings.Join(catalog.Models, ", "))
	}
	return catalog.Recommended, nil
}

func DiscoverModels(ctx context.Context, cfg EnvConfig) (ModelCatalog, error) {
	client, err := openai.NewClient(openai.Config{
		BaseURL: cfg.BaseURL,
		APIKey:  cfg.APIKey,
		Timeout: cfg.Timeout,
	})
	if err != nil {
		return ModelCatalog{}, err
	}

	discovered, err := client.ListModels(ctx)
	if err != nil {
		return ModelCatalog{}, fmt.Errorf("operator agent: model discovery failed: %w", err)
	}

	models := make([]string, 0, len(discovered))
	for _, item := range discovered {
		models = append(models, item.ID)
	}
	sort.Strings(models)

	recommended, err := selectOperatorModel(models)
	if err != nil {
		return ModelCatalog{Models: models}, nil
	}
	return ModelCatalog{
		Models:      models,
		Recommended: recommended,
	}, nil
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
	})
	if err != nil {
		return Decision{}, fmt.Errorf("operator agent: model request failed: %w", err)
	}

	return parseModelDecision(resp.Content, ctx)
}

func (a ModelAgent) Respond(input string, ctx Context, runtime ToolRuntime) (Response, error) {
	raw := strings.TrimSpace(input)
	if raw == "" {
		return Response{}, fmt.Errorf("empty input")
	}
	if asksForBackgroundRuntime(strings.ToLower(raw)) {
		return Response{}, fmt.Errorf("timed and background runtime tasks stay outside the operator console")
	}
	if runtime == nil {
		return Response{}, fmt.Errorf("operator agent: tool runtime is required")
	}

	tools := runtime.DescribeTools(ctx)
	messages := []openai.Message{
		{Role: "system", Content: loopSystemPrompt(tools, ctx)},
		{Role: "user", Content: buildLoopUserPrompt(raw, ctx)},
	}

	toolHistory := make([]string, 0, maxLoopSteps)
	trace := make([]ToolCallTrace, 0, maxLoopSteps)
	for step := 0; step < maxLoopSteps; step++ {
		resp, err := a.client.ChatCompletion(context.Background(), openai.ChatCompletionRequest{
			Messages:    messages,
			Temperature: 0,
		})
		if err != nil {
			return Response{}, fmt.Errorf("operator agent: model request failed: %w", err)
		}

		envelope, legacyDecision, err := parseLoopResponse(resp.Content, ctx)
		if err != nil {
			return Response{}, fmt.Errorf("operator agent: invalid loop response: %w", err)
		}
		if legacyDecision != nil {
			return Response{Decision: legacyDecision, Trace: append([]ToolCallTrace(nil), trace...)}, nil
		}

		switch envelope.Type {
		case "final":
			final := strings.TrimSpace(envelope.Message)
			if final == "" {
				return Response{}, fmt.Errorf("operator agent: final response is empty")
			}
			return Response{Final: final, Trace: append([]ToolCallTrace(nil), trace...)}, nil
		case "tool_call":
			toolName := strings.TrimSpace(envelope.Tool)
			if toolName == "" {
				return Response{}, fmt.Errorf("operator agent: tool_call.tool is required")
			}
			callSignature := toolCallSignature(toolName, envelope.Arguments)
			toolHistory = append(toolHistory, callSignature)
			if hasRepeatedToolLoop(toolHistory, callSignature, repeatedToolCallAbortThreshold) {
				return Response{}, fmt.Errorf("operator agent: repeated tool loop detected for %s", toolName)
			}
			if hasPingPongToolLoop(toolHistory) {
				return Response{}, fmt.Errorf("operator agent: alternating tool loop detected")
			}

			toolResult, toolErr := runtime.CallTool(toolName, envelope.Arguments)
			trace = append(trace, ToolCallTrace{
				Name:      toolName,
				Arguments: cloneToolArguments(envelope.Arguments),
				Status:    toolTraceStatus(toolErr),
				Error:     toolTraceError(toolErr),
			})
			toolContent := strings.TrimSpace(toolResult.Content)
			if toolErr != nil && toolContent != "" {
				toolErr = fmt.Errorf("%w\n%s", toolErr, toolContent)
			}
			messages = append(messages, openai.Message{Role: "assistant", Content: strings.TrimSpace(resp.Content)})
			messages = append(messages, openai.Message{
				Role:    "user",
				Content: buildToolResultPrompt(toolName, toolContent, toolErr),
			})
		default:
			return Response{}, fmt.Errorf("operator agent: unsupported response type %q", envelope.Type)
		}
	}

	return Response{}, fmt.Errorf("operator agent: exceeded max loop steps (%d)", maxLoopSteps)
}

func (a ErrorAgent) Decide(_ string, _ Context) (Decision, error) {
	return Decision{}, a.err
}

func (a ErrorAgent) Respond(_ string, _ Context, _ ToolRuntime) (Response, error) {
	return Response{}, a.err
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

func parseLoopResponse(content string, ctx Context) (loopEnvelope, *Decision, error) {
	jsonPayload, err := extractJSONObject(content)
	if err != nil {
		return loopEnvelope{}, nil, err
	}

	var raw map[string]json.RawMessage
	if err := json.Unmarshal([]byte(jsonPayload), &raw); err != nil {
		return loopEnvelope{}, nil, fmt.Errorf("decode loop response: %w", err)
	}
	if _, ok := raw["action"]; ok {
		decision, err := parseModelDecision(jsonPayload, ctx)
		if err != nil {
			return loopEnvelope{}, nil, err
		}
		return loopEnvelope{}, &decision, nil
	}

	var envelope loopEnvelope
	if err := json.Unmarshal([]byte(jsonPayload), &envelope); err != nil {
		return loopEnvelope{}, nil, fmt.Errorf("decode loop envelope: %w", err)
	}
	envelope.Type = strings.TrimSpace(envelope.Type)
	envelope.Tool = strings.TrimSpace(envelope.Tool)
	envelope.Message = strings.TrimSpace(envelope.Message)
	if envelope.Arguments == nil {
		envelope.Arguments = make(map[string]any)
	}
	switch envelope.Type {
	case "tool_call", "final":
		return envelope, nil, nil
	default:
		return loopEnvelope{}, nil, fmt.Errorf("unsupported type %q", envelope.Type)
	}
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
You are the operator agent for Lore.
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

func loopSystemPrompt(tools []ToolDefinition, ctx Context) string {
	var builder strings.Builder
	localExecMode := "disabled"
	if toolListContains(tools, "workspace_read") || toolListContains(tools, "workspace_list") {
		localExecMode = "enabled"
	}
	shellMode := "disabled"
	if toolListContains(tools, "shell_exec") {
		shellMode = "enabled"
	}
	builder.WriteString(strings.TrimSpace(`
You are the main Lore chat agent.
Return exactly one JSON object and nothing else.

You may respond in exactly one of two forms:
1. {"type":"tool_call","tool":"tool_name","arguments":{...}}
2. {"type":"final","message":"user-facing response"}

Rules:
- call at most one tool per response
- prefer Lore governance/read tools over workspace and shell tools
- workspace_* and shell_exec are only for explicit local file/code/run requests
- vault_write_low is only for explicit note/diary/journal write requests
- never use workspace_* or shell_exec on Lore-managed vault docs or runtime state files
- never schedule, poll, sync, import, attach, or run background jobs from this chat loop
- if a tool already returns a user-ready render, you may return it verbatim in final.message
- if a tool fails, either try a different tool or explain the failure in final.message
- if no tool is needed, answer directly with type=final
`))
	builder.WriteString("\n\nLore governance summary:\n")
	builder.WriteString("- managed core docs and plan/execution docs must stay in draft -> review -> apply\n")
	builder.WriteString("- low-governance vault notes may use vault_write_low only when the user explicitly wants a note, diary, or journal written\n")
	builder.WriteString("- process-sink docs are runtime-owned outputs, not direct chat writes\n")
	builder.WriteString("- local_exec_mode: ")
	builder.WriteString(localExecMode)
	builder.WriteString("\n- shell_exec_mode: ")
	builder.WriteString(shellMode)
	builder.WriteString("\n\nAvailable tools:\n")
	for _, tool := range tools {
		builder.WriteString("- ")
		builder.WriteString(tool.Name)
		builder.WriteString(": ")
		builder.WriteString(strings.TrimSpace(tool.Description))
		if args := strings.TrimSpace(tool.Arguments); args != "" {
			builder.WriteString(" | args: ")
			builder.WriteString(args)
		}
		builder.WriteString("\n")
	}
	builder.WriteString("\nSession context:\n")
	builder.WriteString("- current_draft_id: ")
	builder.WriteString(strings.TrimSpace(ctx.CurrentDraftID))
	builder.WriteString("\n- default_agent_id: ")
	builder.WriteString(withFallback(strings.TrimSpace(ctx.DefaultAgentID), "codex"))
	if !ctx.Now.IsZero() {
		builder.WriteString("\n- today: ")
		builder.WriteString(model.NormalizeDay(ctx.Now).Format("2006-01-02"))
	}
	return strings.TrimSpace(builder.String())
}

func toolListContains(tools []ToolDefinition, name string) bool {
	target := strings.TrimSpace(name)
	for _, tool := range tools {
		if strings.TrimSpace(tool.Name) == target {
			return true
		}
	}
	return false
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

func buildLoopUserPrompt(input string, ctx Context) string {
	var builder strings.Builder
	builder.WriteString("User request:\n")
	builder.WriteString(strings.TrimSpace(input))
	if history := renderHistory(ctx.History, 6); history != "" {
		builder.WriteString("\n\nRecent conversation:\n")
		builder.WriteString(history)
	}
	return builder.String()
}

func renderHistory(history []ConversationTurn, limit int) string {
	if len(history) == 0 || limit <= 0 {
		return ""
	}
	start := 0
	if len(history) > limit {
		start = len(history) - limit
	}
	lines := make([]string, 0, len(history)-start)
	for _, turn := range history[start:] {
		role := strings.TrimSpace(turn.Role)
		if role == "" {
			role = "unknown"
		}
		lines = append(lines, fmt.Sprintf("- %s: %s", role, oneLine(turn.Content, 220)))
	}
	return strings.Join(lines, "\n")
}

func buildToolResultPrompt(toolName string, content string, toolErr error) string {
	if toolErr != nil {
		return fmt.Sprintf("Tool result for %s:\nERROR: %s", toolName, toolErr.Error())
	}
	if strings.TrimSpace(content) == "" {
		content = "(empty result)"
	}
	return fmt.Sprintf("Tool result for %s:\n%s", toolName, content)
}

func toolTraceStatus(err error) string {
	if err != nil {
		return "error"
	}
	return "ok"
}

func toolTraceError(err error) string {
	if err == nil {
		return ""
	}
	return strings.TrimSpace(err.Error())
}

func cloneToolArguments(arguments map[string]any) map[string]any {
	if len(arguments) == 0 {
		return map[string]any{}
	}
	cloned := make(map[string]any, len(arguments))
	for key, value := range arguments {
		cloned[key] = value
	}
	return cloned
}

func toolCallSignature(toolName string, arguments map[string]any) string {
	data, err := json.Marshal(arguments)
	if err != nil {
		return toolName
	}
	return toolName + ":" + string(data)
}

func hasRepeatedToolLoop(history []string, next string, threshold int) bool {
	if threshold <= 1 {
		return true
	}
	if len(history) < threshold {
		return false
	}
	for i := len(history) - threshold; i < len(history); i++ {
		if history[i] != next {
			return false
		}
	}
	return true
}

func hasPingPongToolLoop(history []string) bool {
	if len(history) < 6 {
		return false
	}
	last := history[len(history)-6:]
	return last[0] == last[2] &&
		last[2] == last[4] &&
		last[1] == last[3] &&
		last[3] == last[5] &&
		last[0] != last[1]
}

func oneLine(value string, limit int) string {
	value = strings.TrimSpace(strings.ReplaceAll(strings.ReplaceAll(value, "\r\n", "\n"), "\n", " "))
	if limit > 0 && len(value) > limit {
		return value[:limit] + "..."
	}
	return value
}

func firstNonEmptyEnv(names ...string) string {
	for _, name := range names {
		if value := strings.TrimSpace(os.Getenv(name)); value != "" {
			return value
		}
	}
	return ""
}

func selectOperatorModel(models []string) (string, error) {
	if len(models) == 0 {
		return "", fmt.Errorf("no models available")
	}

	for _, preferred := range []string{"gpt-5.4", "gpt-5.3", "gpt-5.2", "gpt-5.1", "gpt-5", "gpt-4.1"} {
		for _, modelName := range models {
			if strings.EqualFold(strings.TrimSpace(modelName), preferred) {
				return modelName, nil
			}
		}
	}

	candidates := make([]string, 0, len(models))
	for _, modelName := range models {
		if isLikelyOperatorModel(modelName) {
			candidates = append(candidates, modelName)
		}
	}
	if len(candidates) == 0 {
		return "", fmt.Errorf("no likely text model found")
	}
	sort.Strings(candidates)
	return candidates[0], nil
}

func isLikelyOperatorModel(modelName string) bool {
	value := strings.ToLower(strings.TrimSpace(modelName))
	if value == "" {
		return false
	}
	for _, blocked := range []string{"embedding", "moderation", "whisper", "tts", "audio", "realtime", "image", "vision-preview", "transcribe", "transcription", "search"} {
		if strings.Contains(value, blocked) {
			return false
		}
	}
	for _, allowedPrefix := range []string{"gpt-", "o1", "o3", "o4", "claude", "gemini", "qwen", "kimi", "glm", "deepseek"} {
		if strings.HasPrefix(value, allowedPrefix) {
			return true
		}
	}
	return strings.Contains(value, "instruct")
}
