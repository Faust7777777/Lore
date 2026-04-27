package operatoragent

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"

	openai "obsidian-harness/internal/llm/openai"
)

type fakeCompletionClient struct {
	response  openai.ChatCompletionResponse
	responses []openai.ChatCompletionResponse
	err       error
	requests  []openai.ChatCompletionRequest
}

func (f *fakeCompletionClient) ChatCompletion(_ context.Context, req openai.ChatCompletionRequest) (openai.ChatCompletionResponse, error) {
	f.requests = append(f.requests, req)
	if len(f.responses) > 0 {
		resp := f.responses[0]
		f.responses = f.responses[1:]
		return resp, f.err
	}
	return f.response, f.err
}

type fakeToolRuntime struct {
	tools     []ToolDefinition
	results   map[string]ToolResult
	calls     []string
	arguments []map[string]any
	callErr   error
	callFunc  func(name string, arguments map[string]any) (ToolResult, error)
}

func (f *fakeToolRuntime) DescribeTools(_ Context) []ToolDefinition {
	return append([]ToolDefinition(nil), f.tools...)
}

func (f *fakeToolRuntime) CallTool(name string, arguments map[string]any) (ToolResult, error) {
	f.calls = append(f.calls, name)
	f.arguments = append(f.arguments, arguments)
	if f.callFunc != nil {
		return f.callFunc(name, arguments)
	}
	if f.callErr != nil {
		return ToolResult{}, f.callErr
	}
	if result, ok := f.results[name]; ok {
		return result, nil
	}
	return ToolResult{}, nil
}

func clearOperatorEnv(t *testing.T) {
	t.Helper()
	for _, key := range []string{
		"OBSIDIAN_HARNESS_LLM_BASE_URL",
		"OBSIDIAN_HARNESS_LLM_API_KEY",
		"OBSIDIAN_HARNESS_LLM_MODEL",
		"OBSIDIAN_HARNESS_OPERATOR_BASE_URL",
		"OBSIDIAN_HARNESS_OPERATOR_API_KEY",
		"OBSIDIAN_HARNESS_OPERATOR_MODEL",
		"OBSIDIAN_HARNESS_LLM_TIMEOUT",
		"OBSIDIAN_HARNESS_OPERATOR_TIMEOUT",
		"LORE_LLM_BASE_URL",
		"LORE_LLM_API_KEY",
		"LORE_LLM_MODEL",
		"LORE_OPERATOR_BASE_URL",
		"LORE_OPERATOR_API_KEY",
		"LORE_OPERATOR_MODEL",
		"LORE_LLM_TIMEOUT",
		"LORE_OPERATOR_TIMEOUT",
	} {
		t.Setenv(key, "")
	}
}

func mustJSONToolResult(t *testing.T, value any) ToolResult {
	t.Helper()
	data, err := json.Marshal(value)
	if err != nil {
		t.Fatalf("json.Marshal() error = %v", err)
	}
	return ToolResult{Content: string(data)}
}

func TestLoadEnvConfigDisabledWhenUnset(t *testing.T) {
	clearOperatorEnv(t)

	cfg, enabled, err := LoadEnvConfig()
	if err != nil {
		t.Fatalf("LoadEnvConfig() error = %v", err)
	}
	if enabled {
		t.Fatalf("enabled = true, want false, cfg = %+v", cfg)
	}
}

func TestLoadEnvConfigAllowsDiscoveryWithoutModel(t *testing.T) {
	clearOperatorEnv(t)
	t.Setenv("OBSIDIAN_HARNESS_LLM_BASE_URL", "https://example.test/v1")
	t.Setenv("OBSIDIAN_HARNESS_LLM_API_KEY", "secret")
	t.Setenv("OBSIDIAN_HARNESS_LLM_MODEL", "")

	cfg, enabled, err := LoadEnvConfig()
	if err != nil {
		t.Fatalf("LoadEnvConfig() error = %v", err)
	}
	if !enabled {
		t.Fatal("enabled = false, want true")
	}
	if cfg.BaseURL != "https://example.test/v1" || cfg.APIKey != "secret" || cfg.Model != "" {
		t.Fatalf("cfg = %+v, want base+key and empty model", cfg)
	}
}

