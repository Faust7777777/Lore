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
