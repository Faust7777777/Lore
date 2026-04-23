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
		"vault_write_low is only for explicit note/diary/journal write requests",
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
		"workspace_* and shell_exec are only for explicit local file/code/run requests",
		"local_exec_mode: enabled",
		"shell_exec_mode: disabled",
		"- git_status: show git status for the local repo",
		"- git_diff_summary: summarize git diff for the local repo",
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
