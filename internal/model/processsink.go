package model

import (
	"fmt"
	"time"
)

type CheckpointState string

const (
	CheckpointPlaceholder  CheckpointState = "placeholder"
	CheckpointMaterialized CheckpointState = "materialized"
	CheckpointArchived     CheckpointState = "archived"
)

type SessionWindow struct {
	AgentID     string    `json:"agent_id"`
	SessionID   string    `json:"session_id"`
	WindowStart time.Time `json:"window_start"`
	WindowEnd   time.Time `json:"window_end"`
}

func (w SessionWindow) Key() string {
	return fmt.Sprintf(
		"%s|%s|%s|%s",
		w.AgentID,
		w.SessionID,
		w.WindowStart.UTC().Format(time.RFC3339),
		w.WindowEnd.UTC().Format(time.RFC3339),
	)
}

type CheckpointDoc struct {
	WindowKey     string          `json:"window_key"`
	Path          string          `json:"path"`
	Window        SessionWindow   `json:"window"`
	State         CheckpointState `json:"state"`
	Title         string          `json:"title"`
	Content       string          `json:"content"`
	RawTranscript string          `json:"raw_transcript,omitempty"`
	CreatedAt     time.Time       `json:"created_at"`
	UpdatedAt     time.Time       `json:"updated_at"`
}

type DailyReport struct {
	AgentID       string    `json:"agent_id"`
	ReportDay     time.Time `json:"report_day"`
	Path          string    `json:"path"`
	WindowKeys    []string  `json:"window_keys"`
	Title         string    `json:"title"`
	Content       string    `json:"content"`
	ArchivedPaths []string  `json:"archived_paths,omitempty"`
	CreatedAt     time.Time `json:"created_at"`
	UpdatedAt     time.Time `json:"updated_at"`
}

func NormalizeDay(ts time.Time) time.Time {
	year, month, day := ts.Date()
	return time.Date(year, month, day, 0, 0, 0, 0, ts.Location())
}
