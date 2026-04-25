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
)

const (
	DefaultMaxToolArgumentsBytes = 4096
	DefaultMaxToolErrorBytes     = 2048
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
