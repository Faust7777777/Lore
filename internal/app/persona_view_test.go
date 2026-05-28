package app

import (
	"testing"
	"time"

	"obsidian-harness/internal/persona"
)

func TestNewPersonaCandidateViewFlattensRecordAndEmbedsActions(t *testing.T) {
	// Pure-value unit: the converter must surface every record-side
	// field the brief enumerates (id/state/field/proposed/current/
	// evidence_quote/reason/confidence/source/session/observed_at/
	// draft_id/conflict/dedup_key) and embed the supplied actions
	// bundle verbatim. Regressions here would silently drop a column
	// from the TUI surface.
	observed := time.Date(2026, 5, 28, 12, 0, 0, 0, time.UTC)
	rec := persona.PersonaCandidateRecord{
		ID:       "cand-1",
		State:    persona.PersonaCandidateDrafted,
		DedupKey: "dedup-xyz",
		DraftID:  "draft-9",
		Candidate: persona.PersonaCandidate{
			Field:           "major",
			ProposedValue:   "economics",
			CurrentValue:    "philosophy",
			EvidenceQuote:   "I study economics now",
			Reason:          "user switched majors",
			Confidence:      persona.ConfidenceHigh,
			Conflict:        true,
			SourceKind:      persona.SourceConsole,
			SourceSessionID: "lore-sess-1",
			ObservedAt:      observed,
		},
	}
	actions := PersonaCandidateActions{
		CanRetry:      true,
		DraftReason:   "already drafted",
		DismissReason: "linked draft",
		RecoverReason: "already linked",
	}

	view := newPersonaCandidateView(rec, actions)

	if view.ID != "cand-1" || view.State != persona.PersonaCandidateDrafted {
		t.Fatalf("id/state mismatch: %+v", view)
	}
	if view.Field != "major" || view.ProposedValue != "economics" || view.CurrentValue != "philosophy" {
		t.Fatalf("field/proposed/current mismatch: %+v", view)
	}
	if view.EvidenceQuote != "I study economics now" || view.Reason != "user switched majors" {
		t.Fatalf("evidence/reason mismatch: %+v", view)
	}
	if view.Confidence != persona.ConfidenceHigh {
		t.Fatalf("confidence = %q, want high", view.Confidence)
	}
	if view.Source != persona.SourceConsole || view.Session != "lore-sess-1" {
		t.Fatalf("source/session mismatch: %+v", view)
	}
	if !view.ObservedAt.Equal(observed) {
		t.Fatalf("observed_at = %v, want %v", view.ObservedAt, observed)
	}
	if view.DraftID != "draft-9" || !view.Conflict || view.DedupKey != "dedup-xyz" {
		t.Fatalf("draft_id/conflict/dedup_key mismatch: %+v", view)
	}
	if view.Actions != actions {
		t.Fatalf("actions not embedded verbatim: %+v vs %+v", view.Actions, actions)
	}
}

func TestListPersonaCandidateViewsHonorsStateFilterAndEmbedsActions(t *testing.T) {
	// Integration: list-then-DTO path. Seed two open candidates,
	// promote one to drafted+linked, then assert the Open filter
	// returns exactly the remaining one with TUI-ready Actions
	// reflecting Open semantics (CanDraft + CanDismiss live, no
	// Recover / Retry).
	runtime := openTestRuntime(t)
	keepID := seedPersonaCandidate(t, runtime, "major", "economics", "I study economics")
	promoteID := seedPersonaCandidate(t, runtime, "city", "beijing", "I live in beijing")
	if _, _, err := runtime.CreatePersonaDraftFromCandidate(promoteID, time.Date(2026, 5, 28, 11, 0, 0, 0, time.UTC)); err != nil {
		t.Fatalf("promote: %v", err)
	}

	open, err := runtime.ListPersonaCandidateViews(persona.PersonaCandidateOpen, 0)
	if err != nil {
		t.Fatalf("ListPersonaCandidateViews: %v", err)
	}
	if len(open) != 1 || open[0].ID != keepID {
		t.Fatalf("expected exactly the un-promoted candidate; got %+v", open)
	}
	if !open[0].Actions.CanDraft || !open[0].Actions.CanDismiss {
		t.Fatalf("Open candidate view should expose Draft+Dismiss actions; got %+v", open[0].Actions)
	}
	if open[0].Actions.CanRecover || open[0].Actions.CanRetry {
		t.Fatalf("Open candidate view should not expose Recover/Retry; got %+v", open[0].Actions)
	}
	if open[0].DedupKey == "" {
		t.Fatalf("Open view missing DedupKey debug field")
	}

	drafted, err := runtime.ListPersonaCandidateViews(persona.PersonaCandidateDrafted, 0)
	if err != nil {
		t.Fatalf("ListPersonaCandidateViews(Drafted): %v", err)
	}
	if len(drafted) != 1 || drafted[0].ID != promoteID {
		t.Fatalf("expected exactly the promoted candidate in Drafted state; got %+v", drafted)
	}
	if drafted[0].DraftID == "" {
		t.Fatalf("Drafted view should carry a linked DraftID; got %+v", drafted[0])
	}
	if drafted[0].Actions.CanDraft || drafted[0].Actions.CanRetry {
		t.Fatalf("Drafted+pending-review should disable both Draft and Retry; got %+v", drafted[0].Actions)
	}
}

