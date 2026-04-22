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