func TestLoadEnvConfigRequiresBaseAndKeyTogether(t *testing.T) {
	clearOperatorEnv(t)
	t.Setenv("OBSIDIAN_HARNESS_LLM_BASE_URL", "https://example.test/v1")
	t.Setenv("OBSIDIAN_HARNESS_LLM_API_KEY", "")
	t.Setenv("OBSIDIAN_HARNESS_LLM_MODEL", "")

	_, _, err := LoadEnvConfig()
	if err == nil {
		t.Fatal("LoadEnvConfig() error = nil, want incomplete base/key error")
	}
	if !strings.Contains(err.Error(), "must be set together") {
		t.Fatalf("error = %q, want must be set together", err)
	}
}

func TestLoadEnvConfigPrefersLoreEnvNames(t *testing.T) {
	clearOperatorEnv(t)
	t.Setenv("OBSIDIAN_HARNESS_LLM_BASE_URL", "https://legacy.test/v1")
	t.Setenv("OBSIDIAN_HARNESS_LLM_API_KEY", "legacy-secret")
	t.Setenv("LORE_LLM_BASE_URL", "https://lore.test/v1")
	t.Setenv("LORE_LLM_API_KEY", "lore-secret")
	t.Setenv("LORE_LLM_MODEL", "gpt-5.4")

	cfg, enabled, err := LoadEnvConfig()
	if err != nil {
		t.Fatalf("LoadEnvConfig() error = %v", err)
	}
	if !enabled {
		t.Fatal("enabled = false, want true")
	}
	if cfg.BaseURL != "https://lore.test/v1" || cfg.APIKey != "lore-secret" || cfg.Model != "gpt-5.4" {
		t.Fatalf("cfg = %+v, want Lore-preferring env config", cfg)
	}
}

func TestNewDefaultReturnsUnavailableAgentWhenConfigMissing(t *testing.T) {
	clearOperatorEnv(t)

	_, err := NewDefault().Decide("show status", Context{})
	if err == nil {
		t.Fatal("Decide() error = nil, want unavailable agent error")
	}
	if !strings.Contains(err.Error(), "model-backed operator agent is required") {
		t.Fatalf("error = %q, want unavailable agent guidance", err)
	}
}

func TestModelAgentDecideParsesSingleJSONDecision(t *testing.T) {
	client := &fakeCompletionClient{
		response: openai.ChatCompletionResponse{
			Content: "```json\n{\"action\":\"approve_draft\",\"use_focused_draft\":true}\n```",
		},
	}
	agent := NewModelAgent(client)

	decision, err := agent.Decide("approve it", Context{
		CurrentDraftID: "draft-123",
		DefaultAgentID: "codex",
		Now:            time.Date(2026, 4, 22, 11, 0, 0, 0, time.Local),
	})
	if err != nil {
		t.Fatalf("Decide() error = %v", err)
	}
	if decision.Action != ActionApproveDraft {
		t.Fatalf("decision.Action = %q, want %q", decision.Action, ActionApproveDraft)
	}
	if !decision.UseFocusedDraft {
		t.Fatal("decision.UseFocusedDraft = false, want true")
	}
	if decision.AgentID != "codex" {
		t.Fatalf("decision.AgentID = %q, want codex", decision.AgentID)
	}
	if len(client.requests) != 1 || len(client.requests[0].Messages) != 2 {
		t.Fatalf("requests = %+v, want one chat completion with system+user", client.requests)
	}
}

