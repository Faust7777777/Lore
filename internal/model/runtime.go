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

// NormalizeUsageDay buckets a timestamp into the local calendar day
// midnight. Usage records carry RecordedAt in UTC (operatoragent
// captures time.Now().UTC() before each ChatCompletion), but CLI
// callers query "today" via time.Now() which is local. Without
// converting both sides to a single zone first, an Asia/Shanghai user
// making a model call at 2026-05-18 00:30 +0800 (= 2026-05-17 16:30
// UTC) would see the cost bucketed against 2026-05-17 UTC while
// `lore usage` looked up 2026-05-18 local and miss it.
//
// All three store backends (memory, jsonstore, sqlitestore) must use
// this helper on both the write and the read side of usage records so
// they agree on bucket boundaries. Process-sink data uses NormalizeDay
// directly because its Window times are constructed with intentional
// locations by the codex JSONL importer.
func NormalizeUsageDay(ts time.Time) time.Time {
	return NormalizeDay(ts.Local())
}
