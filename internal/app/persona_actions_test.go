package app

import (
	"strings"
	"testing"
	"time"

	"obsidian-harness/internal/persona"
)

// newPersonaActionsRecord builds a minimal candidate record at the
// supplied state / DraftID purely as a value, without touching any
// store. Used by the state-only PersonaCandidateActions tests so they
// can pass `(*Runtime)(nil)` and skip harness setup; the helper does
// not race or share state across goroutines.
func newPersonaActionsRecord(state persona.PersonaCandidateState, draftID string) persona.PersonaCandidateRecord {
	now := time.Date(2026, 5, 28, 9, 0, 0, 0, time.UTC)
	c := persona.PersonaCandidate{
		Field:           "major",
		ProposedValue:   "economics",
		EvidenceQuote:   "I study economics",
		Confidence:      persona.ConfidenceHigh,
		SourceKind:      persona.SourceConsole,
		SourceSessionID: "lore-test",
		ObservedAt:      now,
	}
	return persona.PersonaCandidateRecord{
		ID:        persona.NewCandidateID(now),
		State:     state,
		DraftID:   draftID,
		DedupKey:  persona.DedupKey(c),
		Candidate: c,
		CreatedAt: now,
		UpdatedAt: now,
	}
}

func TestPersonaCandidateActionsOpenAllowsDraftAndDismiss(t *testing.T) {
	rec := newPersonaActionsRecord(persona.PersonaCandidateOpen, "")
	// Nil receiver is fine: Open path does not touch r.Harness, so
	// the helper stays pure for state-only inputs.
	got := (*Runtime)(nil).PersonaCandidateActions(rec)

	if !got.CanDraft || got.DraftReason != "" {
		t.Fatalf("Open should allow Draft cleanly; got %+v", got)
	}
	if !got.CanDismiss || got.DismissReason != "" {
		t.Fatalf("Open should allow Dismiss cleanly; got %+v", got)
	}
	if got.CanRecover || got.RecoverReason == "" {
		t.Fatalf("Open should disallow Recover with a reason; got %+v", got)
	}
	if got.CanRetry || got.RetryReason == "" {
		t.Fatalf("Open should disallow Retry with a reason; got %+v", got)
	}
}

func TestPersonaCandidateActionsDraftedOrphanAllowsRecoverOnly(t *testing.T) {
	rec := newPersonaActionsRecord(persona.PersonaCandidateDrafted, "")
	got := (*Runtime)(nil).PersonaCandidateActions(rec)

	if got.CanDraft || !strings.Contains(got.DraftReason, "partial") {
		t.Fatalf("Drafted orphan should disallow Draft with partial-state reason; got %+v", got)
	}
	if got.CanDismiss || !strings.Contains(got.DismissReason, "recover") {
		t.Fatalf("Drafted orphan Dismiss should route through Recover; got %+v", got)
	}
	if !got.CanRecover || got.RecoverReason != "" {
		t.Fatalf("Drafted orphan should allow Recover cleanly; got %+v", got)
	}
	if got.CanRetry || !strings.Contains(got.RetryReason, "no linked draft") {
		t.Fatalf("Drafted orphan should disallow Retry; got %+v", got)
	}
}

func TestPersonaCandidateActionsDismissedDisablesEverything(t *testing.T) {
	rec := newPersonaActionsRecord(persona.PersonaCandidateDismissed, "draft-x")
	got := (*Runtime)(nil).PersonaCandidateActions(rec)

	if got.CanDraft || got.CanDismiss || got.CanRecover || got.CanRetry {
		t.Fatalf("Dismissed should disallow every action; got %+v", got)
	}
	for label, reason := range map[string]string{
		"DraftReason":   got.DraftReason,
		"DismissReason": got.DismissReason,
		"RecoverReason": got.RecoverReason,
		"RetryReason":   got.RetryReason,
	} {
		if reason == "" {
			t.Fatalf("%s should be populated for dismissed candidate", label)
		}
	}
}