func TestModelAgentDecideRejectsBackgroundTaskRequests(t *testing.T) {
	client := &fakeCompletionClient{}
	agent := NewModelAgent(client)

	_, err := agent.Decide("sync codex session every 30 minutes", Context{})
	if err == nil {
		t.Fatal("Decide() error = nil, want background task rejection")
	}
	if !strings.Contains(err.Error(), "timed and background") {
		t.Fatalf("error = %q, want background rejection", err)
	}
	if len(client.requests) != 0 {
		t.Fatalf("requests = %d, want 0", len(client.requests))
	}
}

func TestModelAgentDecideErrorsOnInvalidModelOutput(t *testing.T) {
	client := &fakeCompletionClient{
		response: openai.ChatCompletionResponse{
			Content: "status please",
		},
	}
	agent := NewModelAgent(client)

	_, err := agent.Decide("show status", Context{})
	if err == nil {
		t.Fatal("Decide() error = nil, want invalid model output error")
	}
	if !strings.Contains(err.Error(), "invalid model response") {
		t.Fatalf("error = %q, want invalid model response", err)
	}
}

func TestModelAgentRespondRunsToolLoopThenFinal(t *testing.T) {
	client := &fakeCompletionClient{
		responses: []openai.ChatCompletionResponse{
			{Content: `{"type":"tool_call","tool":"managed_status","arguments":{}}`},
			{Content: `{"type":"final","message":"Managed Status\n--------------\nready"}`},
		},
	}
	agent := NewModelAgent(client).(ModelAgent)
	runtime := &fakeToolRuntime{
		tools: []ToolDefinition{{Name: "managed_status", Description: "show status"}},
		results: map[string]ToolResult{
			"managed_status": {Content: "Managed Status\n--------------\nready"},
		},
	}

	response, err := agent.Respond("show current status", Context{
		DefaultAgentID: "codex",
		Now:            time.Date(2026, 4, 22, 11, 0, 0, 0, time.Local),
	}, runtime)
	if err != nil {
		t.Fatalf("Respond() error = %v", err)
	}
	if strings.TrimSpace(response.Final) != "Managed Status\n--------------\nready" {
		t.Fatalf("response.Final = %q", response.Final)
	}
	if len(response.Trace) != 1 || response.Trace[0].Name != "managed_status" || response.Trace[0].Status != "ok" {
		t.Fatalf("response.Trace = %+v, want one managed_status ok trace", response.Trace)
	}
	if len(runtime.calls) != 1 || runtime.calls[0] != "managed_status" {
		t.Fatalf("tool calls = %+v, want managed_status", runtime.calls)
	}
	if len(client.requests) != 2 {
		t.Fatalf("requests = %d, want 2", len(client.requests))
	}
}

func TestModelAgentRespondRunsNativeToolCallThenFinal(t *testing.T) {
	client := &fakeCompletionClient{
		responses: []openai.ChatCompletionResponse{
			{ToolCalls: []openai.ToolCall{{Name: "managed_status", Arguments: map[string]any{}}}},
			{Content: `{"type":"final","message":"Managed Status\n--------------\nready"}`},
		},
	}
	agent := NewModelAgent(client).(ModelAgent)
	runtime := &fakeToolRuntime{
		tools: []ToolDefinition{{Name: "managed_status", Description: "show status"}},
		results: map[string]ToolResult{
			"managed_status": {Content: "Managed Status\n--------------\nready"},
		},
	}

	response, err := agent.Respond("show current status", Context{
		DefaultAgentID: "codex",
		Now:            time.Date(2026, 4, 22, 11, 0, 0, 0, time.Local),
	}, runtime)
	if err != nil {
		t.Fatalf("Respond() error = %v", err)
	}
	if strings.TrimSpace(response.Final) != "Managed Status\n--------------\nready" {
		t.Fatalf("response.Final = %q", response.Final)
	}
	if len(response.Trace) != 1 || response.Trace[0].Name != "managed_status" || response.Trace[0].Status != "ok" {
		t.Fatalf("response.Trace = %+v, want one managed_status ok trace", response.Trace)
	}
	if len(runtime.calls) != 1 || runtime.calls[0] != "managed_status" {
		t.Fatalf("tool calls = %+v, want managed_status", runtime.calls)
	}
	if len(client.requests) != 2 {
		t.Fatalf("requests = %d, want 2", len(client.requests))
	}
	if len(client.requests[0].Tools) != 1 || client.requests[0].Tools[0].Name != "managed_status" {
		t.Fatalf("request tools = %+v, want one managed_status tool schema", client.requests[0].Tools)
	}
	secondMessages := client.requests[1].Messages
	if len(secondMessages) != 4 {
		t.Fatalf("second request messages = %+v, want 4 messages", secondMessages)
	}
	if secondMessages[2].Role != "assistant" || !strings.Contains(secondMessages[2].Content, `"type":"tool_call"`) || !strings.Contains(secondMessages[2].Content, `"tool":"managed_status"`) {
		t.Fatalf("synthetic tool call message = %+v", secondMessages[2])
	}
	if secondMessages[3].Role != "user" || !strings.Contains(secondMessages[3].Content, "Tool result for managed_status:") || !strings.Contains(secondMessages[3].Content, "Managed Status") {
		t.Fatalf("tool result message = %+v", secondMessages[3])
	}
}

