package app

import (
	"testing"
	"time"

	"obsidian-harness/internal/model"
	"obsidian-harness/internal/persona"
)

func mustSaveDraft(t *testing.T, runtime *Runtime, id string, state model.DraftState) {
	t.Helper()
	if err := runtime.Store.Drafts().SaveDraft(model.Draft{
		ID:    id,
		Kind:  model.DraftKindMarkdownNoteWrite,
		State: state,
		Target: model.DocumentRef{
			Path:        "03-notes/" + id + ".md",
			Class:       model.DocClassNote,
			BaseVersion: model.DraftBaseVersionNewFile,
		},
		Title:           id,
		Summary:         "fixture",
		ProposedContent: "{}",
		CreatedAt:       time.Now(),
		UpdatedAt:       time.Now(),
	}); err != nil {
		t.Fatalf("SaveDraft(%s): %v", id, err)
	}
}

func mustSaveFinding(t *testing.T, runtime *Runtime, id string, state model.FindingState) {
	t.Helper()
	if err := runtime.Store.Findings().SaveFinding(model.Finding{
		ID:         id,
		Kind:       model.FindingOutOfBandVaultWrite,
		State:      state,
		Severity:   model.FindingSeverityInfo,
		Title:      id,
		Summary:    "fixture",
		Source:     "test",
		DetectedAt: time.Now(),
		UpdatedAt:  time.Now(),
	}); err != nil {
		t.Fatalf("SaveFinding(%s): %v", id, err)
	}
}

func TestRuntimeOperatorQueueAggregatesPendingItems(t *testing.T) {
	// OperatorQueue gathers pending drafts, open findings, and open persona
	// candidates into one view, filtering out non-actionable states, so the
	// operator has a single place to see what needs a decision.
	runtime, err := openRuntimeWithFakeProcessSinkSummarizer(t, t.TempDir())
	if err != nil {
		t.Fatalf("OpenRuntime() error = %v", err)
	}
	now := time.Date(2026, 6, 4, 10, 0, 0, 0, time.UTC)

	// One actionable + one non-actionable of each, to prove filtering.
	mustSaveDraft(t, runtime, "draft-pending", model.DraftPendingReview)
	mustSaveDraft(t, runtime, "draft-approved", model.DraftApproved)
	mustSaveFinding(t, runtime, "finding-open", model.FindingOpen)
	mustSaveFinding(t, runtime, "finding-resolved", model.FindingResolved)

	cand := persona.PersonaCandidate{Field: "major", ProposedValue: "economics", EvidenceQuote: "i study economics"}
	if _, _, err := runtime.Store.PersonaCandidates().UpsertCandidate(persona.PersonaCandidateRecord{
		ID:        "pc-1",
		State:     persona.PersonaCandidateOpen,
		DedupKey:  persona.DedupKey(cand),
		Candidate: cand,
		CreatedAt: now,
		UpdatedAt: now,
	}); err != nil {
		t.Fatalf("UpsertCandidate: %v", err)
	}

	queue, err := runtime.OperatorQueue(now)
	if err != nil {
		t.Fatalf("OperatorQueue() error = %v", err)
	}

	if len(queue.PendingDrafts) != 1 || queue.PendingDrafts[0].ID != "draft-pending" {
		t.Fatalf("PendingDrafts = %+v, want only draft-pending", queue.PendingDrafts)
	}
	if len(queue.OpenFindings) != 1 || queue.OpenFindings[0].ID != "finding-open" {
		t.Fatalf("OpenFindings = %+v, want only finding-open", queue.OpenFindings)
	}
	if len(queue.OpenPersonaCandidates) != 1 || queue.OpenPersonaCandidates[0].ID != "pc-1" {
		t.Fatalf("OpenPersonaCandidates = %+v, want only pc-1", queue.OpenPersonaCandidates)
	}
	if queue.ActionItemCount() != 3 {
		t.Fatalf("ActionItemCount = %d, want 3", queue.ActionItemCount())
	}
	if !queue.Day.Equal(now) {
		t.Fatalf("Day = %v, want %v", queue.Day, now)
	}
}
