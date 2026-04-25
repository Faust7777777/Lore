package drafts

import (
	"testing"
	"time"

	"obsidian-harness/internal/model"
)

func TestNewDraftStartsInDraftState(t *testing.T) {
	now := time.Date(2026, 4, 22, 21, 0, 0, 0, time.UTC)

	draft, err := New(Params{
		ID:        "draft-001",
		Kind:      KindProgressUpdate,
		Source:    SourceExternalAgent,
		Target:    Target{Path: "0-\u6392\u671f/\u6587\u6863\u8fdb\u5ea6\u603b\u8868.md", Class: model.DocClassProgressIndex, BaseVersion: "progress-v7"},
		Summary:   "update database learning progress",
		CreatedAt: now,
	})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	if draft.State != StateDraft {
		t.Fatalf("State = %q, want %q", draft.State, StateDraft)
	}
	if draft.Source != SourceExternalAgent {
		t.Fatalf("Source = %q, want %q", draft.Source, SourceExternalAgent)
	}
	if draft.Title != draft.Summary {
		t.Fatalf("Title = %q, want fallback summary %q", draft.Title, draft.Summary)
	}
	if draft.AllowsDirectWrite() {
		t.Fatal("AllowsDirectWrite() = true, want false")
	}
}

func TestDraftMustBeReviewedBeforeApply(t *testing.T) {
	now := time.Date(2026, 4, 22, 21, 0, 0, 0, time.UTC)

	draft, err := New(Params{
		ID:        "draft-002",
		Kind:      KindWeaknessCard,
		Source:    SourcePrimaryAgent,
		Target:    Target{Path: "0-\u6392\u671f/00-\u7cfb\u7edf/\u8584\u5f31\u70b9\u603b\u8868.md", Class: model.DocClassSystemDoc, BaseVersion: "weakness-v3"},
		Summary:   "record weakness card for SQL triggers",
		CreatedAt: now,
	})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	if err := draft.Apply("operator", now.Add(time.Minute)); err == nil {
		t.Fatal("Apply() error = nil, want transition error before review")
	}

	if err := draft.SubmitForReview(now.Add(2 * time.Minute)); err != nil {
		t.Fatalf("SubmitForReview() error = %v", err)
	}
	if draft.State != StateReview {
		t.Fatalf("State = %q, want %q", draft.State, StateReview)
	}

	if err := draft.Apply("operator", now.Add(3*time.Minute)); err == nil {
		t.Fatal("Apply() error = nil, want transition error before approval")
	}

	if err := draft.Approve("operator", "looks good", now.Add(4*time.Minute)); err != nil {
		t.Fatalf("Approve() error = %v", err)
	}
	if draft.State != StateApproved {
		t.Fatalf("State = %q, want %q", draft.State, StateApproved)
	}
	if !draft.ReadyToApply() {
		t.Fatal("ReadyToApply() = false, want true after approval")
	}

	if err := draft.Apply("operator", now.Add(5*time.Minute)); err != nil {
		t.Fatalf("Apply() error = %v", err)
	}
	if draft.State != StateApplied {
		t.Fatalf("State = %q, want %q", draft.State, StateApplied)
	}
	if draft.AppliedBy != "operator" {
		t.Fatalf("AppliedBy = %q, want %q", draft.AppliedBy, "operator")
	}
}

func TestValidateTransitionCoversAllDraftTerminalStates(t *testing.T) {
	validTransitions := []struct {
		from State
		to   State
	}{
		{StateDraft, StateReview},
		{StateDraft, StateExpired},
		{StateDraft, StateSuperseded},
		{StateReview, StateApproved},
		{StateReview, StateRejected},
		{StateReview, StateRevision},
		{StateReview, StateExpired},
		{StateReview, StateSuperseded},
		{StateApproved, StateApplied},
		{StateApproved, StateConflicted},
		{StateApproved, StateExpired},
		{StateApproved, StateSuperseded},
	}

	for _, transition := range validTransitions {
		if err := ValidateTransition(transition.from, transition.to); err != nil {
			t.Fatalf("ValidateTransition(%q, %q) error = %v", transition.from, transition.to, err)
		}
	}

	invalidTransitions := []struct {
		from State
		to   State
	}{
		{StateApplied, StateReview},
		{StateRejected, StateApproved},
		{StateRevision, StateApproved},
		{StateConflicted, StateApplied},
		{StateReview, StateApplied},
		{StateApproved, StateRejected},
	}

	for _, transition := range invalidTransitions {
		if err := ValidateTransition(transition.from, transition.to); err == nil {
			t.Fatalf("ValidateTransition(%q, %q) error = nil, want invalid transition", transition.from, transition.to)
		}
	}
}