func TestModelAgentRespondIncludesWorkingSetInUserPrompt(t *testing.T) {
	client := &fakeCompletionClient{
		response: openai.ChatCompletionResponse{Content: `{"type":"final","message":"ok"}`},
	}
	agent := NewModelAgent(client).(ModelAgent)
	runtime := &fakeToolRuntime{tools: []ToolDefinition{{Name: "vault_read", Description: "read note", Arguments: `{"path":"relative/path.md"}`}}}

	_, err := agent.Respond("\u8bfb\u53d6", Context{
		DefaultAgentID: "codex",
		WorkingSet: []WorkingSetItem{{
			Kind:   "vault_path",
			Path:   "03-\u753b\u50cf/\u4eba\u7269\u753b\u50cf.md",
			Source: "vault_list",
		}},
	}, runtime)
	if err != nil {
		t.Fatalf("Respond() error = %v", err)
	}
	if len(client.requests) != 1 || len(client.requests[0].Messages) < 2 {
		t.Fatalf("requests = %+v, want one request with user prompt", client.requests)
	}
	userPrompt := client.requests[0].Messages[1].Content
	for _, want := range []string{
		"Current working set:",
		"03-\u753b\u50cf/\u4eba\u7269\u753b\u50cf.md",
		"resolve it against this working set",
	} {
		if !strings.Contains(userPrompt, want) {
			t.Fatalf("user prompt missing %q:\n%s", want, userPrompt)
		}
	}
}
func TestModelAgentRespondRejectsMultipleNativeToolCalls(t *testing.T) {
	client := &fakeCompletionClient{
		response: openai.ChatCompletionResponse{
			ToolCalls: []openai.ToolCall{
				{Name: "managed_status", Arguments: map[string]any{}},
				{Name: "draft_list", Arguments: map[string]any{"pending_only": true}},
			},
		},
	}
	agent := NewModelAgent(client).(ModelAgent)
	runtime := &fakeToolRuntime{
		tools: []ToolDefinition{
			{Name: "managed_status", Description: "show status"},
			{Name: "draft_list", Description: "list drafts", Arguments: `{"pending_only":true}`},
		},
	}

	_, err := agent.Respond("show status and drafts", Context{DefaultAgentID: "codex"}, runtime)
	if err == nil {
		t.Fatal("Respond() error = nil, want multiple tool call error")
	}
	if !strings.Contains(err.Error(), "Lore supports one tool call per loop step") {
		t.Fatalf("error = %q, want one-tool-call guidance", err)
	}
	if len(runtime.calls) != 0 {
		t.Fatalf("runtime.calls = %+v, want no executed tool calls", runtime.calls)
	}
}