func TestGetPersonaCandidateViewReflectsLinkedRejectedDraftActions(t *testing.T) {
	// End-to-end: a candidate whose linked draft was rejected should
	// surface CanRetry=true through the GetPersonaCandidateView path.
	// This protects the TUI detail page from re-implementing the
	// terminal-state lookup the runtime owns.
	runtime := openTestRuntime(t)
	candidateID := seedPersonaCandidate(t, runtime, "major", "economics", "I study economics")
	_, result, err := runtime.CreatePersonaDraftFromCandidate(candidateID, time.Date(2026, 5, 28, 11, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatalf("CreateDraft: %v", err)
	}
	if _, err := runtime.RejectDraft(result.DraftID); err != nil {
		t.Fatalf("RejectDraft: %v", err)
	}

	view, err := runtime.GetPersonaCandidateView(candidateID)
	if err != nil {
		t.Fatalf("GetPersonaCandidateView: %v", err)
	}
	if !view.Actions.CanRetry {
		t.Fatalf("rejected-draft candidate view should expose CanRetry=true; got %+v", view.Actions)
	}
	if view.DraftID != result.DraftID {
		t.Fatalf("view.DraftID = %q, want %q", view.DraftID, result.DraftID)
	}
	if view.State != persona.PersonaCandidateDrafted {
		t.Fatalf("view.State = %q, want drafted", view.State)
	}
}

func TestListPersonaCandidateViewsEmptyReturnsEmptySlice(t *testing.T) {
	// Empty list must serialize as [] not null for the JSON consumers;
	// the slice initializer in ListPersonaCandidateViews guarantees
	// this even when the store has zero rows.
	runtime := openTestRuntime(t)
	if _, err := runtime.Bootstrap(time.Date(2026, 5, 28, 9, 0, 0, 0, time.UTC)); err != nil {
		t.Fatalf("Bootstrap: %v", err)
	}

	views, err := runtime.ListPersonaCandidateViews(persona.PersonaCandidateOpen, 0)
	if err != nil {
		t.Fatalf("ListPersonaCandidateViews: %v", err)
	}
	if views == nil {
		t.Fatalf("empty list should be non-nil empty slice for stable JSON shape")
	}
	if len(views) != 0 {
		t.Fatalf("expected zero views, got %d: %+v", len(views), views)
	}
}

func TestGetPersonaCandidateViewMissingIDPropagatesError(t *testing.T) {
	// store.ErrNotFound must surface so the CLI/TUI can distinguish
	// "unknown id" from "runtime error". The view path delegates to
	// GetPersonaCandidate; this test pins the delegation.
	runtime := openTestRuntime(t)
	if _, err := runtime.Bootstrap(time.Date(2026, 5, 28, 9, 0, 0, 0, time.UTC)); err != nil {
		t.Fatalf("Bootstrap: %v", err)
	}

	if _, err := runtime.GetPersonaCandidateView("does-not-exist"); err == nil {
		t.Fatal("GetPersonaCandidateView(nonexistent) = nil error, want store error")
	}
}
