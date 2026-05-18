package sessionlog

import (
	"encoding/json"
	"time"

	"obsidian-harness/internal/operatoragent"
)

const (
	EventSessionMeta      = "session_meta"
	EventUserMessage      = "user_message"
	EventAssistantMessage = "assistant_message"
	EventToolCall         = "tool_call"
	EventWorkingSet       = "working_set"
	EventLocalCommand     = "local_command"
	EventError            = "error"
	EventSessionEnd       = "session_end"
	EventModelUsage       = "model_usage"
)

const (
	DefaultMaxToolArgumentsBytes = 4096
	DefaultMaxToolErrorBytes     = 2048
	MaxJSONLLineBytes            = 16 * 1024 * 1024
)

type Meta struct {
	SessionID string    `json:"session_id"`
	AgentID   string    `json:"agent_id,omitempty"`
	Model     string    `json:"model,omitempty"`
	WorkDir   string    `json:"workdir,omitempty"`
	VaultRoot string    `json:"vault_root,omitempty"`
	StartedAt time.Time `json:"started_at"`
}

type Event struct {
	Version     int                            `json:"version,omitempty"`
	Type        string                         `json:"type"`
	Timestamp   time.Time                      `json:"timestamp"`
	SessionID   string                         `json:"session_id,omitempty"`
	AgentID     string                         `json:"agent_id,omitempty"`
	Model       string                         `json:"model,omitempty"`
	WorkDir     string                         `json:"workdir,omitempty"`
	VaultRoot   string                         `json:"vault_root,omitempty"`
	Text        string                         `json:"text,omitempty"`
	Name        string                         `json:"name,omitempty"`
	Arguments   json.RawMessage                `json:"arguments,omitempty"`
	Status      string                         `json:"status,omitempty"`
	Error       string                         `json:"error,omitempty"`
	Items       []operatoragent.WorkingSetItem `json:"items,omitempty"`
	Command     string                         `json:"command,omitempty"`
	Reason      string                         `json:"reason,omitempty"`
	Recoverable bool                           `json:"recoverable,omitempty"`
	// Model-usage event fields. Provider/Model reuse semantics from
	// session_meta; StartedAt is when the LLM request was issued
	// (distinct from Timestamp which is when the event was recorded).
	// All fields are omitempty so older JSONL files (no usage events)
	// remain valid Event records when re-read.
	Provider         string    `json:"provider,omitempty"`
	PromptTokens     int       `json:"prompt_tokens,omitempty"`
	CompletionTokens int       `json:"completion_tokens,omitempty"`
	StartedAt        time.Time `json:"started_at,omitempty"`
}

type Summary struct {
	ID        string    `json:"id"`
	Path      string    `json:"path"`
	StartedAt time.Time `json:"started_at"`
	UpdatedAt time.Time `json:"updated_at"`
	Title     string    `json:"title"`
	TurnCount int       `json:"turn_count"`
	Model     string    `json:"model,omitempty"`
	AgentID   string    `json:"agent_id,omitempty"`
}

type Index struct {
	Version  int       `json:"version"`
	Sessions []Summary `json:"sessions"`
}

type Snapshot struct {
	Meta       Meta
	Summary    Summary
	History    []operatoragent.ConversationTurn
	WorkingSet []operatoragent.WorkingSetItem
	Warnings   []string
}
