package sqlitestore

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"obsidian-harness/internal/model"
	"obsidian-harness/internal/persona"
)

func TestStorePersistsAcrossReload(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state", "store.db")
	createdAt := time.Date(2026, 4, 22, 10, 0, 0, 0, time.UTC)

	st, err := New(path)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	defer func() { _ = st.Close() }()

	if err := st.SaveDraft(model.Draft{
		ID:    "draft-1",
		Kind:  model.DraftKindProgressSync,
		State: model.DraftPendingReview,
		Target: model.DocumentRef{
			Path:        "progress.md",
			Class:       model.DocClassProgressIndex,
			BaseVersion: "v1",
		},
		Title:           "progress sync",
		Summary:         "sync progress",
		ProposedContent: "content",
		CreatedAt:       createdAt,
		UpdatedAt:       createdAt,
	}); err != nil {
		t.Fatalf("SaveDraft() error = %v", err)
	}

	window := model.SessionWindow{
		AgentID:     "codex",
		SessionID:   "session-1",
		WindowStart: createdAt,
		WindowEnd:   createdAt.Add(30 * time.Minute),
	}
	if err := st.SaveCheckpoint(model.CheckpointDoc{
		WindowKey: window.Key(),
		Path:      "process/codex/checkpoint.md",
		Window:    window,
		State:     model.CheckpointMaterialized,
		Title:     "checkpoint",
		Content:   "summary",
		CreatedAt: createdAt,
		UpdatedAt: createdAt,
	}); err != nil {
		t.Fatalf("SaveCheckpoint() error = %v", err)
	}

	if err := st.SaveDailyReport(model.DailyReport{
		AgentID:    "codex",
		ReportDay:  createdAt,
		Path:       "process/codex/daily/2026-04-22.md",
		WindowKeys: []string{window.Key()},
		Title:      "daily",
		Content:    "report",
		CreatedAt:  createdAt,
		UpdatedAt:  createdAt,
	}); err != nil {
		t.Fatalf("SaveDailyReport() error = %v", err)
	}

	if err := st.AppendAudit(model.AuditRecord{
		ID:         "audit-1",
		Kind:       model.AuditDraftCreated,
		Target:     "progress.md",
		OccurredAt: createdAt,
	}); err != nil {
		t.Fatalf("AppendAudit(first) error = %v", err)
	}
	if err := st.AppendAudit(model.AuditRecord{
		ID:         "audit-2",
		Kind:       model.AuditDraftApplied,
		Target:     "progress.md",
		OccurredAt: createdAt.Add(time.Minute),
	}); err != nil {
		t.Fatalf("AppendAudit(second) error = %v", err)
	}

	if err := st.SaveFinding(model.Finding{
		ID:       "finding-1",
		Kind:     model.FindingOutOfBandVaultWrite,
		State:    model.FindingOpen,
		Severity: model.FindingSeverityInfo,
		Target: model.DocumentRef{
			Path:        "notes/outside.md",
			Class:       model.DocClassNote,
			BaseVersion: "hash-1",
		},
		Title:      "outside write",
		Summary:    "outside write detected",
		Source:     "vault_daemon",
		AuditID:    "audit-2",
		DetectedAt: createdAt,
		UpdatedAt:  createdAt,
	}); err != nil {
		t.Fatalf("SaveFinding() error = %v", err)
	}

	if err := st.AppendUsage(model.UsageRecord{
		RecordedAt:       createdAt,
		PromptTokens:     11,
		CompletionTokens: 7,
	}); err != nil {
		t.Fatalf("AppendUsage() error = %v", err)
	}

	if err := st.SaveCursor("codex:session-1", "cursor-42"); err != nil {
		t.Fatalf("SaveCursor() error = %v", err)
	}

	if err := st.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}

	reloaded, err := New(path)
	if err != nil {
		t.Fatalf("New(reload) error = %v", err)
	}
	defer func() { _ = reloaded.Close() }()

	draft, err := reloaded.GetDraft("draft-1")
	if err != nil {
		t.Fatalf("GetDraft() error = %v", err)
	}
	if draft.Target.BaseVersion != "v1" {
		t.Fatalf("BaseVersion = %q, want v1", draft.Target.BaseVersion)
	}

	checkpoint, err := reloaded.GetCheckpointByWindowKey(window.Key())
	if err != nil {
		t.Fatalf("GetCheckpointByWindowKey() error = %v", err)
	}
	if checkpoint.Path == "" {
		t.Fatal("checkpoint.Path is empty after reload")
	}

	report, err := reloaded.GetDailyReport("codex", createdAt)
	if err != nil {
		t.Fatalf("GetDailyReport() error = %v", err)
	}
	if len(report.WindowKeys) != 1 {
		t.Fatalf("WindowKeys = %d, want 1", len(report.WindowKeys))
	}

	auditRecords, err := reloaded.ListAudit(1)
	if err != nil {
		t.Fatalf("ListAudit() error = %v", err)
	}
	if len(auditRecords) != 1 || auditRecords[0].ID != "audit-2" {
		t.Fatalf("auditRecords = %+v, want last audit record only", auditRecords)
	}

	findings, err := reloaded.ListFindings(1)
	if err != nil {
		t.Fatalf("ListFindings() error = %v", err)
	}
	if len(findings) != 1 || findings[0].ID != "finding-1" {
		t.Fatalf("findings = %+v, want finding-1", findings)
	}

	summary, err := reloaded.SummarizeUsage(createdAt)
	if err != nil {
		t.Fatalf("SummarizeUsage() error = %v", err)
	}
	if summary.Calls != 1 || summary.PromptTokens != 11 || summary.CompletionTokens != 7 || summary.TotalTokens != 18 {
		t.Fatalf("summary = %+v, want 1 call and 18 total tokens", summary)
	}

	cursor, err := reloaded.GetCursor("codex:session-1")
	if err != nil {
		t.Fatalf("GetCursor() error = %v", err)
	}
	if cursor != "cursor-42" {
		t.Fatalf("cursor = %q, want cursor-42", cursor)
	}
}

