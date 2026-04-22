package openai

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

type Config struct {
	BaseURL string
	APIKey  string
	Model   string
	Timeout time.Duration
}

type Message struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type ChatCompletionRequest struct {
	Messages    []Message
	Temperature float64
	MaxTokens   int
}

type ChatCompletionResponse struct {
	Content          string
	PromptTokens     int
	CompletionTokens int
}

type ModelInfo struct {
	ID string
}

type Client struct {
	cfg        Config
	httpClient *http.Client
}

type chatCompletionRequestPayload struct {
	Model       string    `json:"model"`
	Messages    []Message `json:"messages"`
	Temperature float64   `json:"temperature,omitempty"`
	MaxTokens   int       `json:"max_tokens,omitempty"`
	Stream      bool      `json:"stream"`
}

type chatCompletionResponsePayload struct {
	Choices []struct {
		Message struct {
			Content string `json:"content"`
		} `json:"message"`
	} `json:"choices"`
	Usage struct {
		PromptTokens     int `json:"prompt_tokens"`
		CompletionTokens int `json:"completion_tokens"`
	} `json:"usage"`
	Error *struct {
		Message string `json:"message"`
		Type    string `json:"type"`
	} `json:"error,omitempty"`
}

type listModelsResponsePayload struct {
	Data []struct {
		ID string `json:"id"`
	} `json:"data"`
	Error *struct {
		Message string `json:"message"`
		Type    string `json:"type"`
	} `json:"error,omitempty"`
}

func NewClient(cfg Config) (*Client, error) {
	cfg.BaseURL = strings.TrimRight(strings.TrimSpace(cfg.BaseURL), "/")
	cfg.APIKey = strings.TrimSpace(cfg.APIKey)
	cfg.Model = strings.TrimSpace(cfg.Model)
	if cfg.Timeout <= 0 {
		cfg.Timeout = 30 * time.Second
	}

	if cfg.BaseURL == "" {
		return nil, fmt.Errorf("openai client: base url is required")
	}
	if cfg.APIKey == "" {
		return nil, fmt.Errorf("openai client: api key is required")
	}
	if _, err := url.Parse(cfg.BaseURL); err != nil {
		return nil, fmt.Errorf("openai client: invalid base url: %w", err)
	}

	return &Client{
		cfg: cfg,
		httpClient: &http.Client{
			Timeout: cfg.Timeout,
		},
	}, nil
}

func (c *Client) ChatCompletion(ctx context.Context, req ChatCompletionRequest) (ChatCompletionResponse, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if strings.TrimSpace(c.cfg.Model) == "" {
		return ChatCompletionResponse{}, fmt.Errorf("openai client: model is required for chat completion")
	}
	if len(req.Messages) == 0 {
		return ChatCompletionResponse{}, fmt.Errorf("openai client: at least one message is required")
	}

	payload := chatCompletionRequestPayload{
		Model:       c.cfg.Model,
		Messages:    req.Messages,
		Temperature: req.Temperature,
		MaxTokens:   req.MaxTokens,
		Stream:      false,
	}
	data, err := json.Marshal(payload)
	if err != nil {
		return ChatCompletionResponse{}, err
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, c.cfg.BaseURL+"/chat/completions", bytes.NewReader(data))
	if err != nil {
		return ChatCompletionResponse{}, err
	}
	httpReq.Header.Set("Authorization", "Bearer "+c.cfg.APIKey)
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Accept", "application/json")
	httpReq.Header.Set("User-Agent", "obsidian-harness/operator-agent")

	httpResp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return ChatCompletionResponse{}, err
	}
	defer httpResp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(httpResp.Body, 1<<20))
	if err != nil {
		return ChatCompletionResponse{}, err
	}

	var parsed chatCompletionResponsePayload
	if err := json.Unmarshal(body, &parsed); err != nil {
		return ChatCompletionResponse{}, fmt.Errorf("openai client: decode response: %w", err)
	}

	if httpResp.StatusCode < 200 || httpResp.StatusCode >= 300 {
		message := strings.TrimSpace(extractAPIError(parsed, string(body)))
		if message == "" {
			message = httpResp.Status
		}
		return ChatCompletionResponse{}, fmt.Errorf("openai client: chat completion failed: %s", message)
	}

	if len(parsed.Choices) == 0 {
		return ChatCompletionResponse{}, fmt.Errorf("openai client: empty choices")
	}

	content := strings.TrimSpace(parsed.Choices[0].Message.Content)
	if content == "" {
		return ChatCompletionResponse{}, fmt.Errorf("openai client: empty message content")
	}

	return ChatCompletionResponse{
		Content:          content,
		PromptTokens:     parsed.Usage.PromptTokens,
		CompletionTokens: parsed.Usage.CompletionTokens,
	}, nil
}

func (c *Client) ListModels(ctx context.Context) ([]ModelInfo, error) {
	if ctx == nil {
		ctx = context.Background()
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodGet, c.cfg.BaseURL+"/models", nil)
	if err != nil {
		return nil, err
	}
	httpReq.Header.Set("Authorization", "Bearer "+c.cfg.APIKey)
	httpReq.Header.Set("Accept", "application/json")
	httpReq.Header.Set("User-Agent", "obsidian-harness/operator-agent")

	httpResp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return nil, err
	}
	defer httpResp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(httpResp.Body, 1<<20))
	if err != nil {
		return nil, err
	}

	var parsed listModelsResponsePayload
	if err := json.Unmarshal(body, &parsed); err != nil {
		return nil, fmt.Errorf("openai client: decode models response: %w", err)
	}

	if httpResp.StatusCode < 200 || httpResp.StatusCode >= 300 {
		message := strings.TrimSpace(extractModelListError(parsed, string(body)))
		if message == "" {
			message = httpResp.Status
		}
		return nil, fmt.Errorf("openai client: list models failed: %s", message)
	}

	seen := make(map[string]struct{}, len(parsed.Data))
	models := make([]ModelInfo, 0, len(parsed.Data))
	for _, item := range parsed.Data {
		id := strings.TrimSpace(item.ID)
		if id == "" {
			continue
		}
		key := strings.ToLower(id)
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		models = append(models, ModelInfo{ID: id})
	}
	if len(models) == 0 {
		return nil, fmt.Errorf("openai client: models endpoint returned no models")
	}
	return models, nil
}

func extractAPIError(parsed chatCompletionResponsePayload, raw string) string {
	if parsed.Error != nil {
		return strings.TrimSpace(parsed.Error.Message)
	}
	raw = strings.TrimSpace(raw)
	if len(raw) > 300 {
		raw = raw[:300] + "..."
	}
	return raw
}

func extractModelListError(parsed listModelsResponsePayload, raw string) string {
	if parsed.Error != nil {
		return strings.TrimSpace(parsed.Error.Message)
	}
	raw = strings.TrimSpace(raw)
	if len(raw) > 300 {
		raw = raw[:300] + "..."
	}
	return raw
}
