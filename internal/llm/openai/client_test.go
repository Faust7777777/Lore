package openai

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestChatCompletion(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/chat/completions" {
			t.Fatalf("path = %q, want /chat/completions", r.URL.Path)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer secret" {
			t.Fatalf("authorization = %q, want bearer token", got)
		}

		var payload map[string]any
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Fatalf("Decode() error = %v", err)
		}
		if payload["model"] != "gpt-test" {
			t.Fatalf("model = %#v, want gpt-test", payload["model"])
		}
		if _, ok := payload["max_tokens"]; ok {
			t.Fatalf("payload unexpectedly included max_tokens: %#v", payload["max_tokens"])
		}

		_ = json.NewEncoder(w).Encode(map[string]any{
			"choices": []map[string]any{{
				"message": map[string]any{
					"content": `{"action":"show_status"}`,
				},
			}},
			"usage": map[string]any{
				"prompt_tokens":     12,
				"completion_tokens": 5,
			},
		})
	}))
	defer server.Close()

	client, err := NewClient(Config{
		BaseURL: server.URL,
		APIKey:  "secret",
		Model:   "gpt-test",
		Timeout: time.Second,
	})
	if err != nil {
		t.Fatalf("NewClient() error = %v", err)
	}

	resp, err := client.ChatCompletion(context.Background(), ChatCompletionRequest{
		Messages: []Message{{Role: "user", Content: "hello"}},
	})
	if err != nil {
		t.Fatalf("ChatCompletion() error = %v", err)
	}
	if resp.Content != `{"action":"show_status"}` {
		t.Fatalf("resp.Content = %q", resp.Content)
	}
	if resp.PromptTokens != 12 || resp.CompletionTokens != 5 {
		t.Fatalf("usage = %+v, want prompt=12 completion=5", resp)
	}
}

func TestChatCompletionUsesResponsesAPIForGPT5(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/responses" {
			t.Fatalf("path = %q, want /responses", r.URL.Path)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer secret" {
			t.Fatalf("authorization = %q, want bearer token", got)
		}

		var payload map[string]any
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Fatalf("Decode() error = %v", err)
		}
		if payload["model"] != "gpt-5.4" {
			t.Fatalf("model = %#v, want gpt-5.4", payload["model"])
		}
		if payload["instructions"] != "be concise" {
			t.Fatalf("instructions = %#v, want be concise", payload["instructions"])
		}
		input, ok := payload["input"].([]any)
		if !ok || len(input) != 1 {
			t.Fatalf("input = %#v, want single message", payload["input"])
		}

		_ = json.NewEncoder(w).Encode(map[string]any{
			"output": []map[string]any{{
				"type": "message",
				"role": "assistant",
				"content": []map[string]any{{
					"type": "output_text",
					"text": `{"action":"show_status"}`,
				}},
			}},
			"usage": map[string]any{
				"input_tokens":  21,
				"output_tokens": 7,
			},
		})
	}))
	defer server.Close()

	client, err := NewClient(Config{
		BaseURL: server.URL,
		APIKey:  "secret",
		Model:   "gpt-5.4",
		Timeout: time.Second,
	})
	if err != nil {
		t.Fatalf("NewClient() error = %v", err)
	}

	resp, err := client.ChatCompletion(context.Background(), ChatCompletionRequest{
		Messages: []Message{
			{Role: "system", Content: "be concise"},
			{Role: "user", Content: "hello"},
		},
	})
	if err != nil {
		t.Fatalf("ChatCompletion() error = %v", err)
	}
	if resp.Content != `{"action":"show_status"}` {
		t.Fatalf("resp.Content = %q", resp.Content)
	}
	if resp.PromptTokens != 21 || resp.CompletionTokens != 7 {
		t.Fatalf("usage = %+v, want prompt=21 completion=7", resp)
	}
}

