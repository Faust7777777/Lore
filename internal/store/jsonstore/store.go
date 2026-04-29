package jsonstore

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"time"

	"obsidian-harness/internal/model"
	"obsidian-harness/internal/store"
	"obsidian-harness/internal/vault"
)

type persistedState struct {
	Drafts      map[string]model.Draft         `json:"drafts"`
	Checkpoints map[string]model.CheckpointDoc `json:"checkpoints"`
	Reports     map[string]model.DailyReport   `json:"reports"`
	Audit       []model.AuditRecord            `json:"audit"`
	Findings    map[string]model.Finding       `json:"findings"`
	Usage       []model.UsageRecord            `json:"usage"`
	Cursors     map[string]string              `json:"cursors"`
}

type Store struct {
	mu         sync.RWMutex
	path       string
	tempSuffix string
	state      persistedState
}

func New(path string, tempSuffix string) (*Store, error) {
	s := &Store{
		path:       path,
		tempSuffix: tempSuffix,
		state: persistedState{
			Drafts:      make(map[string]model.Draft),
			Checkpoints: make(map[string]model.CheckpointDoc),
			Reports:     make(map[string]model.DailyReport),
			Findings:    make(map[string]model.Finding),
			Cursors:     make(map[string]string),
		},
	}

	if err := s.load(); err != nil {
		return nil, err
	}
	return s, nil
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

func (s *Store) Findings() store.FindingStore {
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
	s.state.Drafts[draft.ID] = draft
	return s.persistLocked()
}

func (s *Store) GetDraft(id string) (model.Draft, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	draft, ok := s.state.Drafts[id]
	if !ok {
		return model.Draft{}, store.ErrNotFound
	}
	return draft, nil
}

func (s *Store) ListDrafts() ([]model.Draft, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	drafts := make([]model.Draft, 0, len(s.state.Drafts))
	for _, draft := range s.state.Drafts {
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

	draft, ok := s.state.Drafts[id]
	if !ok {
		return model.Draft{}, store.ErrNotFound
	}
	draft.State = state
	draft.UpdatedAt = updatedAt
	s.state.Drafts[id] = draft
	if err := s.persistLocked(); err != nil {
		return model.Draft{}, err
	}
	return draft, nil
}

func (s *Store) SupersedeDraft(oldID string, newDraft model.Draft, updatedAt time.Time) (model.Draft, model.Draft, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if oldID == "" || newDraft.ID == "" {
		return model.Draft{}, model.Draft{}, store.ErrInvalidKey
	}
	if oldID == newDraft.ID {
		return model.Draft{}, model.Draft{}, store.ErrConflict
	}
	originalDraft, ok := s.state.Drafts[oldID]
	if !ok {
		return model.Draft{}, model.Draft{}, store.ErrNotFound
	}
	if _, exists := s.state.Drafts[newDraft.ID]; exists {
		return model.Draft{}, model.Draft{}, store.ErrConflict
	}

	oldDraft := originalDraft
	oldDraft.State = model.DraftSuperseded
	oldDraft.UpdatedAt = updatedAt
	s.state.Drafts[oldID] = oldDraft
	s.state.Drafts[newDraft.ID] = newDraft
	if err := s.persistLocked(); err != nil {
		delete(s.state.Drafts, newDraft.ID)
		s.state.Drafts[oldID] = originalDraft
		return model.Draft{}, model.Draft{}, err
	}
	return oldDraft, newDraft, nil
}

func (s *Store) SaveCheckpoint(doc model.CheckpointDoc) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.state.Checkpoints[doc.WindowKey] = doc
	return s.persistLocked()
}

func (s *Store) GetCheckpointByWindowKey(windowKey string) (model.CheckpointDoc, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	doc, ok := s.state.Checkpoints[windowKey]
	if !ok {
		return model.CheckpointDoc{}, store.ErrNotFound
	}
	return doc, nil
}

func (s *Store) ListCheckpointsByDay(agentID string, day time.Time) ([]model.CheckpointDoc, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	normalized := model.NormalizeDay(day)
	checkpoints := make([]model.CheckpointDoc, 0)
	for _, doc := range s.state.Checkpoints {
		if doc.Window.AgentID != agentID {
			continue
		}
		if !model.NormalizeDay(doc.Window.WindowStart).Equal(normalized) {
			continue
		}
		checkpoints = append(checkpoints, doc)
	}
	sort.Slice(checkpoints, func(i, j int) bool {
		return checkpoints[i].Window.WindowStart.Before(checkpoints[j].Window.WindowStart)
	})
	return checkpoints, nil
}

func (s *Store) SaveDailyReport(report model.DailyReport) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.state.Reports[reportKey(report.AgentID, report.ReportDay)] = report
	return s.persistLocked()
}

