package store

import (
	"errors"
	"time"

	"obsidian-harness/internal/model"
)

var (
	ErrNotFound   = errors.New("store: not found")
	ErrConflict   = errors.New("store: conflict")
	ErrInvalidKey = errors.New("store: invalid key")
)

type DraftStore interface {
	SaveDraft(draft model.Draft) error
	GetDraft(id string) (model.Draft, error)
	ListDrafts() ([]model.Draft, error)
	UpdateDraftState(id string, state model.DraftState, updatedAt time.Time) (model.Draft, error)
	SupersedeDraft(oldID string, newDraft model.Draft, updatedAt time.Time) (model.Draft, model.Draft, error)
}

type ProcessSinkStore interface {
	SaveCheckpoint(doc model.CheckpointDoc) error
	GetCheckpointByWindowKey(windowKey string) (model.CheckpointDoc, error)
	ListCheckpointsByDay(agentID string, day time.Time) ([]model.CheckpointDoc, error)
	SaveDailyReport(report model.DailyReport) error
	GetDailyReport(agentID string, day time.Time) (model.DailyReport, error)
}

type AuditStore interface {
	AppendAudit(record model.AuditRecord) error
	ListAudit(limit int) ([]model.AuditRecord, error)
}

type FindingStore interface {
	SaveFinding(finding model.Finding) error
	GetFinding(id string) (model.Finding, error)
	ListFindings(limit int) ([]model.Finding, error)
	UpdateFindingState(id string, state model.FindingState, updatedAt time.Time) (model.Finding, error)
}

type UsageStore interface {
	AppendUsage(record model.UsageRecord) error
	SummarizeUsage(day time.Time) (model.UsageSummary, error)
}

type CursorStore interface {
	SaveCursor(source string, cursor string) error
	GetCursor(source string) (string, error)
}

type StateStore interface {
	Drafts() DraftStore
	ProcessSink() ProcessSinkStore
	Audit() AuditStore
	Findings() FindingStore
	Usage() UsageStore
	Cursors() CursorStore
}
