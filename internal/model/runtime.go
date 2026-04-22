package model

import "time"

type HealthSnapshot struct {
	Outcome          Outcome   `json:"outcome"`
	ModelAvailable   bool      `json:"model_available"`
	AdapterConnected bool      `json:"adapter_connected"`
	Message          string    `json:"message,omitempty"`
	CheckedAt        time.Time `json:"checked_at"`
}

type UsageRecord struct {
	Provider         string    `json:"provider"`
	Model            string    `json:"model"`
	AgentID          string    `json:"agent_id,omitempty"`
	SessionID        string    `json:"session_id,omitempty"`
	PromptTokens     int       `json:"prompt_tokens"`
	CompletionTokens int       `json:"completion_tokens"`
	RecordedAt       time.Time `json:"recorded_at"`
}

func (u UsageRecord) TotalTokens() int {
	return u.PromptTokens + u.CompletionTokens
}

type UsageSummary struct {
	Day              time.Time `json:"day"`
	Calls            int       `json:"calls"`
	PromptTokens     int       `json:"prompt_tokens"`
	CompletionTokens int       `json:"completion_tokens"`
	TotalTokens      int       `json:"total_tokens"`
}
