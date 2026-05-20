package sqlitestore

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"obsidian-harness/internal/model"
	"obsidian-harness/internal/persona"
	"obsidian-harness/internal/store"

	_ "modernc.org/sqlite"
)

type legacyJSONState struct {
	Drafts      map[string]model.Draft         `json:"drafts"`
	Checkpoints map[string]model.CheckpointDoc `json:"checkpoints"`
	Reports     map[string]model.DailyReport   `json:"reports"`
	Audit       []model.AuditRecord            `json:"audit"`
	Findings    map[string]model.Finding       `json:"findings"`
	Usage       []model.UsageRecord            `json:"usage"`
	Cursors     map[string]string              `json:"cursors"`
}

type Store struct {
	path string
	db   *sql.DB
}

func New(path string) (*Store, error) {
	if strings.TrimSpace(path) == "" {
		return nil, fmt.Errorf("sqlitestore: path is required")
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, err
	}

	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)

	store := &Store{
		path: path,
		db:   db,
	}
	if err := store.init(); err != nil {
		_ = db.Close()
		return nil, err
	}
	return store, nil
}

func OpenWithJSONMigration(path string, legacyJSONPath string) (*Store, error) {
	store, err := New(path)
	if err != nil {
		return nil, err
	}
	if err := store.migrateLegacyJSONIfNeeded(legacyJSONPath); err != nil {
		_ = store.Close()
		return nil, err
	}
	return store, nil
}

func (s *Store) Close() error {
	if s == nil || s.db == nil {
		return nil
	}
	return s.db.Close()
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
	payload, err := marshalPayload(draft)
	if err != nil {
		return err
	}
	_, err = s.db.Exec(
		`INSERT INTO drafts (id, state, updated_at, payload)
		 VALUES (?, ?, ?, ?)
		 ON CONFLICT(id) DO UPDATE SET
		   state = excluded.state,
		   updated_at = excluded.updated_at,
		   payload = excluded.payload`,
		draft.ID,
		string(draft.State),
		timeString(draft.UpdatedAt),
		payload,
	)
	return err
}

func (s *Store) GetDraft(id string) (model.Draft, error) {
	var payload string
	err := s.db.QueryRow(`SELECT payload FROM drafts WHERE id = ?`, id).Scan(&payload)
	if err != nil {
		return model.Draft{}, mapSQLError(err)
	}
	return unmarshalPayload[model.Draft](payload)
}