func TestChatCompletionUsesResponsesAPITools(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/responses" {
			t.Fatalf("path = %q, want /responses", r.URL.Path)
		}

		var payload map[string]any
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Fatalf("Decode() error = %v", err)
		}
		tools, ok := payload["tools"].([]any)
		if !ok || len(tools) != 1 {
			t.Fatalf("tools = %#v, want one tool", payload["tools"])
		}
		tool, ok := tools[0].(map[string]any)
		if !ok {
			t.Fatalf("tool = %#v, want object", tools[0])
		}
		if tool["type"] != "function" || tool["name"] != "managed_status" {
			t.Fatalf("tool = %#v, want managed_status function", tool)
		}
		parameters, ok := tool["parameters"].(map[string]any)
		if !ok {
			t.Fatalf("parameters = %#v, want object", tool["parameters"])
		}
		if parameters["type"] != "object" {
			t.Fatalf("parameters.type = %#v, want object", parameters["type"])
		}
		if payload["parallel_tool_calls"] != false {
			t.Fatalf("parallel_tool_calls = %#v, want false", payload["parallel_tool_calls"])
		}

		_ = json.NewEncoder(w).Encode(map[string]any{
			"output": []map[string]any{{
				"type":      "function_call",
				"id":        "fc_123",
				"call_id":   "call_123",
				"name":      "managed_status",
				"arguments": `{}`,
			}},
			"usage": map[string]any{
				"input_tokens":  14,
				"output_tokens": 2,
			},
		})
	}))
	defer server.Close()

	client, err := NewClient(Config{
		BaseURL: server.URL,
		APIKey:  "secret",
		Model:   "gpt-5.4",
		Timeout: time.Second,
	})
	if err != nil {
		t.Fatalf("NewClient() error = %v", err)
	}

	resp, err := client.ChatCompletion(context.Background(), ChatCompletionRequest{
		Messages: []Message{
			{Role: "system", Content: "be concise"},
			{Role: "user", Content: "hello"},
		},
		Tools: []ToolDefinition{{
			Name:        "managed_status",
			Description: "show status",
			Parameters: map[string]any{
				"type":                 "object",
				"properties":           map[string]any{},
				"additionalProperties": false,
			},
			Strict: true,
		}},
	})
	if err != nil {
		t.Fatalf("ChatCompletion() error = %v", err)
	}
	if len(resp.ToolCalls) != 1 {
		t.Fatalf("resp.ToolCalls = %+v, want one function call", resp.ToolCalls)
	}
	if resp.ToolCalls[0].Name != "managed_status" {
		t.Fatalf("resp.ToolCalls[0].Name = %q, want managed_status", resp.ToolCalls[0].Name)
	}
}

func TestChatCompletionLegacyPathTools(t *testing.T) {
	// review-v1 P1-7: a non-Responses model must still send tools on the
	// legacy /chat/completions path and parse tool_calls back, including the
	// content-less (content: null) tool-call turn that previously errored.
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/chat/completions" {
			t.Fatalf("path = %q, want /chat/completions", r.URL.Path)
		}
		var payload map[string]any
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Fatalf("Decode() error = %v", err)
		}
		// Chat tools nest name/description/parameters under "function".
		tools, ok := payload["tools"].([]any)
		if !ok || len(tools) != 1 {
			t.Fatalf("tools = %#v, want one tool", payload["tools"])
		}
		tool := tools[0].(map[string]any)
		if tool["type"] != "function" {
			t.Fatalf("tool.type = %#v, want function", tool["type"])
		}
		fn, ok := tool["function"].(map[string]any)
		if !ok || fn["name"] != "vault_read" {
			t.Fatalf("tool.function = %#v, want vault_read", tool["function"])
		}
		if payload["tool_choice"] != "auto" {
			t.Fatalf("tool_choice = %#v, want auto", payload["tool_choice"])
		}

		_ = json.NewEncoder(w).Encode(map[string]any{
			"choices": []map[string]any{{
				"message": map[string]any{
					"content": nil,
					"tool_calls": []map[string]any{{
						"id":   "call_9",
						"type": "function",
						"function": map[string]any{
							"name":      "vault_read",
							"arguments": `{"path":"a.md"}`,
						},
					}},
				},
			}},
			"usage": map[string]any{"prompt_tokens": 7, "completion_tokens": 3},
		})
	}))
	defer server.Close()

	client, err := NewClient(Config{
		BaseURL: server.URL,
		APIKey:  "secret",
		Model:   "deepseek-v4", // non-gpt-5.4 -> legacy /chat/completions path
		Timeout: time.Second,
	})
	if err != nil {
		t.Fatalf("NewClient() error = %v", err)
	}

	resp, err := client.ChatCompletion(context.Background(), ChatCompletionRequest{
		Messages: []Message{{Role: "user", Content: "read a.md"}},
		Tools:    []ToolDefinition{{Name: "vault_read", Description: "read a note", Parameters: map[string]any{"type": "object"}}},
	})
	if err != nil {
		t.Fatalf("ChatCompletion() error = %v (content-less tool call must not error)", err)
	}
	if len(resp.ToolCalls) != 1 {
		t.Fatalf("resp.ToolCalls = %+v, want one tool call", resp.ToolCalls)
	}
	call := resp.ToolCalls[0]
	if call.Name != "vault_read" {
		t.Fatalf("tool call name = %q, want vault_read", call.Name)
	}
	if call.ID != "call_9" || call.CallID != "call_9" {
		t.Fatalf("tool call id/callid = %q/%q, want call_9", call.ID, call.CallID)
	}
	if got := call.Arguments["path"]; got != "a.md" {
		t.Fatalf("tool call arguments[path] = %#v, want a.md", got)
	}
}

