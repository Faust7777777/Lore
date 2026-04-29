package sqlitestore

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"obsidian-harness/internal/model"
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