func TestPersonaCandidateActionsLinkedPendingReviewDisablesRetry(t *testing.T) {
	// Linked candidate with a still-pending draft: Retry must stay
	// disabled because RetryRejectedPersonaDraft would race the
	// reviewer. RetryReason should explain the non-terminal state so
	// the TUI tooltip can surface it.
	runtime := openTestRuntime(t)
	candidateID := seedPersonaCandidate(t, runtime, "major", "economics", "I study economics")
	updated, _, err := runtime.CreatePersonaDraftFromCandidate(candidateID, time.Date(2026, 5, 28, 10, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatalf("CreatePersonaDraftFromCandidate() error = %v", err)
	}

	got := runtime.PersonaCandidateActions(updated)
	if got.CanRetry {
		t.Fatalf("pending-review draft should not be retryable; got %+v", got)
	}
	if !strings.Contains(got.RetryReason, "not a terminal") {
		t.Fatalf("RetryReason should explain non-terminal state; got %q", got.RetryReason)
	}
	if got.CanDraft || got.CanDismiss || got.CanRecover {
		t.Fatalf("linked-pending should disable Draft/Dismiss/Recover; got %+v", got)
	}
}

func TestPersonaCandidateActionsLinkedRejectedAllowsRetry(t *testing.T) {
	// The retry contract: only rejected / expired / superseded
	// drafts qualify. Drive a rejection through the public Runtime
	// method so the test exercises the same surface a TUI would.
	runtime := openTestRuntime(t)
	candidateID := seedPersonaCandidate(t, runtime, "major", "economics", "I study economics")
	_, result, err := runtime.CreatePersonaDraftFromCandidate(candidateID, time.Date(2026, 5, 28, 10, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatalf("CreateDraft: %v", err)
	}
	if _, err := runtime.RejectDraft(result.DraftID); err != nil {
		t.Fatalf("RejectDraft: %v", err)
	}
	refreshed, err := runtime.GetPersonaCandidate(candidateID)
	if err != nil {
		t.Fatalf("GetPersonaCandidate: %v", err)
	}

	got := runtime.PersonaCandidateActions(refreshed)
	if !got.CanRetry || got.RetryReason != "" {
		t.Fatalf("rejected draft should allow Retry cleanly; got %+v", got)
	}
	if got.CanDraft || got.CanDismiss || got.CanRecover {
		t.Fatalf("retry-eligible candidate should keep Draft/Dismiss/Recover blocked; got %+v", got)
	}
}

func TestPersonaCandidateActionsLinkedApprovedDisablesRetry(t *testing.T) {
	// Approved/applied draft already represents accepted work; retry
	// must not be offered (would duplicate). isTerminalDraftStateForRetry
	// rejects DraftApproved, so CanRetry should be false.
	runtime := openTestRuntime(t)
	candidateID := seedPersonaCandidate(t, runtime, "major", "economics", "I study economics")
	_, result, err := runtime.CreatePersonaDraftFromCandidate(candidateID, time.Date(2026, 5, 28, 10, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatalf("CreateDraft: %v", err)
	}
	if _, err := runtime.ApproveDraft(result.DraftID); err != nil {
		t.Fatalf("ApproveDraft: %v", err)
	}
	refreshed, err := runtime.GetPersonaCandidate(candidateID)
	if err != nil {
		t.Fatalf("GetPersonaCandidate: %v", err)
	}

	got := runtime.PersonaCandidateActions(refreshed)
	if got.CanRetry {
		t.Fatalf("approved draft should not be retryable; got %+v", got)
	}
	if !strings.Contains(got.RetryReason, "not a terminal") {
		t.Fatalf("RetryReason should explain non-terminal state for approved draft; got %q", got.RetryReason)
	}
}

func TestPersonaCandidateActionsLinkedDraftLookupFailureBlocksAll(t *testing.T) {
	// Drafted+linked but pointing at a non-existent draft is a
	// pathological state (operator manually corrupted, or a store
	// truncation). The helper must NOT crash and must surface a
	// reason naming the missing draft ID so the TUI can render a
	// read-only banner instead of broken buttons.
	runtime := openTestRuntime(t)
	rec := newPersonaActionsRecord(persona.PersonaCandidateDrafted, "ghost-draft-id")

	got := runtime.PersonaCandidateActions(rec)
	if got.CanDraft || got.CanDismiss || got.CanRecover || got.CanRetry {
		t.Fatalf("ghost-linked draft should disable every action; got %+v", got)
	}
	if !strings.Contains(got.RetryReason, "ghost-draft-id") {
		t.Fatalf("RetryReason should mention the missing draft ID; got %q", got.RetryReason)
	}
	if !strings.Contains(got.DismissReason, "ghost-draft-id") {
		t.Fatalf("DismissReason should mention the missing draft ID; got %q", got.DismissReason)
	}
}

func TestPersonaCandidateActionsUnknownStateDisablesAll(t *testing.T) {
	// Forward-compat: a future state added to the persona package
	// before this helper learns about it must fail closed (no
	// actions allowed) rather than fail open with all buttons live.
	rec := newPersonaActionsRecord(persona.PersonaCandidateState("future-state"), "")
	got := (*Runtime)(nil).PersonaCandidateActions(rec)

	if got.CanDraft || got.CanDismiss || got.CanRecover || got.CanRetry {
		t.Fatalf("unknown state should disable every action; got %+v", got)
	}
	if !strings.Contains(got.DraftReason, "unknown") {
		t.Fatalf("DraftReason should flag the unknown state; got %q", got.DraftReason)
	}
}
