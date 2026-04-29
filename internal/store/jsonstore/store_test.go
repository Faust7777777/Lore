package jsonstore

import (
	"path/filepath"
	"testing"
	"time"

	"obsidian-harness/internal/model"
)

func TestStorePersistsAcrossReload(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state", "store.json")
	createdAt := time.Date(2026, 4, 22, 10, 0, 0, 0, time.UTC)

	st, err := New(path, ".tmp")
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

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
		DetectedAt: createdAt,
		UpdatedAt:  createdAt,
	}); err != nil {
		t.Fatalf("SaveFinding() error = %v", err)
	}

	if err := st.SaveCursor("codex:session-1", "cursor-42"); err != nil {
		t.Fatalf("SaveCursor() error = %v", err)
	}

	reloaded, err := New(path, ".tmp")
	if err != nil {
		t.Fatalf("New(reload) error = %v", err)
	}

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

	findings, err := reloaded.ListFindings(1)
	if err != nil {
		t.Fatalf("ListFindings() error = %v", err)
	}
	if len(findings) != 1 || findings[0].ID != "finding-1" {
		t.Fatalf("findings = %+v, want finding-1", findings)
	}

	cursor, err := reloaded.GetCursor("codex:session-1")
	if err != nil {
		t.Fatalf("GetCursor() error = %v", err)
	}
	if cursor != "cursor-42" {
		t.Fatalf("cursor = %q, want cursor-42", cursor)
	}
}