func TestModelAgentRespondUsesLeadingJSONObjectWhenProviderConcatenatesObjects(t *testing.T) {
	client := &fakeCompletionClient{
		responses: []openai.ChatCompletionResponse{
			{Content: `{"type":"tool_call","tool":"managed_status","arguments":{}}{"type":"final","message":"ignore this trailing object"}`},
			{Content: `{"type":"final","message":"Managed Status\n--------------\nready"}`},
		},
	}
	agent := NewModelAgent(client).(ModelAgent)
	runtime := &fakeToolRuntime{
		tools: []ToolDefinition{{Name: "managed_status", Description: "show status"}},
		results: map[string]ToolResult{
			"managed_status": {Content: "Managed Status\n--------------\nready"},
		},
	}

	response, err := agent.Respond("show current status", Context{
		DefaultAgentID: "codex",
		Now:            time.Date(2026, 4, 22, 11, 0, 0, 0, time.Local),
	}, runtime)
	if err != nil {
		t.Fatalf("Respond() error = %v", err)
	}
	if strings.TrimSpace(response.Final) != "Managed Status\n--------------\nready" {
		t.Fatalf("response.Final = %q", response.Final)
	}
	if len(runtime.calls) != 1 || runtime.calls[0] != "managed_status" {
		t.Fatalf("tool calls = %+v, want managed_status", runtime.calls)
	}
}

func TestModelAgentRespondPromptIncludesGovernanceSummaryAndModes(t *testing.T) {
	client := &fakeCompletionClient{
		response: openai.ChatCompletionResponse{
			Content: `{"type":"final","message":"ok"}`,
		},
	}
	agent := NewModelAgent(client).(ModelAgent)
	runtime := &fakeToolRuntime{
		tools: []ToolDefinition{
			{Name: "managed_status", Description: "show status"},
			{Name: "vault_write_low", Description: "write note"},
			{Name: "workspace_read", Description: "read workspace file"},
			{Name: "shell_exec", Description: "run shell command"},
		},
	}

	_, err := agent.Respond("write a diary and inspect local code", Context{DefaultAgentID: "codex"}, runtime)
	if err != nil {
		t.Fatalf("Respond() error = %v", err)
	}
	if len(client.requests) != 1 || len(client.requests[0].Messages) == 0 {
		t.Fatalf("requests = %+v, want one request with system prompt", client.requests)
	}
	systemPrompt := client.requests[0].Messages[0].Content
	for _, want := range []string{
		"Lore governance summary",
		"low-governance vault notes may use vault_write_low",
		"shell_exec returns a confirmation prompt first; do not assume the command already ran",
		"local_exec_mode: enabled",
		"shell_exec_mode: enabled",
	} {
		if !strings.Contains(systemPrompt, want) {
			t.Fatalf("system prompt missing %q:\n%s", want, systemPrompt)
		}
	}
}

func TestModelAgentRespondPromptListsGitToolsWhenShellModeIsDisabled(t *testing.T) {
	client := &fakeCompletionClient{
		response: openai.ChatCompletionResponse{
			Content: `{"type":"final","message":"ok"}`,
		},
	}
	agent := NewModelAgent(client).(ModelAgent)
	runtime := &fakeToolRuntime{
		tools: []ToolDefinition{
			{Name: "managed_status", Description: "show status"},
			{Name: "workspace_read", Description: "read workspace file"},
			{Name: "git_status", Description: "show git status for the local repo"},
			{Name: "git_diff_summary", Description: "summarize git diff for the local repo"},
		},
	}

	_, err := agent.Respond("inspect git status in the local repo", Context{DefaultAgentID: "codex"}, runtime)
	if err != nil {
		t.Fatalf("Respond() error = %v", err)
	}
	if len(client.requests) != 1 || len(client.requests[0].Messages) == 0 {
		t.Fatalf("requests = %+v, want one request with system prompt", client.requests)
	}
	systemPrompt := client.requests[0].Messages[0].Content
	for _, want := range []string{
		"use workspace_* for local workspace files outside Lore-managed vault/state",
		"prefer git_* over shell_exec for repository inspection",
		"local_exec_mode: enabled",
		"shell_exec_mode: disabled",
		"Available tools (use exact names; keep calls minimal):",
		"managed_status, workspace_read, git_status, git_diff_summary",
	} {
		if !strings.Contains(systemPrompt, want) {
			t.Fatalf("system prompt missing %q:\n%s", want, systemPrompt)
		}
	}
}

