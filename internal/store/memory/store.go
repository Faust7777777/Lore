package memory

import (
	"sort"
	"sync"
	"time"

	"obsidian-harness/internal/model"
	"obsidian-harness/internal/persona"
	"obsidian-harness/internal/store"
)

type Store struct {
	mu          sync.RWMutex
	drafts      map[string]model.Draft
	checkpoints map[string]model.CheckpointDoc
	reports     map[string]model.DailyReport
	audit       []model.AuditRecord
	findings    map[string]model.Finding
	usage       []model.UsageRecord
	cursors     map[string]string
	// Persona candidates are keyed by ID for direct lookup and by
	// DedupKey for collision detection on Upsert. Both maps hold the
	// same record values; the dedup index lets Upsert short-circuit
	// without scanning the whole set.
	personaCandidates       map[string]persona.PersonaCandidateRecord
	personaCandidatesByKey  map[string]string
}

func New() *Store {
	return &Store{
		drafts:                 make(map[string]model.Draft),
		checkpoints:            make(map[string]model.CheckpointDoc),
		reports:                make(map[string]model.DailyReport),
		findings:               make(map[string]model.Finding),
		cursors:                make(map[string]string),
		personaCandidates:      make(map[string]persona.PersonaCandidateRecord),
		personaCandidatesByKey: make(map[string]string),
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

func (s *Store) Findings() store.FindingStore {
	return s
}

func (s *Store) Usage() store.UsageStore {
	return s
}

func (s *Store) Cursors() store.CursorStore {
	return s
}

func (s *Store) PersonaCandidates() store.PersonaCandidateStore {
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

func (s *Store) SupersedeDraft(oldID string, newDraft model.Draft, updatedAt time.Time) (model.Draft, model.Draft, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if oldID == "" || newDraft.ID == "" {
		return model.Draft{}, model.Draft{}, store.ErrInvalidKey
	}
	if oldID == newDraft.ID {
		return model.Draft{}, model.Draft{}, store.ErrConflict
	}
	oldDraft, ok := s.drafts[oldID]
	if !ok {
		return model.Draft{}, model.Draft{}, store.ErrNotFound
	}
	if _, exists := s.drafts[newDraft.ID]; exists {
		return model.Draft{}, model.Draft{}, store.ErrConflict
	}

	oldDraft.State = model.DraftSuperseded
	oldDraft.UpdatedAt = updatedAt
	s.drafts[oldID] = oldDraft
	s.drafts[newDraft.ID] = newDraft
	return oldDraft, newDraft, nil
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
	record = model.NormalizeAuditRecord(record)
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

func (s *Store) SaveFinding(finding model.Finding) error {
	if finding.ID == "" {
		return store.ErrInvalidKey
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.findings[finding.ID] = finding
	return nil
}

func (s *Store) GetFinding(id string) (model.Finding, error) {
	if id == "" {
		return model.Finding{}, store.ErrInvalidKey
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	finding, ok := s.findings[id]
	if !ok {
		return model.Finding{}, store.ErrNotFound
	}
	return finding, nil
}

func (s *Store) ListFindings(limit int) ([]model.Finding, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	findings := make([]model.Finding, 0, len(s.findings))
	for _, finding := range s.findings {
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
	finding, ok := s.findings[id]
	if !ok {
		return model.Finding{}, store.ErrNotFound
	}
	finding.State = state
	finding.UpdatedAt = updatedAt
	s.findings[id] = finding
	return finding, nil
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

	normalized := model.NormalizeUsageDay(day)
	summary := model.UsageSummary{Day: normalized}
	// B-P11a: populate PurposeBreakdown in the same pass. Memory
	// records carry Purpose directly so no extra unmarshal step;
	// records with an empty Purpose collapse into the "" bucket.
	breakdown := map[string]model.UsagePurposeStats{}
	for _, record := range s.usage {
		if !model.NormalizeUsageDay(record.RecordedAt).Equal(normalized) {
			continue
		}
		summary.Calls++
		summary.PromptTokens += record.PromptTokens
		summary.CompletionTokens += record.CompletionTokens
		stats := breakdown[record.Purpose]
		stats.Calls++
		stats.PromptTokens += record.PromptTokens
		stats.CompletionTokens += record.CompletionTokens
		breakdown[record.Purpose] = stats
	}
	summary.TotalTokens = summary.PromptTokens + summary.CompletionTokens
	if len(breakdown) > 0 {
		summary.PurposeBreakdown = breakdown
	}
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

func (s *Store) UpsertCandidate(record persona.PersonaCandidateRecord) (persona.PersonaCandidateRecord, bool, error) {
	if record.ID == "" || record.DedupKey == "" {
		return persona.PersonaCandidateRecord{}, false, store.ErrInvalidKey
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	// ID-collision contract: re-inserting the same ID is idempotent
	// only when the DedupKey matches. Reusing an ID for a different
	// dedup identity would silently overwrite the original record
	// and leave the by-key index pointing at a stale row; reject
	// instead with ErrConflict so the caller can pick a fresh ID.
	if existing, ok := s.personaCandidates[record.ID]; ok {
		if existing.DedupKey == record.DedupKey {
			return existing, false, nil
		}
		return persona.PersonaCandidateRecord{}, false, store.ErrConflict
	}
	if existingID, ok := s.personaCandidatesByKey[record.DedupKey]; ok {
		return s.personaCandidates[existingID], false, nil
	}
	record.State = persona.NormalizeCandidateState(record.State)
	s.personaCandidates[record.ID] = record
	s.personaCandidatesByKey[record.DedupKey] = record.ID
	return record, true, nil
}

func (s *Store) GetCandidate(id string) (persona.PersonaCandidateRecord, error) {
	if id == "" {
		return persona.PersonaCandidateRecord{}, store.ErrInvalidKey
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	record, ok := s.personaCandidates[id]
	if !ok {
		return persona.PersonaCandidateRecord{}, store.ErrNotFound
	}
	return record, nil
}

func (s *Store) ListCandidatesByState(state persona.PersonaCandidateState, limit int) ([]persona.PersonaCandidateRecord, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	wantState := persona.NormalizeCandidateState(state)
	out := make([]persona.PersonaCandidateRecord, 0, len(s.personaCandidates))
	for _, record := range s.personaCandidates {
		if persona.NormalizeCandidateState(record.State) != wantState {
			continue
		}
		out = append(out, record)
	}
	sort.SliceStable(out, func(i, j int) bool {
		return out[i].UpdatedAt.After(out[j].UpdatedAt)
	})
	if limit > 0 && len(out) > limit {
		out = out[:limit]
	}
	return out, nil
}

func (s *Store) UpdateCandidateState(id string, state persona.PersonaCandidateState, updatedAt time.Time) (persona.PersonaCandidateRecord, error) {
	if id == "" {
		return persona.PersonaCandidateRecord{}, store.ErrInvalidKey
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	record, ok := s.personaCandidates[id]
	if !ok {
		return persona.PersonaCandidateRecord{}, store.ErrNotFound
	}
	record.State = persona.NormalizeCandidateState(state)
	record.UpdatedAt = updatedAt
	s.personaCandidates[id] = record
	return record, nil
}

func (s *Store) LinkCandidateDraft(id string, draftID string, updatedAt time.Time) (persona.PersonaCandidateRecord, error) {
	if id == "" || draftID == "" {
		return persona.PersonaCandidateRecord{}, store.ErrInvalidKey
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	record, ok := s.personaCandidates[id]
	if !ok {
		return persona.PersonaCandidateRecord{}, store.ErrNotFound
	}
	record.State = persona.PersonaCandidateDrafted
	record.DraftID = draftID
	record.UpdatedAt = updatedAt
	s.personaCandidates[id] = record
	return record, nil
}

func (s *Store) ClaimCandidateForDraft(id string, now time.Time) (persona.PersonaCandidateRecord, error) {
	if id == "" {
		return persona.PersonaCandidateRecord{}, store.ErrInvalidKey
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	record, ok := s.personaCandidates[id]
	if !ok {
		return persona.PersonaCandidateRecord{}, store.ErrNotFound
	}
	if persona.NormalizeCandidateState(record.State) != persona.PersonaCandidateOpen {
		return persona.PersonaCandidateRecord{}, store.ErrConflict
	}
	record.State = persona.PersonaCandidateDrafted
	record.DraftID = ""
	record.UpdatedAt = now
	s.personaCandidates[id] = record
	return record, nil
}
