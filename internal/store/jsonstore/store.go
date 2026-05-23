package jsonstore

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"time"

	"obsidian-harness/internal/model"
	"obsidian-harness/internal/persona"
	"obsidian-harness/internal/store"
	"obsidian-harness/internal/vault"
)

type persistedState struct {
	Drafts            map[string]model.Draft                    `json:"drafts"`
	Checkpoints       map[string]model.CheckpointDoc            `json:"checkpoints"`
	Reports           map[string]model.DailyReport              `json:"reports"`
	Audit             []model.AuditRecord                       `json:"audit"`
	Findings          map[string]model.Finding                  `json:"findings"`
	Usage             []model.UsageRecord                       `json:"usage"`
	Cursors           map[string]string                         `json:"cursors"`
	PersonaCandidates map[string]persona.PersonaCandidateRecord `json:"persona_candidates,omitempty"`
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
			Drafts:            make(map[string]model.Draft),
			Checkpoints:       make(map[string]model.CheckpointDoc),
			Reports:           make(map[string]model.DailyReport),
			Findings:          make(map[string]model.Finding),
			Cursors:           make(map[string]string),
			PersonaCandidates: make(map[string]persona.PersonaCandidateRecord),
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

func (s *Store) PersonaCandidates() store.PersonaCandidateStore {
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
	// Architect-flagged P0: validate Finding state transitions
	// rather than overwriting silently. Mirrors the memory and
	// sqlite backends so all three behave identically.
	if err := model.ValidateFindingTransition(finding.State, state); err != nil {
		return model.Finding{}, err
	}
	previous := finding
	finding.State = state
	finding.UpdatedAt = updatedAt
	s.state.Findings[id] = finding
	if err := s.persistLocked(); err != nil {
		s.state.Findings[id] = previous
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

	normalized := model.NormalizeUsageDay(day)
	summary := model.UsageSummary{Day: normalized}
	// B-P11a: populate PurposeBreakdown in the same pass so the JSON
	// store mirrors the sqlite + memory implementations. Records
	// pre-dating the Purpose field deserialize with Purpose == ""
	// and collapse into that bucket.
	breakdown := map[string]model.UsagePurposeStats{}
	for _, record := range s.state.Usage {
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
	if s.state.PersonaCandidates == nil {
		// Legacy stores predate persona candidates; populate the map
		// so subsequent UpsertCandidate calls do not need a nil check.
		s.state.PersonaCandidates = make(map[string]persona.PersonaCandidateRecord)
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

func (s *Store) UpsertCandidate(record persona.PersonaCandidateRecord) (persona.PersonaCandidateRecord, bool, error) {
	if record.ID == "" || record.DedupKey == "" {
		return persona.PersonaCandidateRecord{}, false, store.ErrInvalidKey
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	// ID-collision contract: re-inserting the same ID is idempotent
	// only when the DedupKey matches. Reusing an ID for a different
	// dedup identity would silently overwrite the original record;
	// reject instead with ErrConflict so the caller can pick a fresh
	// ID. Same contract enforced across all three backends.
	if existing, ok := s.state.PersonaCandidates[record.ID]; ok {
		if existing.DedupKey == record.DedupKey {
			return existing, false, nil
		}
		return persona.PersonaCandidateRecord{}, false, store.ErrConflict
	}
	for _, existing := range s.state.PersonaCandidates {
		if existing.DedupKey == record.DedupKey {
			return existing, false, nil
		}
	}
	record.State = persona.NormalizeCandidateState(record.State)
	s.state.PersonaCandidates[record.ID] = record
	if err := s.persistLocked(); err != nil {
		// Roll back the in-memory insertion on persist failure so a
		// subsequent retry can succeed without the dedup index
		// blocking it. Failure here is rare (disk full) but ignoring
		// it would leave the store in a state where the candidate is
		// visible in-process but lost on restart.
		delete(s.state.PersonaCandidates, record.ID)
		return persona.PersonaCandidateRecord{}, false, err
	}
	return record, true, nil
}

func (s *Store) GetCandidate(id string) (persona.PersonaCandidateRecord, error) {
	if id == "" {
		return persona.PersonaCandidateRecord{}, store.ErrInvalidKey
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	record, ok := s.state.PersonaCandidates[id]
	if !ok {
		return persona.PersonaCandidateRecord{}, store.ErrNotFound
	}
	return record, nil
}

func (s *Store) ListCandidatesByState(state persona.PersonaCandidateState, limit int) ([]persona.PersonaCandidateRecord, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	wantState := persona.NormalizeCandidateState(state)
	out := make([]persona.PersonaCandidateRecord, 0, len(s.state.PersonaCandidates))
	for _, record := range s.state.PersonaCandidates {
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
	record, ok := s.state.PersonaCandidates[id]
	if !ok {
		return persona.PersonaCandidateRecord{}, store.ErrNotFound
	}
	previous := record
	record.State = persona.NormalizeCandidateState(state)
	record.UpdatedAt = updatedAt
	s.state.PersonaCandidates[id] = record
	if err := s.persistLocked(); err != nil {
		s.state.PersonaCandidates[id] = previous
		return persona.PersonaCandidateRecord{}, err
	}
	return record, nil
}

func (s *Store) LinkCandidateDraft(id string, draftID string, updatedAt time.Time) (persona.PersonaCandidateRecord, error) {
	if id == "" || draftID == "" {
		return persona.PersonaCandidateRecord{}, store.ErrInvalidKey
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	record, ok := s.state.PersonaCandidates[id]
	if !ok {
		return persona.PersonaCandidateRecord{}, store.ErrNotFound
	}
	previous := record
	record.State = persona.PersonaCandidateDrafted
	record.DraftID = draftID
	record.UpdatedAt = updatedAt
	s.state.PersonaCandidates[id] = record
	if err := s.persistLocked(); err != nil {
		s.state.PersonaCandidates[id] = previous
		return persona.PersonaCandidateRecord{}, err
	}
	return record, nil
}

func (s *Store) ClaimCandidateForDraft(id string, now time.Time) (persona.PersonaCandidateRecord, error) {
	if id == "" {
		return persona.PersonaCandidateRecord{}, store.ErrInvalidKey
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	record, ok := s.state.PersonaCandidates[id]
	if !ok {
		return persona.PersonaCandidateRecord{}, store.ErrNotFound
	}
	if persona.NormalizeCandidateState(record.State) != persona.PersonaCandidateOpen {
		return persona.PersonaCandidateRecord{}, store.ErrConflict
	}
	previous := record
	record.State = persona.PersonaCandidateDrafted
	record.DraftID = ""
	record.UpdatedAt = now
	s.state.PersonaCandidates[id] = record
	if err := s.persistLocked(); err != nil {
		s.state.PersonaCandidates[id] = previous
		return persona.PersonaCandidateRecord{}, err
	}
	return record, nil
}

func (s *Store) ClaimCandidateForRetry(id string, expectedDraftID string, now time.Time) (persona.PersonaCandidateRecord, error) {
	if id == "" || expectedDraftID == "" {
		return persona.PersonaCandidateRecord{}, store.ErrInvalidKey
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	record, ok := s.state.PersonaCandidates[id]
	if !ok {
		return persona.PersonaCandidateRecord{}, store.ErrNotFound
	}
	if record.State != persona.PersonaCandidateDrafted || record.DraftID != expectedDraftID {
		return persona.PersonaCandidateRecord{}, store.ErrConflict
	}
	previous := record
	record.DraftID = ""
	record.UpdatedAt = now
	s.state.PersonaCandidates[id] = record
	if err := s.persistLocked(); err != nil {
		s.state.PersonaCandidates[id] = previous
		return persona.PersonaCandidateRecord{}, err
	}
	return record, nil
}

func reportKey(agentID string, day time.Time) string {
	return agentID + "|" + model.NormalizeDay(day).Format("2006-01-02")
}