func TestModelAgentRespondPromptIncludesRuntimeAgentDocs(t *testing.T) {
	client := &fakeCompletionClient{
		response: openai.ChatCompletionResponse{
			Content: `{"type":"final","message":"ok"}`,
		},
	}
	agent := NewModelAgent(client).(ModelAgent)
	runtime := &fakeToolRuntime{
		tools: []ToolDefinition{
			{Name: "system_doc_get", Description: "read one managed core doc"},
		},
		callFunc: func(name string, arguments map[string]any) (ToolResult, error) {
			if name != "system_doc_get" {
				return ToolResult{}, fmt.Errorf("unexpected tool: %s", name)
			}
			switch arguments["name"] {
			case "agent":
				return mustJSONToolResult(t, map[string]any{
					"path":      "agent.md",
					"doc_class": "agent_doc",
					"content":   "# Lore Agent Instructions\n\n- Stay concise.\n- Respect governance.",
				}), nil
			case "identity":
				return mustJSONToolResult(t, map[string]any{
					"path":      "identity.md",
					"doc_class": "identity_doc",
					"content":   "# Lore Identity\n\n- Practical\n- Audit-aware",
				}), nil
			default:
				return ToolResult{}, fmt.Errorf("unexpected system doc request: %+v", arguments)
			}
		},
	}

	_, err := agent.Respond("who are you in this workspace?", Context{DefaultAgentID: "codex"}, runtime)
	if err != nil {
		t.Fatalf("Respond() error = %v", err)
	}
	if len(client.requests) != 1 || len(client.requests[0].Messages) == 0 {
		t.Fatalf("requests = %+v, want one request with system prompt", client.requests)
	}
	systemPrompt := client.requests[0].Messages[0].Content
	for _, want := range []string{
		"Workspace agent docs (supplemental; runtime hard rules above still win):",
		"agent.md",
		"# Lore Agent Instructions",
		"identity.md",
		"# Lore Identity",
	} {
		if !strings.Contains(systemPrompt, want) {
			t.Fatalf("system prompt missing %q:\n%s", want, systemPrompt)
		}
	}
	if len(runtime.calls) != 2 || runtime.calls[0] != "system_doc_get" || runtime.calls[1] != "system_doc_get" {
		t.Fatalf("runtime calls = %+v, want two system_doc_get preloads", runtime.calls)
	}
}

func TestModelAgentRespondReturnsShellConfirmationImmediately(t *testing.T) {
	client := &fakeCompletionClient{
		response: openai.ChatCompletionResponse{
			Content: `{"type":"tool_call","tool":"shell_exec","arguments":{"command":"go test ./...","timeout_seconds":30}}`,
		},
	}
	agent := NewModelAgent(client).(ModelAgent)
	runtime := &fakeToolRuntime{
		tools: []ToolDefinition{
			{Name: "shell_exec", Description: "request one shell command with confirmation"},
		},
		results: map[string]ToolResult{
			"shell_exec": {
				Content: "Shell command pending confirmation.\ncommand: go test ./...\ntimeout_seconds: 30\nreply with yes/confirm to run it, or no/cancel to skip it.",
			},
		},
	}

	response, err := agent.Respond("run go test", Context{DefaultAgentID: "codex"}, runtime)
	if err != nil {
		t.Fatalf("Respond() error = %v", err)
	}
	if !strings.Contains(response.Final, "Shell command pending confirmation.") {
		t.Fatalf("response.Final = %q, want shell confirmation prompt", response.Final)
	}
	if len(response.Trace) != 1 || response.Trace[0].Name != "shell_exec" || response.Trace[0].Status != "pending" {
		t.Fatalf("response.Trace = %+v, want one pending shell_exec trace", response.Trace)
	}
	if len(runtime.calls) != 1 || runtime.calls[0] != "shell_exec" {
		t.Fatalf("runtime.calls = %+v, want one shell_exec call", runtime.calls)
	}
	if len(client.requests) != 1 {
		t.Fatalf("requests = %d, want 1 because shell confirmation should end the loop immediately", len(client.requests))
	}
}

