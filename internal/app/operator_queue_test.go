package app

import (
	"errors"
	"testing"
	"time"

	"obsidian-harness/internal/config"
	"obsidian-harness/internal/model"
	"obsidian-harness/internal/orchestrator"
	"obsidian-harness/internal/persona"
	"obsidian-harness/internal/store"
	"obsidian-harness/internal/store/memory"
)

// usageFailingStateStore wraps a real StateStore but hands out a Usage
// store whose SummarizeUsage always fails, so a test can prove the
// operator queue stays best-effort on the usage glance.
type usageFailingStateStore struct {
	store.StateStore
}

func (usageFailingStateStore) Usage() store.UsageStore { return failingUsageStore{} }

type failingUsageStore struct{}

func (failingUsageStore) AppendUsage(model.UsageRecord) error { return nil }

func (failingUsageStore) SummarizeUsage(time.Time) (model.UsageSummary, error) {
	return model.UsageSummary{}, errors.New("usage store unavailable")
}

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

func TestRuntimeOperatorQueueToleratesUsageFailure(t *testing.T) {
	// A usage-store hiccup must not hide the action items: the queue is
	// best-effort on the usage glance and still returns pending
	// drafts / findings / candidates. Memory-backed so there is no sqlite
	// handle to lock the temp dir on cleanup.
	mem := memory.New()
	harness, err := orchestrator.New(config.Default(t.TempDir()), mem)
	if err != nil {
		t.Fatalf("orchestrator.New() error = %v", err)
	}
	if err := mem.Findings().SaveFinding(model.Finding{
		ID:         "f-open",
		Kind:       model.FindingOutOfBandVaultWrite,
		State:      model.FindingOpen,
		Severity:   model.FindingSeverityInfo,
		Title:      "seed",
		Summary:    "fixture",
		Source:     "test",
		DetectedAt: time.Now(),
		UpdatedAt:  time.Now(),
	}); err != nil {
		t.Fatalf("SaveFinding: %v", err)
	}

	// usageFailingStateStore wraps the shared mem store so SummarizeUsage
	// fails while Findings / Drafts / candidates still resolve.
	r := &Runtime{Store: usageFailingStateStore{StateStore: mem}, Harness: harness}

	queue, err := r.OperatorQueue(time.Date(2026, 6, 4, 10, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatalf("usage failure must not fail the queue: %v", err)
	}
	if len(queue.OpenFindings) != 1 || queue.OpenFindings[0].ID != "f-open" {
		t.Fatalf("action items must still surface despite usage failure: %+v", queue.OpenFindings)
	}
	if queue.TodayUsage.Calls != 0 {
		t.Fatalf("usage glance should be zero on failure, got %d calls", queue.TodayUsage.Calls)
	}
}
