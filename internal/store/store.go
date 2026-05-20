package store

import (
	"errors"
	"time"

	"obsidian-harness/internal/model"
	"obsidian-harness/internal/persona"
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

// PersonaCandidateStore persists LLM-mined persona update candidates
// produced by internal/persona's extractor. The contract is small on
// purpose -- P3 only persists; P4 wires console.Session to call
// UpsertCandidate after each user turn, and P5+ adds the candidate ->
// draft promotion via existing harness.ProposePersonaUpdate.
//
// Backends MUST enforce DedupKey uniqueness: a second
// UpsertCandidate with a record whose DedupKey already exists returns
// the stored record (with isNew=false) instead of inserting a new
// row. This is the storage-level half of the extractor's "do not
// emit the same fact twice" contract.
type PersonaCandidateStore interface {
	// UpsertCandidate inserts the record if its DedupKey is not yet
	// present, or returns the existing record otherwise. isNew is
	// true only when a fresh insertion occurred. The returned record
	// reflects the stored row, including any backend-assigned
	// timestamps.
	UpsertCandidate(record persona.PersonaCandidateRecord) (stored persona.PersonaCandidateRecord, isNew bool, err error)
	// GetCandidate returns the record with the given ID, or
	// ErrNotFound if none exists.
	GetCandidate(id string) (persona.PersonaCandidateRecord, error)
	// ListCandidatesByState returns up to limit records in the given
	// state, newest UpdatedAt first. limit <= 0 means "no cap".
	ListCandidatesByState(state persona.PersonaCandidateState, limit int) ([]persona.PersonaCandidateRecord, error)
	// UpdateCandidateState transitions a record to the new state,
	// stamping UpdatedAt. Returns the post-transition record, or
	// ErrNotFound when the ID does not exist.
	UpdateCandidateState(id string, state persona.PersonaCandidateState, updatedAt time.Time) (persona.PersonaCandidateRecord, error)
}

type StateStore interface {
	Drafts() DraftStore
	ProcessSink() ProcessSinkStore
	Audit() AuditStore
	Findings() FindingStore
	Usage() UsageStore
	Cursors() CursorStore
	PersonaCandidates() PersonaCandidateStore
}