func TestChatCompletionUsesResponsesAPIPreservesAssistantHistory(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/responses" {
			t.Fatalf("path = %q, want /responses", r.URL.Path)
		}

		var payload map[string]any
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Fatalf("Decode() error = %v", err)
		}
		input, ok := payload["input"].([]any)
		if !ok || len(input) != 3 {
			t.Fatalf("input = %#v, want 3-message history", payload["input"])
		}
		gotRoles := make([]string, 0, len(input))
		for _, item := range input {
			record, ok := item.(map[string]any)
			if !ok {
				t.Fatalf("input item = %#v, want object", item)
			}
			gotRoles = append(gotRoles, record["role"].(string))
		}
		if strings.Join(gotRoles, ",") != "user,assistant,user" {
			t.Fatalf("roles = %v, want [user assistant user]", gotRoles)
		}

		_ = json.NewEncoder(w).Encode(map[string]any{
			"output_text": `{"action":"show_status"}`,
			"usage": map[string]any{
				"input_tokens":  13,
				"output_tokens": 4,
			},
		})
	}))
	defer server.Close()

	client, err := NewClient(Config{
		BaseURL: server.URL,
		APIKey:  "secret",
		Model:   "gpt-5.4",
		Timeout: time.Second,
	})
	if err != nil {
		t.Fatalf("NewClient() error = %v", err)
	}

	_, err = client.ChatCompletion(context.Background(), ChatCompletionRequest{
		Messages: []Message{
			{Role: "system", Content: "be concise"},
			{Role: "user", Content: "hello"},
			{Role: "assistant", Content: "previous answer"},
			{Role: "user", Content: "follow up"},
		},
	})
	if err != nil {
		t.Fatalf("ChatCompletion() error = %v", err)
	}
}

func TestChatCompletionReturnsAPIError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadGateway)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"error": map[string]any{
				"message": "upstream unavailable",
			},
		})
	}))
	defer server.Close()

	client, err := NewClient(Config{
		BaseURL: server.URL,
		APIKey:  "secret",
		Model:   "gpt-test",
	})
	if err != nil {
		t.Fatalf("NewClient() error = %v", err)
	}

	_, err = client.ChatCompletion(context.Background(), ChatCompletionRequest{
		Messages: []Message{{Role: "user", Content: "hello"}},
	})
	if err == nil {
		t.Fatal("ChatCompletion() error = nil, want upstream error")
	}
	if !strings.Contains(err.Error(), "upstream unavailable") {
		t.Fatalf("error = %q, want upstream unavailable", err)
	}
}

func TestChatCompletionRetriesTransientGatewayError(t *testing.T) {
	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		if requests == 1 {
			w.WriteHeader(http.StatusBadGateway)
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"choices": []map[string]any{{
				"message": map[string]any{
					"content": `{"action":"show_status"}`,
				},
			}},
			"usage": map[string]any{
				"prompt_tokens":     8,
				"completion_tokens": 4,
			},
		})
	}))
	defer server.Close()

	client, err := NewClient(Config{
		BaseURL: server.URL,
		APIKey:  "secret",
		Model:   "gpt-test",
		Timeout: time.Second,
	})
	if err != nil {
		t.Fatalf("NewClient() error = %v", err)
	}

	resp, err := client.ChatCompletion(context.Background(), ChatCompletionRequest{
		Messages: []Message{{Role: "user", Content: "hello"}},
	})
	if err != nil {
		t.Fatalf("ChatCompletion() error = %v", err)
	}
	if requests != 2 {
		t.Fatalf("request count = %d, want 2", requests)
	}
	if resp.Content != `{"action":"show_status"}` {
		t.Fatalf("resp.Content = %q", resp.Content)
	}
}