// TestSummarizeUsageBucketsAcrossUTCBoundary covers the sqlite-side of
// the timezone fix: usage rows are written with usageDayString
// (local-day bucket) and queried with the same function, so a UTC
// record falling on the previous UTC day is still counted against the
// local "today" the CLI uses.
func TestSummarizeUsageBucketsAcrossUTCBoundary(t *testing.T) {
	originalLocal := time.Local
	t.Cleanup(func() { time.Local = originalLocal })
	time.Local = time.FixedZone("CST", 8*3600)

	path := filepath.Join(t.TempDir(), "state", "store.db")
	st, err := New(path)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	t.Cleanup(func() { _ = st.Close() })

	// 2026-05-17 16:30 UTC = 2026-05-18 00:30 +0800 local.
	recordedUTC := time.Date(2026, 5, 17, 16, 30, 0, 0, time.UTC)
	if err := st.AppendUsage(model.UsageRecord{
		Provider: "openai-compatible", Model: "fake",
		AgentID: "codex", SessionID: "s1",
		PromptTokens: 30, CompletionTokens: 4,
		RecordedAt: recordedUTC,
	}); err != nil {
		t.Fatalf("AppendUsage() error = %v", err)
	}

	queryToday := time.Date(2026, 5, 18, 0, 30, 0, 0, time.Local)
	summary, err := st.SummarizeUsage(queryToday)
	if err != nil {
		t.Fatalf("SummarizeUsage(today) error = %v", err)
	}
	if summary.Calls != 1 || summary.PromptTokens != 30 || summary.CompletionTokens != 4 {
		t.Fatalf("today summary = %+v, want 1 call / 30 prompt / 4 completion", summary)
	}

	queryYesterday := time.Date(2026, 5, 17, 12, 0, 0, 0, time.Local)
	prev, err := st.SummarizeUsage(queryYesterday)
	if err != nil {
		t.Fatalf("SummarizeUsage(yesterday) error = %v", err)
	}
	if prev.Calls != 0 {
		t.Fatalf("yesterday summary should be empty, got %+v", prev)
	}
}

