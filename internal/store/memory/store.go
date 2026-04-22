package memory

import (
	"sort"
	"sync"
	"time"

	"obsidian-harness/internal/model"
	"obsidian-harness/internal/store"
)

type Store struct {
	mu          sync.RWMutex
	drafts      map[string]model.Draft
	checkpoints map[string]model.CheckpointDoc
	reports     map[string]model.DailyReport
	audit       []model.AuditRecord
	usage       []model.UsageRecord
	cursors     map[string]string
}

func New() *Store {
	return &Store{
		drafts:      make(map[string]model.Draft),
		checkpoints: make(map[string]model.CheckpointDoc),
		reports:     make(map[string]model.DailyReport),
		cursors:     make(map[string]string),
	}
}

func (s *Store) Drafts() store.DraftStore {
	return s
}

func (s *Store) ProcessSink() store.ProcessSinkStore {
	return s
}

func (s *Store) Audit() store.AuditStore {
	return s
}

func (s *Store) Usage() store.UsageStore {
	return s
}

func (s *Store) Cursors() store.CursorStore {
	return s
}

func (s *Store) SaveDraft(draft model.Draft) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.drafts[draft.ID] = draft
	return nil
}

func (s *Store) GetDraft(id string) (model.Draft, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	draft, ok := s.drafts[id]
	if !ok {
		return model.Draft{}, store.ErrNotFound
	}
	return draft, nil
}

func (s *Store) ListDrafts() ([]model.Draft, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	drafts := make([]model.Draft, 0, len(s.drafts))
	for _, draft := range s.drafts {
		drafts = append(drafts, draft)
	}
	sort.Slice(drafts, func(i, j int) bool {
		return drafts[i].UpdatedAt.After(drafts[j].UpdatedAt)
	})
	return drafts, nil
}

func (s *Store) UpdateDraftState(id string, state model.DraftState, updatedAt time.Time) (model.Draft, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	draft, ok := s.drafts[id]
	if !ok {
		return model.Draft{}, store.ErrNotFound
	}
	draft.State = state
	draft.UpdatedAt = updatedAt
	s.drafts[id] = draft
	return draft, nil
}

func (s *Store) SaveCheckpoint(doc model.CheckpointDoc) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.checkpoints[doc.WindowKey] = doc
	return nil
}

func (s *Store) GetCheckpointByWindowKey(windowKey string) (model.CheckpointDoc, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	doc, ok := s.checkpoints[windowKey]
	if !ok {
		return model.CheckpointDoc{}, store.ErrNotFound
	}
	return doc, nil
}

func (s *Store) ListCheckpointsByDay(agentID string, day time.Time) ([]model.CheckpointDoc, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	normalized := model.NormalizeDay(day)
	docs := make([]model.CheckpointDoc, 0)
	for _, doc := range s.checkpoints {
		if doc.Window.AgentID != agentID {
			continue
		}
		if !model.NormalizeDay(doc.Window.WindowStart).Equal(normalized) {
			continue
		}
		docs = append(docs, doc)
	}
	sort.Slice(docs, func(i, j int) bool {
		return docs[i].Window.WindowStart.Before(docs[j].Window.WindowStart)
	})
	return docs, nil
}

func reportKey(agentID string, day time.Time) string {
	return agentID + "|" + model.NormalizeDay(day).Format("2006-01-02")
}

func (s *Store) SaveDailyReport(report model.DailyReport) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.reports[reportKey(report.AgentID, report.ReportDay)] = report
	return nil
}

func (s *Store) GetDailyReport(agentID string, day time.Time) (model.DailyReport, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	report, ok := s.reports[reportKey(agentID, day)]
	if !ok {
		return model.DailyReport{}, store.ErrNotFound
	}
	return report, nil
}

func (s *Store) AppendAudit(record model.AuditRecord) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.audit = append(s.audit, record)
	return nil
}

func (s *Store) ListAudit(limit int) ([]model.AuditRecord, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if limit <= 0 || limit >= len(s.audit) {
		return append([]model.AuditRecord(nil), s.audit...), nil
	}
	start := len(s.audit) - limit
	return append([]model.AuditRecord(nil), s.audit[start:]...), nil
}

func (s *Store) AppendUsage(record model.UsageRecord) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.usage = append(s.usage, record)
	return nil
}

func (s *Store) SummarizeUsage(day time.Time) (model.UsageSummary, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	normalized := model.NormalizeDay(day)
	summary := model.UsageSummary{Day: normalized}
	for _, record := range s.usage {
		if !model.NormalizeDay(record.RecordedAt).Equal(normalized) {
			continue
		}
		summary.Calls++
		summary.PromptTokens += record.PromptTokens
		summary.CompletionTokens += record.CompletionTokens
	}
	summary.TotalTokens = summary.PromptTokens + summary.CompletionTokens
	return summary, nil
}

func (s *Store) SaveCursor(source string, cursor string) error {
	if source == "" {
		return store.ErrInvalidKey
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.cursors[source] = cursor
	return nil
}

func (s *Store) GetCursor(source string) (string, error) {
	if source == "" {
		return "", store.ErrInvalidKey
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	cursor, ok := s.cursors[source]
	if !ok {
		return "", store.ErrNotFound
	}
	return cursor, nil
}