func TestChatCompletionRetriesRateLimitForResponsesAPI(t *testing.T) {
	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		if r.URL.Path != "/responses" {
			t.Fatalf("path = %q, want /responses", r.URL.Path)
		}
		if requests == 1 {
			w.WriteHeader(http.StatusTooManyRequests)
			_ = json.NewEncoder(w).Encode(map[string]any{
				"error": map[string]any{
					"message": "rate limited",
				},
			})
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"output": []map[string]any{{
				"type": "message",
				"role": "assistant",
				"content": []map[string]any{{
					"type": "output_text",
					"text": `{"action":"show_status"}`,
				}},
			}},
			"usage": map[string]any{
				"input_tokens":  11,
				"output_tokens": 3,
			},
		})
	}))
	defer server.Close()

	client, err := NewClient(Config{
		BaseURL: server.URL,
		APIKey:  "secret",
		Model:   "gpt-5.4",
		Timeout: time.Second,
	})
	if err != nil {
		t.Fatalf("NewClient() error = %v", err)
	}

	resp, err := client.ChatCompletion(context.Background(), ChatCompletionRequest{
		Messages: []Message{{Role: "user", Content: "hello"}},
	})
	if err != nil {
		t.Fatalf("ChatCompletion() error = %v", err)
	}
	if requests != 2 {
		t.Fatalf("request count = %d, want 2", requests)
	}
	if resp.Content != `{"action":"show_status"}` {
		t.Fatalf("resp.Content = %q", resp.Content)
	}
}

func TestChatCompletionRetriesTransportEOF(t *testing.T) {
	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		if requests == 1 {
			hijacker, ok := w.(http.Hijacker)
			if !ok {
				t.Fatal("response writer does not support hijacking")
			}
			conn, _, err := hijacker.Hijack()
			if err != nil {
				t.Fatalf("Hijack() error = %v", err)
			}
			_ = conn.Close()
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"choices": []map[string]any{{
				"message": map[string]any{
					"content": `{"action":"show_status"}`,
				},
			}},
			"usage": map[string]any{
				"prompt_tokens":     9,
				"completion_tokens": 4,
			},
		})
	}))
	defer server.Close()

	client, err := NewClient(Config{
		BaseURL: server.URL,
		APIKey:  "secret",
		Model:   "gpt-test",
		Timeout: time.Second,
	})
	if err != nil {
		t.Fatalf("NewClient() error = %v", err)
	}

	resp, err := client.ChatCompletion(context.Background(), ChatCompletionRequest{
		Messages: []Message{{Role: "user", Content: "hello"}},
	})
	if err != nil {
		t.Fatalf("ChatCompletion() error = %v", err)
	}
	if requests != 2 {
		t.Fatalf("request count = %d, want 2", requests)
	}
	if resp.Content != `{"action":"show_status"}` {
		t.Fatalf("resp.Content = %q", resp.Content)
	}
}

func TestChatCompletionRequiresModel(t *testing.T) {
	client, err := NewClient(Config{
		BaseURL: "https://example.test/v1",
		APIKey:  "secret",
	})
	if err != nil {
		t.Fatalf("NewClient() error = %v", err)
	}

	_, err = client.ChatCompletion(context.Background(), ChatCompletionRequest{
		Messages: []Message{{Role: "user", Content: "hello"}},
	})
	if err == nil {
		t.Fatal("ChatCompletion() error = nil, want missing model error")
	}
	if !strings.Contains(err.Error(), "model is required") {
		t.Fatalf("error = %q, want missing model", err)
	}
}

