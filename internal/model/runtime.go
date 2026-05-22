package model

import "time"

type HealthSnapshot struct {
	Outcome          Outcome   `json:"outcome"`
	ModelAvailable   bool      `json:"model_available"`
	AdapterConnected bool      `json:"adapter_connected"`
	Message          string    `json:"message,omitempty"`
	CheckedAt        time.Time `json:"checked_at"`
}

// UsagePurpose names the Lore subsystem that billed a UsageRecord.
// Stable string constants suitable for storage; new writers should use
// these rather than free-form strings so the `lore usage` breakdown
// stays consistent. Empty Purpose is allowed and means "unspecified".
const (
	UsagePurposeChat           = "chat"
	UsagePurposeProcessSink    = "process_sink"
	UsagePurposePersonaExtract = "persona_extract"
)

type UsageRecord struct {
	Provider         string    `json:"provider"`
	Model            string    `json:"model"`
	AgentID          string    `json:"agent_id,omitempty"`
	SessionID        string    `json:"session_id,omitempty"`
	PromptTokens     int       `json:"prompt_tokens"`
	CompletionTokens int       `json:"completion_tokens"`
	RecordedAt       time.Time `json:"recorded_at"`
	// Purpose distinguishes which Lore subsystem billed this model
	// call. Values defined today: "chat" (operatoragent loop turn),
	// "process_sink" (process-sink summarizer), "persona_extract"
	// (persona candidate extractor). Older records that pre-date this
	// field deserialize with Purpose == "" and remain valid; CLI/UI
	// callers should treat the empty value as "unspecified". New
	// writers are expected to populate Purpose; backfill of historic
	// rows is intentionally not attempted -- the field exists so that
	// future categories (e.g. background extractors) can be split out
	// in `lore usage` without re-introducing the chat/extractor
	// ambiguity that motivated this addition.
	Purpose string `json:"purpose,omitempty"`
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
	// PurposeBreakdown disaggregates the day's calls and tokens by
	// UsagePurpose so `lore usage` can show operators what fraction
	// of cost went to chat vs persona_extract vs process_sink. Keyed
	// by the Purpose string ("chat", "persona_extract", etc.); empty
	// or absent Purpose collapses to the empty-string bucket.
	// Stores populate this in SummarizeUsage from the same scan that
	// fills Calls/PromptTokens/CompletionTokens; pre-B-P11 records
	// (no Purpose field) deserialize as Purpose="" and land in the
	// empty bucket so legacy data still surfaces.
	PurposeBreakdown map[string]UsagePurposeStats `json:"purpose_breakdown,omitempty"`
}

// UsagePurposeStats holds the per-Purpose aggregate inside a
// UsageSummary.PurposeBreakdown map. Sum of all bucket Calls equals
// the parent UsageSummary.Calls, same for tokens, so a caller can
// reconcile the breakdown against the top-line totals.
type UsagePurposeStats struct {
	Calls            int `json:"calls"`
	PromptTokens     int `json:"prompt_tokens"`
	CompletionTokens int `json:"completion_tokens"`
}

// TotalTokens is a convenience accessor on the bucket; equivalent to
// PromptTokens + CompletionTokens. Kept off the JSON form to avoid
// drift if the two fields are ever recomputed independently.
func (u UsagePurposeStats) TotalTokens() int {
	return u.PromptTokens + u.CompletionTokens
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