func TestOpenWithJSONMigrationImportsLegacyState(t *testing.T) {
	workDir := t.TempDir()
	legacyPath := filepath.Join(workDir, "state", "store.json")
	dbPath := filepath.Join(workDir, "state", "store.db")
	createdAt := time.Date(2026, 4, 22, 10, 0, 0, 0, time.UTC)

	window := model.SessionWindow{
		AgentID:     "codex",
		SessionID:   "session-legacy",
		WindowStart: createdAt,
		WindowEnd:   createdAt.Add(30 * time.Minute),
	}
	legacy := legacyJSONState{
		Drafts: map[string]model.Draft{
			"draft-legacy": {
				ID:    "draft-legacy",
				Kind:  model.DraftKindProgressSync,
				State: model.DraftApproved,
				Target: model.DocumentRef{
					Path:        "progress.md",
					Class:       model.DocClassProgressIndex,
					BaseVersion: "legacy-v1",
				},
				Title:           "legacy draft",
				Summary:         "legacy summary",
				ProposedContent: "legacy content",
				CreatedAt:       createdAt,
				UpdatedAt:       createdAt,
			},
		},
		Checkpoints: map[string]model.CheckpointDoc{
			window.Key(): {
				WindowKey: window.Key(),
				Path:      "process/codex/checkpoint.md",
				Window:    window,
				State:     model.CheckpointMaterialized,
				Title:     "legacy checkpoint",
				Content:   "legacy summary",
				CreatedAt: createdAt,
				UpdatedAt: createdAt,
			},
		},
		Reports: map[string]model.DailyReport{
			reportKey("codex", createdAt): {
				AgentID:    "codex",
				ReportDay:  createdAt,
				Path:       "process/codex/daily/2026-04-22.md",
				WindowKeys: []string{window.Key()},
				Title:      "legacy daily",
				Content:    "legacy daily content",
				CreatedAt:  createdAt,
				UpdatedAt:  createdAt,
			},
		},
		Audit: []model.AuditRecord{{
			ID:         "audit-legacy",
			Kind:       model.AuditDraftCreated,
			Target:     "progress.md",
			OccurredAt: createdAt,
		}},
		Findings: map[string]model.Finding{
			"finding-legacy": {
				ID:       "finding-legacy",
				Kind:     model.FindingGovernanceReviewNeeded,
				State:    model.FindingOpen,
				Severity: model.FindingSeverityWarning,
				Target: model.DocumentRef{
					Path:        "progress.md",
					Class:       model.DocClassProgressIndex,
					BaseVersion: "legacy-hash",
				},
				Title:      "legacy finding",
				Summary:    "legacy finding summary",
				Source:     "vault_daemon",
				DetectedAt: createdAt,
				UpdatedAt:  createdAt,
			},
		},
		Usage: []model.UsageRecord{{
			RecordedAt:       createdAt,
			PromptTokens:     5,
			CompletionTokens: 6,
		}},
		Cursors: map[string]string{
			"codex:legacy": "cursor-legacy",
		},
	}
	if err := os.MkdirAll(filepath.Dir(legacyPath), 0o755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	data, err := json.MarshalIndent(legacy, "", "  ")
	if err != nil {
		t.Fatalf("json.MarshalIndent() error = %v", err)
	}
	if err := os.WriteFile(legacyPath, data, 0o644); err != nil {
		t.Fatalf("WriteFile(legacy) error = %v", err)
	}

	st, err := OpenWithJSONMigration(dbPath, legacyPath)
	if err != nil {
		t.Fatalf("OpenWithJSONMigration() error = %v", err)
	}
	defer func() { _ = st.Close() }()

	draft, err := st.GetDraft("draft-legacy")
	if err != nil {
		t.Fatalf("GetDraft() error = %v", err)
	}
	if draft.Target.BaseVersion != "legacy-v1" {
		t.Fatalf("BaseVersion = %q, want legacy-v1", draft.Target.BaseVersion)
	}

	checkpoint, err := st.GetCheckpointByWindowKey(window.Key())
	if err != nil {
		t.Fatalf("GetCheckpointByWindowKey() error = %v", err)
	}
	if checkpoint.Title != "legacy checkpoint" {
		t.Fatalf("checkpoint.Title = %q, want legacy checkpoint", checkpoint.Title)
	}

	report, err := st.GetDailyReport("codex", createdAt)
	if err != nil {
		t.Fatalf("GetDailyReport() error = %v", err)
	}
	if report.Title != "legacy daily" {
		t.Fatalf("report.Title = %q, want legacy daily", report.Title)
	}

	auditRecords, err := st.ListAudit(10)
	if err != nil {
		t.Fatalf("ListAudit() error = %v", err)
	}
	if len(auditRecords) != 1 || auditRecords[0].ID != "audit-legacy" {
		t.Fatalf("auditRecords = %+v, want legacy audit record", auditRecords)
	}

	findings, err := st.ListFindings(10)
	if err != nil {
		t.Fatalf("ListFindings() error = %v", err)
	}
	if len(findings) != 1 || findings[0].ID != "finding-legacy" {
		t.Fatalf("findings = %+v, want legacy finding", findings)
	}

	summary, err := st.SummarizeUsage(createdAt)
	if err != nil {
		t.Fatalf("SummarizeUsage() error = %v", err)
	}
	if summary.Calls != 1 || summary.TotalTokens != 11 {
		t.Fatalf("summary = %+v, want migrated usage totals", summary)
	}

	cursor, err := st.GetCursor("codex:legacy")
	if err != nil {
		t.Fatalf("GetCursor() error = %v", err)
	}
	if cursor != "cursor-legacy" {
		t.Fatalf("cursor = %q, want cursor-legacy", cursor)
	}
}

// TestOpenWithJSONMigrationBucketsUsageByLocalDay covers the migration
// path's share of the timezone fix: appendUsageTx must use
// usageDayString so that a legacy JSON usage record with UTC RecordedAt
// is bucketed against the user's local day, matching how
// `lore usage` (which queries time.Now() in local) will look it up.
//
// Without this, an Asia/Shanghai user migrating a legacy JSON store
// would silently lose usage that straddled UTC midnight from their
// "today" totals.
func TestOpenWithJSONMigrationBucketsUsageByLocalDay(t *testing.T) {
	originalLocal := time.Local
	t.Cleanup(func() { time.Local = originalLocal })
	time.Local = time.FixedZone("CST", 8*3600)

	workDir := t.TempDir()
	legacyPath := filepath.Join(workDir, "state", "store.json")
	dbPath := filepath.Join(workDir, "state", "store.db")

	// 2026-05-17 16:30 UTC == 2026-05-18 00:30 +0800 local.
	recordedUTC := time.Date(2026, 5, 17, 16, 30, 0, 0, time.UTC)
	legacy := legacyJSONState{
		Usage: []model.UsageRecord{{
			Provider:         "openai-compatible",
			Model:            "fake",
			AgentID:          "codex",
			SessionID:        "s1",
			RecordedAt:       recordedUTC,
			PromptTokens:     30,
			CompletionTokens: 4,
		}},
	}
	if err := os.MkdirAll(filepath.Dir(legacyPath), 0o755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	data, err := json.MarshalIndent(legacy, "", "  ")
	if err != nil {
		t.Fatalf("json.MarshalIndent() error = %v", err)
	}
	if err := os.WriteFile(legacyPath, data, 0o644); err != nil {
		t.Fatalf("WriteFile(legacy) error = %v", err)
	}

	st, err := OpenWithJSONMigration(dbPath, legacyPath)
	if err != nil {
		t.Fatalf("OpenWithJSONMigration() error = %v", err)
	}
	t.Cleanup(func() { _ = st.Close() })

	queryToday := time.Date(2026, 5, 18, 0, 30, 0, 0, time.Local)
	summary, err := st.SummarizeUsage(queryToday)
	if err != nil {
		t.Fatalf("SummarizeUsage(today) error = %v", err)
	}
	if summary.Calls != 1 || summary.PromptTokens != 30 || summary.CompletionTokens != 4 {
		t.Fatalf("migrated today summary = %+v, want 1 call / 30 prompt / 4 completion", summary)
	}

	queryYesterday := time.Date(2026, 5, 17, 12, 0, 0, 0, time.Local)
	prev, err := st.SummarizeUsage(queryYesterday)
	if err != nil {
		t.Fatalf("SummarizeUsage(yesterday) error = %v", err)
	}
	if prev.Calls != 0 {
		t.Fatalf("migrated yesterday summary should be empty, got %+v", prev)
	}
}

// TestOpenWithJSONMigrationSkipsWhenPersonaCandidatesPresent locks the
// migration guard against the regression flagged in the P3 review: if
// the sqlite DB contains ONLY persona candidates (no other tables
// have rows), isEmpty() must still report non-empty so legacy JSON is
// not imported on top. Forgetting persona_candidates in the isEmpty
// table list would let the migration clobber a populated candidate
// queue with stale legacy state.
func TestOpenWithJSONMigrationSkipsWhenPersonaCandidatesPresent(t *testing.T) {
	workDir := t.TempDir()
	legacyPath := filepath.Join(workDir, "state", "store.json")
	dbPath := filepath.Join(workDir, "state", "store.db")

	// Phase 1: seed the sqlite DB with a single persona candidate.
	{
		st, err := New(dbPath)
		if err != nil {
			t.Fatalf("New() error = %v", err)
		}
		candidate := persona.PersonaCandidate{
			Field:           "major",
			ProposedValue:   "economics",
			EvidenceQuote:   "I study economics",
			Confidence:      persona.ConfidenceHigh,
			SourceKind:      persona.SourceConsole,
			SourceSessionID: "lore-session",
			ObservedAt:      time.Date(2026, 5, 20, 10, 0, 0, 0, time.UTC),
		}
		now := time.Date(2026, 5, 20, 10, 0, 0, 0, time.UTC)
		rec := persona.PersonaCandidateRecord{
			ID:        "seeded-1",
			State:     persona.PersonaCandidateOpen,
			DedupKey:  persona.DedupKey(candidate),
			Candidate: candidate,
			CreatedAt: now,
			UpdatedAt: now,
		}
		if _, isNew, err := st.UpsertCandidate(rec); err != nil || !isNew {
			t.Fatalf("seed UpsertCandidate isNew=%v err=%v", isNew, err)
		}
		if err := st.Close(); err != nil {
			t.Fatalf("seed Close() error = %v", err)
		}
	}

	// Phase 2: stage a legacy JSON state that would, if imported,
	// add a draft + a usage row. The migration guard must reject
	// this because the seeded sqlite DB is NOT empty.
	createdAt := time.Date(2026, 4, 22, 10, 0, 0, 0, time.UTC)
	legacy := legacyJSONState{
		Drafts: map[string]model.Draft{
			"draft-legacy": {
				ID:    "draft-legacy",
				Kind:  model.DraftKindProgressSync,
				State: model.DraftApproved,
				Target: model.DocumentRef{
					Path:        "progress.md",
					Class:       model.DocClassProgressIndex,
					BaseVersion: "legacy-v1",
				},
				Title:           "legacy draft",
				Summary:         "should not be imported",
				ProposedContent: "legacy content",
				CreatedAt:       createdAt,
				UpdatedAt:       createdAt,
			},
		},
		Usage: []model.UsageRecord{{
			RecordedAt:       createdAt,
			PromptTokens:     7,
			CompletionTokens: 1,
		}},
	}
	if err := os.MkdirAll(filepath.Dir(legacyPath), 0o755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	data, err := json.MarshalIndent(legacy, "", "  ")
	if err != nil {
		t.Fatalf("json.MarshalIndent() error = %v", err)
	}
	if err := os.WriteFile(legacyPath, data, 0o644); err != nil {
		t.Fatalf("WriteFile(legacy) error = %v", err)
	}

	// Phase 3: reopen with the migration entrypoint and assert the
	// legacy JSON did NOT land in the seeded DB.
	st, err := OpenWithJSONMigration(dbPath, legacyPath)
	if err != nil {
		t.Fatalf("OpenWithJSONMigration() error = %v", err)
	}
	t.Cleanup(func() { _ = st.Close() })

	if _, err := st.GetDraft("draft-legacy"); err == nil {
		t.Fatalf("legacy draft was imported into a non-empty DB; migration guard broken")
	}
	summary, err := st.SummarizeUsage(createdAt)
	if err != nil {
		t.Fatalf("SummarizeUsage() error = %v", err)
	}
	if summary.Calls != 0 {
		t.Fatalf("legacy usage imported into non-empty DB: %+v", summary)
	}

	// And the original seeded candidate must still be present.
	got, err := st.GetCandidate("seeded-1")
	if err != nil {
		t.Fatalf("seeded candidate missing after migration call: %v", err)
	}
	if got.Candidate.ProposedValue != "economics" {
		t.Fatalf("seeded candidate corrupted: %+v", got.Candidate)
	}
}