func TestChatCompletionResponsesEmptyOutputReturnsError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"output": []map[string]any{},
			"usage": map[string]any{
				"input_tokens":  3,
				"output_tokens": 0,
			},
		})
	}))
	defer server.Close()

	client, err := NewClient(Config{
		BaseURL: server.URL,
		APIKey:  "secret",
		Model:   "gpt-5.4",
		Timeout: time.Second,
	})
	if err != nil {
		t.Fatalf("NewClient() error = %v", err)
	}

	_, err = client.ChatCompletion(context.Background(), ChatCompletionRequest{
		Messages: []Message{{Role: "user", Content: "hello"}},
	})
	if err == nil {
		t.Fatal("ChatCompletion() error = nil, want empty response output error")
	}
	if !strings.Contains(err.Error(), "empty response output") {
		t.Fatalf("error = %q, want empty response output", err)
	}
}

func TestBuildResponsesPayloadRejectsUnknownRole(t *testing.T) {
	_, err := buildResponsesPayload("gpt-5.4", ChatCompletionRequest{
		Messages: []Message{
			{Role: "system", Content: "be concise"},
			{Role: "tool", Content: "tool result"},
			{Role: "user", Content: "hello"},
		},
	})
	if err == nil {
		t.Fatal("buildResponsesPayload() error = nil, want unsupported role error")
	}
	if !strings.Contains(err.Error(), "unsupported responses message role") {
		t.Fatalf("error = %q, want unsupported role error", err)
	}
}

func TestBuildResponsesPayloadIncludesTools(t *testing.T) {
	payload, err := buildResponsesPayload("gpt-5.4", ChatCompletionRequest{
		Messages: []Message{
			{Role: "system", Content: "be concise"},
			{Role: "user", Content: "hello"},
		},
		Tools: []ToolDefinition{{
			Name:        "draft_list",
			Description: "show drafts",
			Parameters: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"pending_only": map[string]any{"type": "boolean"},
				},
				"additionalProperties": false,
			},
			Strict: true,
		}},
	})
	if err != nil {
		t.Fatalf("buildResponsesPayload() error = %v", err)
	}
	if len(payload.Tools) != 1 {
		t.Fatalf("payload.Tools = %+v, want one tool", payload.Tools)
	}
	if payload.Tools[0].Name != "draft_list" || payload.Tools[0].Type != "function" {
		t.Fatalf("payload.Tools[0] = %+v, want draft_list function", payload.Tools[0])
	}
	if !payload.Tools[0].Strict {
		t.Fatalf("payload.Tools[0].Strict = false, want explicit client value true")
	}
}

func TestUsesResponsesAPITightAllowlist(t *testing.T) {
	if !usesResponsesAPI("gpt-5.4") {
		t.Fatal("usesResponsesAPI(gpt-5.4) = false, want true")
	}
	if usesResponsesAPI("gpt-50") {
		t.Fatal("usesResponsesAPI(gpt-50) = true, want false")
	}
}

func TestListModels(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/models" {
			t.Fatalf("path = %q, want /models", r.URL.Path)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"data": []map[string]any{
				{"id": "gpt-5.4"},
				{"id": "gpt-4.1"},
				{"id": "gpt-5.4"},
			},
		})
	}))
	defer server.Close()

	client, err := NewClient(Config{
		BaseURL: server.URL,
		APIKey:  "secret",
	})
	if err != nil {
		t.Fatalf("NewClient() error = %v", err)
	}

	models, err := client.ListModels(context.Background())
	if err != nil {
		t.Fatalf("ListModels() error = %v", err)
	}
	if len(models) != 2 {
		t.Fatalf("len(models) = %d, want 2", len(models))
	}
	if models[0].ID != "gpt-5.4" || models[1].ID != "gpt-4.1" {
		t.Fatalf("models = %+v", models)
	}
}

func TestListModelsReturnsAPIError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"error": map[string]any{
				"message": "not supported",
			},
		})
	}))
	defer server.Close()

	client, err := NewClient(Config{
		BaseURL: server.URL,
		APIKey:  "secret",
	})
	if err != nil {
		t.Fatalf("NewClient() error = %v", err)
	}

	_, err = client.ListModels(context.Background())
	if err == nil {
		t.Fatal("ListModels() error = nil, want api error")
	}
	if !strings.Contains(err.Error(), "not supported") {
		t.Fatalf("error = %q, want not supported", err)
	}
}