func (s *Store) ListDrafts() ([]model.Draft, error) {
	rows, err := s.db.Query(`SELECT payload FROM drafts ORDER BY updated_at DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	return decodePayloadRows[model.Draft](rows)
}

func (s *Store) UpdateDraftState(id string, state model.DraftState, updatedAt time.Time) (model.Draft, error) {
	draft, err := s.GetDraft(id)
	if err != nil {
		return model.Draft{}, err
	}
	draft.State = state
	draft.UpdatedAt = updatedAt
	if err := s.SaveDraft(draft); err != nil {
		return model.Draft{}, err
	}
	return draft, nil
}

func (s *Store) SupersedeDraft(oldID string, newDraft model.Draft, updatedAt time.Time) (model.Draft, model.Draft, error) {
	if strings.TrimSpace(oldID) == "" || strings.TrimSpace(newDraft.ID) == "" {
		return model.Draft{}, model.Draft{}, store.ErrInvalidKey
	}
	if oldID == newDraft.ID {
		return model.Draft{}, model.Draft{}, store.ErrConflict
	}

	tx, err := s.db.Begin()
	if err != nil {
		return model.Draft{}, model.Draft{}, err
	}
	defer func() {
		if tx != nil {
			_ = tx.Rollback()
		}
	}()

	oldDraft, err := getDraftTx(tx, oldID)
	if err != nil {
		return model.Draft{}, model.Draft{}, err
	}
	if _, err := getDraftTx(tx, newDraft.ID); err == nil {
		return model.Draft{}, model.Draft{}, store.ErrConflict
	} else if !errors.Is(err, store.ErrNotFound) {
		return model.Draft{}, model.Draft{}, err
	}

	oldDraft.State = model.DraftSuperseded
	oldDraft.UpdatedAt = updatedAt
	if err := saveDraftTx(tx, oldDraft); err != nil {
		return model.Draft{}, model.Draft{}, err
	}
	if err := saveDraftTx(tx, newDraft); err != nil {
		return model.Draft{}, model.Draft{}, err
	}
	if err := tx.Commit(); err != nil {
		return model.Draft{}, model.Draft{}, err
	}
	tx = nil
	return oldDraft, newDraft, nil
}

func (s *Store) SaveCheckpoint(doc model.CheckpointDoc) error {
	payload, err := marshalPayload(doc)
	if err != nil {
		return err
	}
	_, err = s.db.Exec(
		`INSERT INTO checkpoints (window_key, agent_id, day, window_start, payload)
		 VALUES (?, ?, ?, ?, ?)
		 ON CONFLICT(window_key) DO UPDATE SET
		   agent_id = excluded.agent_id,
		   day = excluded.day,
		   window_start = excluded.window_start,
		   payload = excluded.payload`,
		doc.WindowKey,
		doc.Window.AgentID,
		dayString(doc.Window.WindowStart),
		timeString(doc.Window.WindowStart),
		payload,
	)
	return err
}

func (s *Store) GetCheckpointByWindowKey(windowKey string) (model.CheckpointDoc, error) {
	var payload string
	err := s.db.QueryRow(`SELECT payload FROM checkpoints WHERE window_key = ?`, windowKey).Scan(&payload)
	if err != nil {
		return model.CheckpointDoc{}, mapSQLError(err)
	}
	return unmarshalPayload[model.CheckpointDoc](payload)
}

func (s *Store) ListCheckpointsByDay(agentID string, day time.Time) ([]model.CheckpointDoc, error) {
	rows, err := s.db.Query(
		`SELECT payload
		   FROM checkpoints
		  WHERE agent_id = ? AND day = ?
		  ORDER BY window_start ASC`,
		agentID,
		dayString(day),
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	return decodePayloadRows[model.CheckpointDoc](rows)
}

func (s *Store) SaveDailyReport(report model.DailyReport) error {
	payload, err := marshalPayload(report)
	if err != nil {
		return err
	}
	_, err = s.db.Exec(
		`INSERT INTO daily_reports (report_key, agent_id, day, payload)
		 VALUES (?, ?, ?, ?)
		 ON CONFLICT(report_key) DO UPDATE SET
		   agent_id = excluded.agent_id,
		   day = excluded.day,
		   payload = excluded.payload`,
		reportKey(report.AgentID, report.ReportDay),
		report.AgentID,
		dayString(report.ReportDay),
		payload,
	)
	return err
}

func (s *Store) GetDailyReport(agentID string, day time.Time) (model.DailyReport, error) {
	var payload string
	err := s.db.QueryRow(`SELECT payload FROM daily_reports WHERE report_key = ?`, reportKey(agentID, day)).Scan(&payload)
	if err != nil {
		return model.DailyReport{}, mapSQLError(err)
	}
	return unmarshalPayload[model.DailyReport](payload)
}

func (s *Store) AppendAudit(record model.AuditRecord) error {
	record = model.NormalizeAuditRecord(record)
	payload, err := marshalPayload(record)
	if err != nil {
		return err
	}
	_, err = s.db.Exec(
		`INSERT INTO audit_records (occurred_at, payload) VALUES (?, ?)`,
		timeString(record.OccurredAt),
		payload,
	)
	return err
}

func (s *Store) ListAudit(limit int) ([]model.AuditRecord, error) {
	query := `SELECT payload FROM audit_records ORDER BY seq ASC`
	args := []any{}
	if limit > 0 {
		query = `SELECT payload
		           FROM (
		                 SELECT seq, payload
		                   FROM audit_records
		                  ORDER BY seq DESC
		                  LIMIT ?
		                )
		          ORDER BY seq ASC`
		args = append(args, limit)
	}

	rows, err := s.db.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	return decodePayloadRows[model.AuditRecord](rows)
}

func (s *Store) SaveFinding(finding model.Finding) error {
	if strings.TrimSpace(finding.ID) == "" {
		return store.ErrInvalidKey
	}
	payload, err := marshalPayload(finding)
	if err != nil {
		return err
	}
	_, err = s.db.Exec(
		`INSERT INTO findings (id, state, updated_at, detected_at, payload)
		 VALUES (?, ?, ?, ?, ?)
		 ON CONFLICT(id) DO UPDATE SET
		   state = excluded.state,
		   updated_at = excluded.updated_at,
		   detected_at = excluded.detected_at,
		   payload = excluded.payload`,
		finding.ID,
		string(finding.State),
		timeString(finding.UpdatedAt),
		timeString(finding.DetectedAt),
		payload,
	)
	return err
}

func (s *Store) GetFinding(id string) (model.Finding, error) {
	if strings.TrimSpace(id) == "" {
		return model.Finding{}, store.ErrInvalidKey
	}
	var payload string
	err := s.db.QueryRow(`SELECT payload FROM findings WHERE id = ?`, id).Scan(&payload)
	if err != nil {
		return model.Finding{}, mapSQLError(err)
	}
	return unmarshalPayload[model.Finding](payload)
}

func (s *Store) ListFindings(limit int) ([]model.Finding, error) {
	query := `SELECT payload FROM findings ORDER BY updated_at DESC, id ASC`
	args := []any{}
	if limit > 0 {
		query = `SELECT payload FROM findings ORDER BY updated_at DESC, id ASC LIMIT ?`
		args = append(args, limit)
	}

	rows, err := s.db.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	return decodePayloadRows[model.Finding](rows)
}

func (s *Store) UpdateFindingState(id string, state model.FindingState, updatedAt time.Time) (model.Finding, error) {
	finding, err := s.GetFinding(id)
	if err != nil {
		return model.Finding{}, err
	}
	finding.State = state
	finding.UpdatedAt = updatedAt
	if err := s.SaveFinding(finding); err != nil {
		return model.Finding{}, err
	}
	return finding, nil
}

func (s *Store) AppendUsage(record model.UsageRecord) error {
	payload, err := marshalPayload(record)
	if err != nil {
		return err
	}
	_, err = s.db.Exec(
		`INSERT INTO usage_records (day, recorded_at, prompt_tokens, completion_tokens, payload)
		 VALUES (?, ?, ?, ?, ?)`,
		usageDayString(record.RecordedAt),
		timeString(record.RecordedAt),
		record.PromptTokens,
		record.CompletionTokens,
		payload,
	)
	return err
}

func (s *Store) SummarizeUsage(day time.Time) (model.UsageSummary, error) {
	summary := model.UsageSummary{Day: model.NormalizeUsageDay(day)}
	err := s.db.QueryRow(
		`SELECT COUNT(*),
		        COALESCE(SUM(prompt_tokens), 0),
		        COALESCE(SUM(completion_tokens), 0)
		   FROM usage_records
		  WHERE day = ?`,
		usageDayString(day),
	).Scan(&summary.Calls, &summary.PromptTokens, &summary.CompletionTokens)
	if err != nil {
		return model.UsageSummary{}, err
	}
	summary.TotalTokens = summary.PromptTokens + summary.CompletionTokens
	return summary, nil
}

func (s *Store) SaveCursor(source string, cursor string) error {
	if strings.TrimSpace(source) == "" {
		return store.ErrInvalidKey
	}
	_, err := s.db.Exec(
		`INSERT INTO cursors (source, cursor)
		 VALUES (?, ?)
		 ON CONFLICT(source) DO UPDATE SET cursor = excluded.cursor`,
		source,
		cursor,
	)
	return err
}

func (s *Store) GetCursor(source string) (string, error) {
	if strings.TrimSpace(source) == "" {
		return "", store.ErrInvalidKey
	}
	var cursor string
	err := s.db.QueryRow(`SELECT cursor FROM cursors WHERE source = ?`, source).Scan(&cursor)
	if err != nil {
		return "", mapSQLError(err)
	}
	return cursor, nil
}

func (s *Store) UpsertCandidate(record persona.PersonaCandidateRecord) (persona.PersonaCandidateRecord, bool, error) {
	if strings.TrimSpace(record.ID) == "" || strings.TrimSpace(record.DedupKey) == "" {
		return persona.PersonaCandidateRecord{}, false, store.ErrInvalidKey
	}
	// ID-collision pre-check: PRIMARY KEY would error on insert, but
	// the contract distinguishes "same ID + same DedupKey" (idempotent
	// retry, return existing) from "same ID + different DedupKey"
	// (ErrConflict, caller picked a colliding ID for a different
	// dedup identity). Doing the check in-process keeps the error
	// semantics consistent across memory / json / sqlite.
	if existing, err := s.GetCandidate(record.ID); err == nil {
		if existing.DedupKey == record.DedupKey {
			return existing, false, nil
		}
		return persona.PersonaCandidateRecord{}, false, store.ErrConflict
	} else if !errors.Is(err, store.ErrNotFound) {
		return persona.PersonaCandidateRecord{}, false, err
	}
	record.State = persona.NormalizeCandidateState(record.State)
	payload, err := marshalPayload(record)
	if err != nil {
		return persona.PersonaCandidateRecord{}, false, err
	}
	// Optimistic insert using ON CONFLICT(dedup_key) DO NOTHING.
	// sqlite reports zero rows affected when the dedup key already
	// exists; in that case we look up and return the existing row.
	result, err := s.db.Exec(
		`INSERT INTO persona_candidates (id, state, dedup_key, created_at, updated_at, observed_at, payload)
		 VALUES (?, ?, ?, ?, ?, ?, ?)
		 ON CONFLICT(dedup_key) DO NOTHING`,
		record.ID,
		string(record.State),
		record.DedupKey,
		timeString(record.CreatedAt),
		timeString(record.UpdatedAt),
		timeString(record.Candidate.ObservedAt),
		payload,
	)
	if err != nil {
		return persona.PersonaCandidateRecord{}, false, err
	}
	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return persona.PersonaCandidateRecord{}, false, err
	}
	if rowsAffected == 1 {
		return record, true, nil
	}
	existing, err := s.getCandidateByDedupKey(record.DedupKey)
	if err != nil {
		return persona.PersonaCandidateRecord{}, false, err
	}
	return existing, false, nil
}

func (s *Store) GetCandidate(id string) (persona.PersonaCandidateRecord, error) {
	if strings.TrimSpace(id) == "" {
		return persona.PersonaCandidateRecord{}, store.ErrInvalidKey
	}
	var payload string
	err := s.db.QueryRow(`SELECT payload FROM persona_candidates WHERE id = ?`, id).Scan(&payload)
	if err != nil {
		return persona.PersonaCandidateRecord{}, mapSQLError(err)
	}
	return unmarshalPayload[persona.PersonaCandidateRecord](payload)
}

func (s *Store) getCandidateByDedupKey(key string) (persona.PersonaCandidateRecord, error) {
	var payload string
	err := s.db.QueryRow(`SELECT payload FROM persona_candidates WHERE dedup_key = ?`, key).Scan(&payload)
	if err != nil {
		return persona.PersonaCandidateRecord{}, mapSQLError(err)
	}
	return unmarshalPayload[persona.PersonaCandidateRecord](payload)
}

func (s *Store) ListCandidatesByState(state persona.PersonaCandidateState, limit int) ([]persona.PersonaCandidateRecord, error) {
	wantState := persona.NormalizeCandidateState(state)
	// When the caller asks for the Open queue, also surface any
	// legacy rows whose stored state column is empty -- those should
	// behave as Open per NormalizeCandidateState. Upsert normalizes
	// on the write side too, so this branch only matters for rows
	// written by older code paths or direct DB manipulation.
	query := `SELECT payload FROM persona_candidates WHERE state = ? ORDER BY updated_at DESC`
	args := []any{string(wantState)}
	if wantState == persona.PersonaCandidateOpen {
		query = `SELECT payload FROM persona_candidates WHERE state = ? OR state = '' ORDER BY updated_at DESC`
	}
	if limit > 0 {
		query += ` LIMIT ?`
		args = append(args, limit)
	}
	rows, err := s.db.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return decodePayloadRows[persona.PersonaCandidateRecord](rows)
}

func (s *Store) UpdateCandidateState(id string, state persona.PersonaCandidateState, updatedAt time.Time) (persona.PersonaCandidateRecord, error) {
	if strings.TrimSpace(id) == "" {
		return persona.PersonaCandidateRecord{}, store.ErrInvalidKey
	}
	record, err := s.GetCandidate(id)
	if err != nil {
		return persona.PersonaCandidateRecord{}, err
	}
	record.State = persona.NormalizeCandidateState(state)
	record.UpdatedAt = updatedAt
	payload, err := marshalPayload(record)
	if err != nil {
		return persona.PersonaCandidateRecord{}, err
	}
	if _, err := s.db.Exec(
		`UPDATE persona_candidates SET state = ?, updated_at = ?, payload = ? WHERE id = ?`,
		string(record.State),
		timeString(updatedAt),
		payload,
		id,
	); err != nil {
		return persona.PersonaCandidateRecord{}, err
	}
	return record, nil
}

func (s *Store) init() error {
	stmts := []string{
		`PRAGMA journal_mode = WAL`,
		`PRAGMA busy_timeout = 5000`,
		`PRAGMA foreign_keys = ON`,
		`CREATE TABLE IF NOT EXISTS drafts (
			id TEXT PRIMARY KEY,
			state TEXT NOT NULL,
			updated_at TEXT NOT NULL,
			payload TEXT NOT NULL
		)`,
		`CREATE INDEX IF NOT EXISTS drafts_updated_at_idx ON drafts(updated_at DESC)`,
		`CREATE TABLE IF NOT EXISTS checkpoints (
			window_key TEXT PRIMARY KEY,
			agent_id TEXT NOT NULL,
			day TEXT NOT NULL,
			window_start TEXT NOT NULL,
			payload TEXT NOT NULL
		)`,
		`CREATE INDEX IF NOT EXISTS checkpoints_agent_day_idx ON checkpoints(agent_id, day, window_start)`,
		`CREATE TABLE IF NOT EXISTS daily_reports (
			report_key TEXT PRIMARY KEY,
			agent_id TEXT NOT NULL,
			day TEXT NOT NULL,
			payload TEXT NOT NULL
		)`,
		`CREATE INDEX IF NOT EXISTS daily_reports_agent_day_idx ON daily_reports(agent_id, day)`,
		`CREATE TABLE IF NOT EXISTS audit_records (
			seq INTEGER PRIMARY KEY AUTOINCREMENT,
			occurred_at TEXT NOT NULL,
			payload TEXT NOT NULL
		)`,
		`CREATE INDEX IF NOT EXISTS audit_records_seq_idx ON audit_records(seq)`,
		`CREATE TABLE IF NOT EXISTS findings (
			id TEXT PRIMARY KEY,
			state TEXT NOT NULL,
			updated_at TEXT NOT NULL,
			detected_at TEXT NOT NULL,
			payload TEXT NOT NULL
		)`,
		`CREATE INDEX IF NOT EXISTS findings_updated_at_idx ON findings(updated_at DESC)`,
		`CREATE TABLE IF NOT EXISTS usage_records (
			seq INTEGER PRIMARY KEY AUTOINCREMENT,
			day TEXT NOT NULL,
			recorded_at TEXT NOT NULL,
			prompt_tokens INTEGER NOT NULL,
			completion_tokens INTEGER NOT NULL,
			payload TEXT NOT NULL
		)`,
		`CREATE INDEX IF NOT EXISTS usage_records_day_idx ON usage_records(day, recorded_at)`,
		`CREATE TABLE IF NOT EXISTS cursors (
			source TEXT PRIMARY KEY,
			cursor TEXT NOT NULL
		)`,
		// persona_candidates stores LLM-mined persona update
		// candidates (see internal/persona). dedup_key is UNIQUE so a
		// duplicate UpsertCandidate returns the existing row instead
		// of inserting. observed_at is exposed as its own column so
		// future queries can range over time without parsing payload.
		`CREATE TABLE IF NOT EXISTS persona_candidates (
			id TEXT PRIMARY KEY,
			state TEXT NOT NULL,
			dedup_key TEXT NOT NULL UNIQUE,
			created_at TEXT NOT NULL,
			updated_at TEXT NOT NULL,
			observed_at TEXT NOT NULL,
			payload TEXT NOT NULL
		)`,
		`CREATE INDEX IF NOT EXISTS persona_candidates_state_updated_at_idx ON persona_candidates(state, updated_at DESC)`,
	}
	for _, stmt := range stmts {
		if _, err := s.db.Exec(stmt); err != nil {
			return err
		}
	}
	return nil
}

func (s *Store) migrateLegacyJSONIfNeeded(legacyJSONPath string) error {
	if strings.TrimSpace(legacyJSONPath) == "" {
		return nil
	}
	empty, err := s.isEmpty()
	if err != nil || !empty {
		return err
	}

	data, err := os.ReadFile(legacyJSONPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	if len(data) == 0 {
		return nil
	}

	var state legacyJSONState
	if err := json.Unmarshal(data, &state); err != nil {
		return err
	}

	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer func() {
		if tx != nil {
			_ = tx.Rollback()
		}
	}()

	for _, draft := range state.Drafts {
		if err := saveDraftTx(tx, draft); err != nil {
			return err
		}
	}
	for _, checkpoint := range state.Checkpoints {
		if err := saveCheckpointTx(tx, checkpoint); err != nil {
			return err
		}
	}
	for _, report := range state.Reports {
		if err := saveDailyReportTx(tx, report); err != nil {
			return err
		}
	}
	for _, record := range state.Audit {
		if err := appendAuditTx(tx, record); err != nil {
			return err
		}
	}
	for _, finding := range state.Findings {
		if err := saveFindingTx(tx, finding); err != nil {
			return err
		}
	}
	for _, record := range state.Usage {
		if err := appendUsageTx(tx, record); err != nil {
			return err
		}
	}
	for source, cursor := range state.Cursors {
		if strings.TrimSpace(source) == "" {
			continue
		}
		if _, err := tx.Exec(
			`INSERT INTO cursors (source, cursor)
			 VALUES (?, ?)
			 ON CONFLICT(source) DO UPDATE SET cursor = excluded.cursor`,
			source,
			cursor,
		); err != nil {
			return err
		}
	}

	if err := tx.Commit(); err != nil {
		return err
	}
	tx = nil
	return nil
}

func (s *Store) isEmpty() (bool, error) {
	// Every persisted table participates in the migration guard:
	// OpenWithJSONMigration only imports legacy JSON when the sqlite
	// DB is empty across ALL tables. Forgetting a table here means a
	// DB whose only content is in that table would be falsely treated
	// as empty, and legacy JSON would clobber its peers. New tables
	// added in later slices must be appended below.
	for _, table := range []string{"drafts", "checkpoints", "daily_reports", "audit_records", "findings", "usage_records", "cursors", "persona_candidates"} {
		var count int
		if err := s.db.QueryRow(`SELECT COUNT(*) FROM ` + table).Scan(&count); err != nil {
			return false, err
		}
		if count > 0 {
			return false, nil
		}
	}
	return true, nil
}

func saveDraftTx(tx *sql.Tx, draft model.Draft) error {
	payload, err := marshalPayload(draft)
	if err != nil {
		return err
	}
	_, err = tx.Exec(
		`INSERT INTO drafts (id, state, updated_at, payload)
		 VALUES (?, ?, ?, ?)
		 ON CONFLICT(id) DO UPDATE SET
		   state = excluded.state,
		   updated_at = excluded.updated_at,
		   payload = excluded.payload`,
		draft.ID,
		string(draft.State),
		timeString(draft.UpdatedAt),
		payload,
	)
	return err
}

func getDraftTx(tx *sql.Tx, id string) (model.Draft, error) {
	var payload string
	err := tx.QueryRow(`SELECT payload FROM drafts WHERE id = ?`, id).Scan(&payload)
	if err != nil {
		return model.Draft{}, mapSQLError(err)
	}
	return unmarshalPayload[model.Draft](payload)
}

func saveCheckpointTx(tx *sql.Tx, doc model.CheckpointDoc) error {
	payload, err := marshalPayload(doc)
	if err != nil {
		return err
	}
	_, err = tx.Exec(
		`INSERT INTO checkpoints (window_key, agent_id, day, window_start, payload)
		 VALUES (?, ?, ?, ?, ?)
		 ON CONFLICT(window_key) DO UPDATE SET
		   agent_id = excluded.agent_id,
		   day = excluded.day,
		   window_start = excluded.window_start,
		   payload = excluded.payload`,
		doc.WindowKey,
		doc.Window.AgentID,
		dayString(doc.Window.WindowStart),
		timeString(doc.Window.WindowStart),
		payload,
	)
	return err
}

func saveDailyReportTx(tx *sql.Tx, report model.DailyReport) error {
	payload, err := marshalPayload(report)
	if err != nil {
		return err
	}
	_, err = tx.Exec(
		`INSERT INTO daily_reports (report_key, agent_id, day, payload)
		 VALUES (?, ?, ?, ?)
		 ON CONFLICT(report_key) DO UPDATE SET
		   agent_id = excluded.agent_id,
		   day = excluded.day,
		   payload = excluded.payload`,
		reportKey(report.AgentID, report.ReportDay),
		report.AgentID,
		dayString(report.ReportDay),
		payload,
	)
	return err
}

func appendAuditTx(tx *sql.Tx, record model.AuditRecord) error {
	record = model.NormalizeAuditRecord(record)
	payload, err := marshalPayload(record)
	if err != nil {
		return err
	}
	_, err = tx.Exec(
		`INSERT INTO audit_records (occurred_at, payload) VALUES (?, ?)`,
		timeString(record.OccurredAt),
		payload,
	)
	return err
}

func saveFindingTx(tx *sql.Tx, finding model.Finding) error {
	if strings.TrimSpace(finding.ID) == "" {
		return store.ErrInvalidKey
	}
	payload, err := marshalPayload(finding)
	if err != nil {
		return err
	}
	_, err = tx.Exec(
		`INSERT INTO findings (id, state, updated_at, detected_at, payload)
		 VALUES (?, ?, ?, ?, ?)
		 ON CONFLICT(id) DO UPDATE SET
		   state = excluded.state,
		   updated_at = excluded.updated_at,
		   detected_at = excluded.detected_at,
		   payload = excluded.payload`,
		finding.ID,
		string(finding.State),
		timeString(finding.UpdatedAt),
		timeString(finding.DetectedAt),
		payload,
	)
	return err
}

func appendUsageTx(tx *sql.Tx, record model.UsageRecord) error {
	payload, err := marshalPayload(record)
	if err != nil {
		return err
	}
	_, err = tx.Exec(
		`INSERT INTO usage_records (day, recorded_at, prompt_tokens, completion_tokens, payload)
		 VALUES (?, ?, ?, ?, ?)`,
		usageDayString(record.RecordedAt),
		timeString(record.RecordedAt),
		record.PromptTokens,
		record.CompletionTokens,
		payload,
	)
	return err
}

func decodePayloadRows[T any](rows *sql.Rows) ([]T, error) {
	items := make([]T, 0)
	for rows.Next() {
		var payload string
		if err := rows.Scan(&payload); err != nil {
			return nil, err
		}
		item, err := unmarshalPayload[T](payload)
		if err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return items, nil
}

func marshalPayload(value any) (string, error) {
	data, err := json.Marshal(value)
	if err != nil {
		return "", err
	}
	return string(data), nil
}

func unmarshalPayload[T any](payload string) (T, error) {
	var value T
	if err := json.Unmarshal([]byte(payload), &value); err != nil {
		return value, err
	}
	return value, nil
}

func mapSQLError(err error) error {
	if err == sql.ErrNoRows {
		return store.ErrNotFound
	}
	return err
}

func reportKey(agentID string, day time.Time) string {
	return agentID + "|" + dayString(day)
}

func dayString(value time.Time) string {
	return model.NormalizeDay(value).Format("2006-01-02")
}

// usageDayString buckets a usage record (or a SummarizeUsage query
// argument) into the local calendar day. operatoragent records
// RecordedAt in UTC while CLI queries arrive in time.Local; if both
// sides used dayString they would disagree near tz boundaries. Keep
// process-sink data on dayString -- its Window times are constructed
// with intentional locations by the codex JSONL importer.
func usageDayString(value time.Time) string {
	return model.NormalizeUsageDay(value).Format("2006-01-02")
}

func timeString(value time.Time) string {
	if value.IsZero() {
		return ""
	}
	return value.UTC().Format(time.RFC3339Nano)
}