func TestModelAgentRespondFallsBackToLegacyDecisionJSON(t *testing.T) {
	client := &fakeCompletionClient{
		response: openai.ChatCompletionResponse{
			Content: `{"action":"show_status"}`,
		},
	}
	agent := NewModelAgent(client).(ModelAgent)
	runtime := &fakeToolRuntime{}

	response, err := agent.Respond("show current status", Context{DefaultAgentID: "codex"}, runtime)
	if err != nil {
		t.Fatalf("Respond() error = %v", err)
	}
	if response.Decision == nil || response.Decision.Action != ActionShowStatus {
		t.Fatalf("response.Decision = %+v, want show_status", response.Decision)
	}
}

func TestModelAgentRespondDetectsRepeatedToolLoop(t *testing.T) {
	client := &fakeCompletionClient{
		responses: []openai.ChatCompletionResponse{
			{Content: `{"type":"tool_call","tool":"managed_status","arguments":{}}`},
			{Content: `{"type":"tool_call","tool":"managed_status","arguments":{}}`},
			{Content: `{"type":"tool_call","tool":"managed_status","arguments":{}}`},
		},
	}
	agent := NewModelAgent(client).(ModelAgent)
	runtime := &fakeToolRuntime{
		tools: []ToolDefinition{{Name: "managed_status", Description: "show status"}},
		results: map[string]ToolResult{
			"managed_status": {Content: "ok"},
		},
	}

	_, err := agent.Respond("show current status", Context{DefaultAgentID: "codex"}, runtime)
	if err == nil {
		t.Fatal("Respond() error = nil, want repeated tool loop error")
	}
	if !strings.Contains(err.Error(), "repeated tool loop") {
		t.Fatalf("error = %q, want repeated tool loop", err)
	}
}

func TestModelAgentRespondDetectsAlternatingToolLoop(t *testing.T) {
	client := &fakeCompletionClient{
		responses: []openai.ChatCompletionResponse{
			{Content: `{"type":"tool_call","tool":"managed_status","arguments":{}}`},
			{Content: `{"type":"tool_call","tool":"draft_list","arguments":{}}`},
			{Content: `{"type":"tool_call","tool":"managed_status","arguments":{}}`},
			{Content: `{"type":"tool_call","tool":"draft_list","arguments":{}}`},
			{Content: `{"type":"tool_call","tool":"managed_status","arguments":{}}`},
			{Content: `{"type":"tool_call","tool":"draft_list","arguments":{}}`},
		},
	}
	agent := NewModelAgent(client).(ModelAgent)
	runtime := &fakeToolRuntime{
		tools: []ToolDefinition{
			{Name: "managed_status", Description: "show status"},
			{Name: "draft_list", Description: "list drafts"},
		},
		results: map[string]ToolResult{
			"managed_status": {Content: "ok"},
			"draft_list":     {Content: "ok"},
		},
	}

	_, err := agent.Respond("keep checking", Context{DefaultAgentID: "codex"}, runtime)
	if err == nil {
		t.Fatal("Respond() error = nil, want alternating tool loop error")
	}
	if !strings.Contains(err.Error(), "alternating tool loop") {
		t.Fatalf("error = %q, want alternating tool loop", err)
	}
}

