package drafts

import (
	"errors"
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
		{StateRevision, StateSuperseded},
		{StateRevision, StateExpired},
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

func TestNewRejectsMissingRequiredFields(t *testing.T) {
	// New is the creation boundary; each required field guards a later
	// stage (a missing Target.BaseVersion breaks optimistic concurrency
	// at Apply; a missing Target.Path has nowhere to write). Take a valid
	// base set, blank one field at a time, and require the matching
	// sentinel error.
	now := time.Date(2026, 4, 22, 21, 0, 0, 0, time.UTC)
	base := Params{
		ID:        "draft-100",
		Kind:      KindProgressUpdate,
		Source:    SourceOperator,
		Target:    Target{Path: "0-x/y.md", Class: model.DocClassProgressIndex, BaseVersion: "v1"},
		Summary:   "valid summary",
		CreatedAt: now,
	}
	if _, err := New(base); err != nil {
		t.Fatalf("base params should be valid, got %v", err)
	}

	cases := []struct {
		name    string
		mutate  func(p *Params)
		wantErr error
	}{
		{"blank id", func(p *Params) { p.ID = "  " }, ErrDraftIDRequired},
		{"empty kind", func(p *Params) { p.Kind = "" }, ErrDraftKindRequired},
		{"empty source", func(p *Params) { p.Source = "" }, ErrDraftSourceRequired},
		{"blank summary", func(p *Params) { p.Summary = "   " }, ErrDraftSummaryRequired},
		{"empty target path", func(p *Params) { p.Target.Path = "" }, ErrTargetPathRequired},
		{"empty base version", func(p *Params) { p.Target.BaseVersion = "" }, ErrTargetBaseVersionMissing},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			p := base
			tc.mutate(&p)
			if _, err := New(p); !errors.Is(err, tc.wantErr) {
				t.Fatalf("New() error = %v, want %v", err, tc.wantErr)
			}
		})
	}
}

func TestDraftRejectsInvalidTransitionsAndMissingActors(t *testing.T) {
	// Method-level lifecycle guards, distinct from the pure CanTransition
	// table: the transition methods must reject being called from the
	// wrong state and must require a named actor, and a rejected guard
	// must leave the state untouched -- otherwise a persona change could
	// be approved/applied without review or attribution.
	now := time.Date(2026, 4, 22, 21, 0, 0, 0, time.UTC)
	newDraft := func() *Draft {
		d, err := New(Params{
			ID: "draft-200", Kind: KindPersonaUpdate, Source: SourceOperator,
			Target:  Target{Path: "0-x/persona.md", Class: model.DocClassSystemDoc, BaseVersion: "v1"},
			Summary: "valid", CreatedAt: now,
		})
		if err != nil {
			t.Fatalf("New() error = %v", err)
		}
		return d
	}

	// Approve before SubmitForReview: wrong state.
	if err := newDraft().Approve("reviewer", "", now); err == nil {
		t.Fatal("Approve() from draft state error = nil, want invalid transition")
	}

	// SubmitForReview twice: the second call is from review state, not draft.
	d := newDraft()
	if err := d.SubmitForReview(now); err != nil {
		t.Fatalf("first SubmitForReview() error = %v", err)
	}
	if err := d.SubmitForReview(now); err == nil {
		t.Fatal("second SubmitForReview() error = nil, want invalid transition from review state")
	}

	// Approve with a blank reviewer must fail and leave state in review.
	d = newDraft()
	if err := d.SubmitForReview(now); err != nil {
		t.Fatalf("SubmitForReview() error = %v", err)
	}
	if err := d.Approve("   ", "comment", now); err == nil {
		t.Fatal("Approve() with blank reviewer error = nil, want reviewer-required")
	}
	if d.State != StateReview {
		t.Fatalf("State = %q after failed approve, want still %q", d.State, StateReview)
	}

	// Apply with a blank applier must fail and leave state approved.
	d = newDraft()
	if err := d.SubmitForReview(now); err != nil {
		t.Fatalf("SubmitForReview() error = %v", err)
	}
	if err := d.Approve("reviewer", "ok", now); err != nil {
		t.Fatalf("Approve() error = %v", err)
	}
	if err := d.Apply("  ", now); err == nil {
		t.Fatal("Apply() with blank applier error = nil, want applier-required")
	}
	if d.State != StateApproved {
		t.Fatalf("State = %q after failed apply, want still %q", d.State, StateApproved)
	}
}
