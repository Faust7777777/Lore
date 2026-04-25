package openai

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
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

type ToolDefinition struct {
	Name        string
	Description string
	Parameters  map[string]any
	Strict      bool
}

type ToolCall struct {
	ID        string
	CallID    string
	Name      string
	Arguments map[string]any
}

type ChatCompletionRequest struct {
	Messages    []Message
	Tools       []ToolDefinition
	Temperature float64
	MaxTokens   int
}

type ChatCompletionResponse struct {
	Content          string
	ToolCalls        []ToolCall
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

const (
	chatCompletionRetryAttempts = 3
	chatCompletionRetryMinDelay = 300 * time.Millisecond
	chatCompletionRetryMaxDelay = 30 * time.Second
)

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

type responsesRequestPayload struct {
	Model             string                  `json:"model"`
	Input             []responsesInputMessage `json:"input"`
	Tools             []responsesTool         `json:"tools,omitempty"`
	ParallelToolCalls *bool                   `json:"parallel_tool_calls,omitempty"`
	Instructions      string                  `json:"instructions,omitempty"`
	Temperature       float64                 `json:"temperature,omitempty"`
	MaxOutputTokens   int                     `json:"max_output_tokens,omitempty"`
	Stream            bool                    `json:"stream"`
}

type responsesInputMessage struct {
	Type    string `json:"type"`
	Role    string `json:"role"`
	Content string `json:"content"`
}

type responsesTool struct {
	Type        string         `json:"type"`
	Name        string         `json:"name"`
	Description string         `json:"description,omitempty"`
	Parameters  map[string]any `json:"parameters"`
	Strict      bool           `json:"strict"`
}

type responsesResponsePayload struct {
	Output []struct {
		Type      string `json:"type"`
		Role      string `json:"role,omitempty"`
		Text      string `json:"text,omitempty"`
		Name      string `json:"name,omitempty"`
		Arguments string `json:"arguments,omitempty"`
		CallID    string `json:"call_id,omitempty"`
		ID        string `json:"id,omitempty"`
		Content   []struct {
			Type string `json:"type"`
			Text string `json:"text"`
		} `json:"content,omitempty"`
	} `json:"output"`
	OutputText string `json:"output_text,omitempty"`
	Usage      struct {
		InputTokens  int `json:"input_tokens"`
		OutputTokens int `json:"output_tokens"`
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

	var lastErr error
	for attempt := 1; attempt <= chatCompletionRetryAttempts; attempt++ {
		resp, err := c.chatCompletionOnce(ctx, req)
		if err == nil {
			return resp, nil
		}
		lastErr = err
		if !isRetryableChatCompletionError(err) || attempt >= chatCompletionRetryAttempts {
			return ChatCompletionResponse{}, err
		}
		if err := waitForRetry(ctx, retryDelayForChatCompletionError(err, attempt)); err != nil {
			return ChatCompletionResponse{}, err
		}
	}
	return ChatCompletionResponse{}, lastErr
}

func (c *Client) chatCompletionOnce(ctx context.Context, req ChatCompletionRequest) (ChatCompletionResponse, error) {
	if usesResponsesAPI(c.cfg.Model) {
		return c.responsesOnce(ctx, req)
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
	httpReq.Header.Set("User-Agent", "lore/operator-agent")

	httpResp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return ChatCompletionResponse{}, err
	}
	defer httpResp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(httpResp.Body, 1<<20))
	if err != nil {
		return ChatCompletionResponse{}, err
	}

	if httpResp.StatusCode < 200 || httpResp.StatusCode >= 300 {
		return ChatCompletionResponse{}, chatCompletionAPIError{
			StatusCode: httpResp.StatusCode,
			Message:    extractAPIErrorBody(body, httpResp.Status),
			RetryAfter: parseRetryAfterHeader(httpResp.Header.Get("Retry-After")),
		}
	}

	var parsed chatCompletionResponsePayload
	if err := json.Unmarshal(body, &parsed); err != nil {
		return ChatCompletionResponse{}, fmt.Errorf("openai client: decode response: %w", err)
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

func (c *Client) responsesOnce(ctx context.Context, req ChatCompletionRequest) (ChatCompletionResponse, error) {
	payload, err := buildResponsesPayload(c.cfg.Model, req)
	if err != nil {
		return ChatCompletionResponse{}, err
	}
	data, err := json.Marshal(payload)
	if err != nil {
		return ChatCompletionResponse{}, err
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, c.cfg.BaseURL+"/responses", bytes.NewReader(data))
	if err != nil {
		return ChatCompletionResponse{}, err
	}
	httpReq.Header.Set("Authorization", "Bearer "+c.cfg.APIKey)
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Accept", "application/json")
	httpReq.Header.Set("User-Agent", "lore/operator-agent")

	httpResp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return ChatCompletionResponse{}, err
	}
	defer httpResp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(httpResp.Body, 1<<20))
	if err != nil {
		return ChatCompletionResponse{}, err
	}

	if httpResp.StatusCode < 200 || httpResp.StatusCode >= 300 {
		return ChatCompletionResponse{}, chatCompletionAPIError{
			StatusCode: httpResp.StatusCode,
			Message:    extractAPIErrorBody(body, httpResp.Status),
			RetryAfter: parseRetryAfterHeader(httpResp.Header.Get("Retry-After")),
		}
	}

	var parsed responsesResponsePayload
	if err := json.Unmarshal(body, &parsed); err != nil {
		return ChatCompletionResponse{}, fmt.Errorf("openai client: decode responses payload: %w", err)
	}

	content := extractResponsesText(parsed)
	toolCalls, err := extractResponsesToolCalls(parsed)
	if err != nil {
		return ChatCompletionResponse{}, err
	}
	if content == "" && len(toolCalls) == 0 {
		return ChatCompletionResponse{}, fmt.Errorf("openai client: empty response output")
	}

	return ChatCompletionResponse{
		Content:          content,
		ToolCalls:        toolCalls,
		PromptTokens:     parsed.Usage.InputTokens,
		CompletionTokens: parsed.Usage.OutputTokens,
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
	httpReq.Header.Set("User-Agent", "lore/operator-agent")

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

type chatCompletionAPIError struct {
	StatusCode int
	Message    string
	RetryAfter time.Duration
}

func (e chatCompletionAPIError) Error() string {
	message := strings.TrimSpace(e.Message)
	if message == "" {
		message = http.StatusText(e.StatusCode)
	}
	if message == "" {
		message = "request failed"
	}
	return fmt.Sprintf("openai client: chat completion failed (%d): %s", e.StatusCode, message)
}

func isRetryableChatCompletionError(err error) bool {
	var apiErr chatCompletionAPIError
	if errors.As(err, &apiErr) {
		switch {
		case apiErr.StatusCode == http.StatusTooManyRequests:
			return true
		case apiErr.StatusCode >= 500 && apiErr.StatusCode <= 599:
			return true
		default:
			return false
		}
	}

	if errors.Is(err, io.EOF) {
		return true
	}
	var netErr net.Error
	return errors.As(err, &netErr)
}

func extractAPIErrorBody(body []byte, fallback string) string {
	var parsed chatCompletionResponsePayload
	if err := json.Unmarshal(body, &parsed); err == nil {
		if message := strings.TrimSpace(extractAPIError(parsed, "")); message != "" {
			return message
		}
	}

	raw := strings.TrimSpace(string(body))
	if raw == "" {
		return strings.TrimSpace(fallback)
	}
	if len(raw) > 300 {
		raw = raw[:300] + "..."
	}
	return raw
}

func waitForRetry(ctx context.Context, delay time.Duration) error {
	timer := time.NewTimer(delay)
	defer timer.Stop()

	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

func retryDelayForChatCompletionError(err error, attempt int) time.Duration {
	var apiErr chatCompletionAPIError
	if errors.As(err, &apiErr) && apiErr.RetryAfter > 0 {
		return clampDuration(maxDuration(apiErr.RetryAfter, chatCompletionRetryMinDelay), chatCompletionRetryMinDelay, chatCompletionRetryMaxDelay)
	}

	if attempt < 1 {
		attempt = 1
	}
	delay := chatCompletionRetryMinDelay * time.Duration(1<<(attempt-1))
	return clampDuration(delay, chatCompletionRetryMinDelay, chatCompletionRetryMaxDelay)
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

func usesResponsesAPI(model string) bool {
	model = strings.ToLower(strings.TrimSpace(model))
	switch model {
	case "gpt-5.4", "gpt-5.4-pro":
		return true
	default:
		return false
	}
}

func buildResponsesPayload(model string, req ChatCompletionRequest) (responsesRequestPayload, error) {
	payload := responsesRequestPayload{
		Model:           model,
		Temperature:     req.Temperature,
		MaxOutputTokens: req.MaxTokens,
		Stream:          false,
	}

	instructions := make([]string, 0, len(req.Messages))
	input := make([]responsesInputMessage, 0, len(req.Messages))
	for _, message := range req.Messages {
		role := strings.TrimSpace(strings.ToLower(message.Role))
		content := strings.TrimSpace(message.Content)
		if content == "" {
			continue
		}
		switch role {
		case "system", "developer":
			instructions = append(instructions, content)
		case "user", "assistant":
			input = append(input, responsesInputMessage{
				Type:    "message",
				Role:    role,
				Content: content,
			})
		default:
			return responsesRequestPayload{}, fmt.Errorf("openai client: unsupported responses message role %q", message.Role)
		}
	}
	if len(input) == 0 {
		return responsesRequestPayload{}, fmt.Errorf("openai client: responses input is empty")
	}
	payload.Input = input
	payload.Instructions = strings.Join(instructions, "\n\n")
	if len(req.Tools) > 0 {
		parallelToolCalls := false
		payload.ParallelToolCalls = &parallelToolCalls
		payload.Tools = make([]responsesTool, 0, len(req.Tools))
		for _, tool := range req.Tools {
			parameters := tool.Parameters
			if len(parameters) == 0 {
				parameters = map[string]any{
					"type":       "object",
					"properties": map[string]any{},
				}
			}
			payload.Tools = append(payload.Tools, responsesTool{
				Type:        "function",
				Name:        strings.TrimSpace(tool.Name),
				Description: strings.TrimSpace(tool.Description),
				Parameters:  parameters,
				Strict:      tool.Strict,
			})
		}
	}
	return payload, nil
}

func extractResponsesText(parsed responsesResponsePayload) string {
	for _, item := range parsed.Output {
		if item.Type == "output_text" && strings.TrimSpace(item.Text) != "" {
			return strings.TrimSpace(item.Text)
		}
		for _, part := range item.Content {
			if part.Type == "output_text" && strings.TrimSpace(part.Text) != "" {
				return strings.TrimSpace(part.Text)
			}
		}
	}
	return strings.TrimSpace(parsed.OutputText)
}

func extractResponsesToolCalls(parsed responsesResponsePayload) ([]ToolCall, error) {
	toolCalls := make([]ToolCall, 0)
	for _, item := range parsed.Output {
		if item.Type != "function_call" {
			continue
		}
		name := strings.TrimSpace(item.Name)
		if name == "" {
			return nil, fmt.Errorf("openai client: responses function_call missing name")
		}
		arguments := map[string]any{}
		rawArguments := strings.TrimSpace(item.Arguments)
		if rawArguments != "" {
			if err := json.Unmarshal([]byte(rawArguments), &arguments); err != nil {
				return nil, fmt.Errorf("openai client: decode function_call arguments for %s: %w", name, err)
			}
		}
		toolCalls = append(toolCalls, ToolCall{
			ID:        strings.TrimSpace(item.ID),
			CallID:    strings.TrimSpace(item.CallID),
			Name:      name,
			Arguments: arguments,
		})
	}
	return toolCalls, nil
}

func parseRetryAfterHeader(raw string) time.Duration {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return 0
	}
	if seconds, err := time.ParseDuration(raw + "s"); err == nil && seconds > 0 {
		return seconds
	}
	if retryAt, err := http.ParseTime(raw); err == nil {
		delay := time.Until(retryAt)
		if delay > 0 {
			return delay
		}
	}
	return 0
}

func clampDuration(value time.Duration, min time.Duration, max time.Duration) time.Duration {
	if value < min {
		return min
	}
	if value > max {
		return max
	}
	return value
}

func maxDuration(a time.Duration, b time.Duration) time.Duration {
	if a > b {
		return a
	}
	return b
}