func (s *Store) GetDailyReport(agentID string, day time.Time) (model.DailyReport, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	report, ok := s.state.Reports[reportKey(agentID, day)]
	if !ok {
		return model.DailyReport{}, store.ErrNotFound
	}
	return report, nil
}

func (s *Store) AppendAudit(record model.AuditRecord) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	record = model.NormalizeAuditRecord(record)
	s.state.Audit = append(s.state.Audit, record)
	return s.persistLocked()
}

func (s *Store) ListAudit(limit int) ([]model.AuditRecord, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if limit <= 0 || limit >= len(s.state.Audit) {
		return append([]model.AuditRecord(nil), s.state.Audit...), nil
	}
	start := len(s.state.Audit) - limit
	return append([]model.AuditRecord(nil), s.state.Audit[start:]...), nil
}

func (s *Store) SaveFinding(finding model.Finding) error {
	if finding.ID == "" {
		return store.ErrInvalidKey
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.state.Findings[finding.ID] = finding
	return s.persistLocked()
}

func (s *Store) GetFinding(id string) (model.Finding, error) {
	if id == "" {
		return model.Finding{}, store.ErrInvalidKey
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	finding, ok := s.state.Findings[id]
	if !ok {
		return model.Finding{}, store.ErrNotFound
	}
	return finding, nil
}

func (s *Store) ListFindings(limit int) ([]model.Finding, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	findings := make([]model.Finding, 0, len(s.state.Findings))
	for _, finding := range s.state.Findings {
		findings = append(findings, finding)
	}
	sort.Slice(findings, func(i, j int) bool {
		if findings[i].UpdatedAt.Equal(findings[j].UpdatedAt) {
			return findings[i].ID < findings[j].ID
		}
		return findings[i].UpdatedAt.After(findings[j].UpdatedAt)
	})
	if limit > 0 && limit < len(findings) {
		findings = findings[:limit]
	}
	return findings, nil
}

func (s *Store) UpdateFindingState(id string, state model.FindingState, updatedAt time.Time) (model.Finding, error) {
	if id == "" {
		return model.Finding{}, store.ErrInvalidKey
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	finding, ok := s.state.Findings[id]
	if !ok {
		return model.Finding{}, store.ErrNotFound
	}
	finding.State = state
	finding.UpdatedAt = updatedAt
	s.state.Findings[id] = finding
	if err := s.persistLocked(); err != nil {
		return model.Finding{}, err
	}
	return finding, nil
}

func (s *Store) AppendUsage(record model.UsageRecord) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.state.Usage = append(s.state.Usage, record)
	return s.persistLocked()
}

func (s *Store) SummarizeUsage(day time.Time) (model.UsageSummary, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	normalized := model.NormalizeDay(day)
	summary := model.UsageSummary{Day: normalized}
	for _, record := range s.state.Usage {
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
	s.state.Cursors[source] = cursor
	return s.persistLocked()
}

func (s *Store) GetCursor(source string) (string, error) {
	if source == "" {
		return "", store.ErrInvalidKey
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	cursor, ok := s.state.Cursors[source]
	if !ok {
		return "", store.ErrNotFound
	}
	return cursor, nil
}

func (s *Store) load() error {
	if err := os.MkdirAll(filepath.Dir(s.path), 0o755); err != nil {
		return err
	}
	data, err := os.ReadFile(s.path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	if len(data) == 0 {
		return nil
	}
	if err := json.Unmarshal(data, &s.state); err != nil {
		return err
	}
	if s.state.Drafts == nil {
		s.state.Drafts = make(map[string]model.Draft)
	}
	if s.state.Checkpoints == nil {
		s.state.Checkpoints = make(map[string]model.CheckpointDoc)
	}
	if s.state.Reports == nil {
		s.state.Reports = make(map[string]model.DailyReport)
	}
	if s.state.Findings == nil {
		s.state.Findings = make(map[string]model.Finding)
	}
	if s.state.Cursors == nil {
		s.state.Cursors = make(map[string]string)
	}
	return nil
}

func (s *Store) persistLocked() error {
	data, err := json.MarshalIndent(s.state, "", "  ")
	if err != nil {
		return err
	}
	_, err = vault.WriteFileAtomic(s.path, data, s.tempSuffix)
	return err
}

func reportKey(agentID string, day time.Time) string {
	return agentID + "|" + model.NormalizeDay(day).Format("2006-01-02")
}