func TestSelectOperatorModelPrefersGpt54(t *testing.T) {
	modelName, err := selectOperatorModel([]string{"gpt-4.1", "gpt-5.4", "gpt-4o"})
	if err != nil {
		t.Fatalf("selectOperatorModel() error = %v", err)
	}
	if modelName != "gpt-5.4" {
		t.Fatalf("modelName = %q, want gpt-5.4", modelName)
	}
}

func TestOpenAIToolDefinitionsExposeVaultResolveDirSchema(t *testing.T) {
	definitions := openAIToolDefinitions([]ToolDefinition{{
		Name:        "vault_resolve",
		Description: "Resolve a natural-language note reference to vault markdown paths. Returns status unique, ambiguous, or not_found; read selected_path only when status is unique.",
		Arguments:   `{"query":"note title or reference","dir":"","limit":5}`,
	}})
	if len(definitions) != 1 {
		t.Fatalf("definitions = %+v, want one tool", definitions)
	}
	if !strings.Contains(definitions[0].Description, "read selected_path only when status is unique") {
		t.Fatalf("description = %q, want selected_path guidance", definitions[0].Description)
	}
	properties, ok := definitions[0].Parameters["properties"].(map[string]any)
	if !ok {
		t.Fatalf("parameters = %#v, want object schema", definitions[0].Parameters)
	}
	if _, ok := properties["query"].(map[string]any); !ok {
		t.Fatalf("query schema missing: %#v", properties)
	}
	if dir, ok := properties["dir"].(map[string]any); !ok || dir["type"] != "string" {
		t.Fatalf("dir schema = %#v, want string", properties["dir"])
	}
	if limit, ok := properties["limit"].(map[string]any); !ok || limit["type"] != "number" {
		t.Fatalf("limit schema = %#v, want number", properties["limit"])
	}
}

func TestSelectOperatorModelFallsBackToCompatibleTextModel(t *testing.T) {
	modelName, err := selectOperatorModel([]string{"text-embedding-3-large", "claude-sonnet-4"})
	if err != nil {
		t.Fatalf("selectOperatorModel() error = %v", err)
	}
	if modelName != "claude-sonnet-4" {
		t.Fatalf("modelName = %q, want claude-sonnet-4", modelName)
	}
}

func TestSelectOperatorModelRejectsNonTextCatalog(t *testing.T) {
	_, err := selectOperatorModel([]string{"text-embedding-3-large", "omni-moderation-latest"})
	if err == nil {
		t.Fatal("selectOperatorModel() error = nil, want no likely text model error")
	}
	if !strings.Contains(err.Error(), "no likely text model") {
		t.Fatalf("error = %q, want no likely text model", err)
	}
}

func TestOpenAIToolDefinitionsInfersParametersFromArgumentExamples(t *testing.T) {
	definitions := openAIToolDefinitions([]ToolDefinition{{
		Name:        "context_pack",
		Description: "assemble context",
		Arguments:   `{"target_path":"optional/path.md","task":"what you need","limit":6}`,
	}})
	if len(definitions) != 1 {
		t.Fatalf("definitions = %+v, want one tool", definitions)
	}
	if definitions[0].Description != "assemble context" {
		t.Fatalf("definitions[0].Description = %q, want original tool description", definitions[0].Description)
	}
	if definitions[0].Strict {
		t.Fatalf("definitions[0].Strict = true, want operator tools to use provider-compatible strict=false")
	}
	properties, ok := definitions[0].Parameters["properties"].(map[string]any)
	if !ok {
		t.Fatalf("parameters = %#v, want object schema", definitions[0].Parameters)
	}
	limit, ok := properties["limit"].(map[string]any)
	if !ok || limit["type"] != "number" {
		t.Fatalf("limit schema = %#v, want number", properties["limit"])
	}
	task, ok := properties["task"].(map[string]any)
	if !ok || task["type"] != "string" {
		t.Fatalf("task schema = %#v, want string", properties["task"])
	}
}
