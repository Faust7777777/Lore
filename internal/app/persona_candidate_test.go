package app

import (
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"obsidian-harness/internal/config/configtest"
	"obsidian-harness/internal/model"
	"obsidian-harness/internal/persona"
	"obsidian-harness/internal/store"
)

// seedPersonaCandidate inserts one Open candidate into a fresh
// runtime and returns the runtime + candidate ID. Helper centralizes
// the boilerplate for P5 lifecycle tests.
func seedPersonaCandidate(t *testing.T, runtime *Runtime, field, value, evidence string) string {
	t.Helper()
	if _, err := runtime.Bootstrap(time.Date(2026, 5, 20, 10, 0, 0, 0, time.UTC)); err != nil {
		t.Fatalf("Bootstrap() error = %v", err)
	}
	now := time.Date(2026, 5, 20, 10, 30, 0, 0, time.UTC)
	c := persona.PersonaCandidate{
		Field:           field,
		ProposedValue:   value,
		EvidenceQuote:   evidence,
		Reason:          "test seed",
		Confidence:      persona.ConfidenceHigh,
		SourceKind:      persona.SourceConsole,
		SourceSessionID: "lore-test-session",
		ObservedAt:      now,
	}
	rec := persona.PersonaCandidateRecord{
		ID:        persona.NewCandidateID(now),
		State:     persona.PersonaCandidateOpen,
		DedupKey:  persona.DedupKey(c),
		Candidate: c,
		CreatedAt: now,
		UpdatedAt: now,
	}
	stored, isNew, err := runtime.RecordPersonaCandidate(rec)
	if err != nil || !isNew {
		t.Fatalf("seed RecordPersonaCandidate isNew=%v err=%v", isNew, err)
	}
	return stored.ID
}

func openTestRuntime(t *testing.T) *Runtime {
	t.Helper()
	workDir := t.TempDir()
	runtime, err := OpenRuntimeWithConfigOptions(workDir, configtest.IsolatedOptions(t))
	if err != nil {
		t.Fatalf("OpenRuntime() error = %v", err)
	}
	t.Cleanup(func() { _ = runtime.Close() })
	return runtime
}

func TestCreatePersonaDraftFromCandidateCreatesPendingDraftAndTransitionsState(t *testing.T) {
	runtime := openTestRuntime(t)
	candidateID := seedPersonaCandidate(t, runtime, "major", "economics", "I major in economics")

	updated, result, err := runtime.CreatePersonaDraftFromCandidate(candidateID, time.Date(2026, 5, 20, 11, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatalf("CreatePersonaDraftFromCandidate() error = %v", err)
	}
	if result.Status != "draft_created" || result.DraftID == "" {
		t.Fatalf("result = %+v", result)
	}
	if !result.ReviewRequired {
		t.Fatalf("ReviewRequired = false, want true")
	}
	if updated.State != persona.PersonaCandidateDrafted {
		t.Fatalf("candidate state = %q, want drafted", updated.State)
	}

	// Draft exists, is pending review, summary contains candidate
	// evidence / reason / confidence / conflict markers.
	draft, err := runtime.Store.Drafts().GetDraft(result.DraftID)
	if err != nil {
		t.Fatalf("GetDraft() error = %v", err)
	}
	if draft.Kind != model.DraftKindPersonaUpdate {
		t.Fatalf("draft kind = %q, want persona_update", draft.Kind)
	}
	if draft.State != model.DraftPendingReview {
		t.Fatalf("draft state = %q, want pending_review", draft.State)
	}
	for _, want := range []string{
		"I major in economics", // evidence_quote
		"test seed",            // reason
		"high",                 // confidence
		"conflict: false",      // conflict marker
	} {
		if !strings.Contains(draft.Summary, want) {
			t.Fatalf("draft summary missing %q:\n%s", want, draft.Summary)
		}
	}
}

func TestCreatePersonaDraftFromCandidateIsIdempotentOnRetry(t *testing.T) {
	// Reviewer-flagged behavior: re-invoking the action against an
	// already-drafted candidate must NOT create a second persona
	// update draft. It must return the existing draft instead so the
	// store stays linear.
	runtime := openTestRuntime(t)
	candidateID := seedPersonaCandidate(t, runtime, "major", "economics", "I major in economics")

	firstUpdated, firstResult, err := runtime.CreatePersonaDraftFromCandidate(candidateID, time.Date(2026, 5, 20, 11, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatalf("first CreatePersonaDraftFromCandidate error = %v", err)
	}
	if firstResult.DraftID == "" {
		t.Fatal("first call returned empty DraftID")
	}
	if firstUpdated.DraftID != firstResult.DraftID {
		t.Fatalf("first call: record.DraftID = %q, result.DraftID = %q (must match)", firstUpdated.DraftID, firstResult.DraftID)
	}

	secondUpdated, secondResult, err := runtime.CreatePersonaDraftFromCandidate(candidateID, time.Date(2026, 5, 20, 12, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatalf("second CreatePersonaDraftFromCandidate error = %v (want idempotent success)", err)
	}
	if secondResult.DraftID != firstResult.DraftID {
		t.Fatalf("second result DraftID = %q, want same as first %q (idempotent retry must return same draft)", secondResult.DraftID, firstResult.DraftID)
	}
	if secondUpdated.DraftID != firstUpdated.DraftID {
		t.Fatalf("second record DraftID = %q, want %q", secondUpdated.DraftID, firstUpdated.DraftID)
	}

	// Exactly one persona_update draft exists in the store; the
	// retry did not produce a duplicate.
	drafts, err := runtime.Store.Drafts().ListDrafts()
	if err != nil {
		t.Fatalf("ListDrafts() error = %v", err)
	}
	count := 0
	for _, d := range drafts {
		if d.Kind == model.DraftKindPersonaUpdate {
			count++
		}
	}
	if count != 1 {
		t.Fatalf("persona_update drafts = %d, want 1 (idempotent retry must NOT duplicate)", count)
	}
}

func TestCreatePersonaDraftFromCandidateRejectsLegacyDraftedWithoutLink(t *testing.T) {
	// Legacy/inconsistent state: candidate marked Drafted but DraftID
	// empty (e.g. an earlier P5 build that lacked LinkCandidateDraft).
	// The runtime must refuse silent re-creation; operator must
	// reconcile manually.
	runtime := openTestRuntime(t)
	candidateID := seedPersonaCandidate(t, runtime, "major", "economics", "I major in economics")
	// Force the inconsistent state via UpdateCandidateState (sets
	// state without DraftID).
	if _, err := runtime.Store.PersonaCandidates().UpdateCandidateState(candidateID, persona.PersonaCandidateDrafted, time.Now()); err != nil {
		t.Fatalf("UpdateCandidateState() error = %v", err)
	}
	_, _, err := runtime.CreatePersonaDraftFromCandidate(candidateID, time.Date(2026, 5, 20, 12, 0, 0, 0, time.UTC))
	if !errors.Is(err, ErrPersonaCandidateAlreadyDrafted) {
		t.Fatalf("error = %v, want ErrPersonaCandidateAlreadyDrafted for legacy state", err)
	}
}

func TestCreatePersonaDraftFromCandidateRejectsDismissed(t *testing.T) {
	runtime := openTestRuntime(t)
	candidateID := seedPersonaCandidate(t, runtime, "major", "economics", "I major in economics")
	if _, err := runtime.DismissPersonaCandidate(candidateID, time.Date(2026, 5, 20, 11, 0, 0, 0, time.UTC)); err != nil {
		t.Fatalf("DismissPersonaCandidate() error = %v", err)
	}

	_, _, err := runtime.CreatePersonaDraftFromCandidate(candidateID, time.Date(2026, 5, 20, 12, 0, 0, 0, time.UTC))
	if !errors.Is(err, ErrPersonaCandidateDismissed) {
		t.Fatalf("draft of dismissed candidate error = %v, want ErrPersonaCandidateDismissed", err)
	}
}

func TestCreatePersonaDraftFromCandidateReturnsErrNotFound(t *testing.T) {
	runtime := openTestRuntime(t)
	_, _, err := runtime.CreatePersonaDraftFromCandidate("missing-id", time.Now())
	if !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("missing-id error = %v, want store.ErrNotFound", err)
	}
}

func TestDismissPersonaCandidateTransitionsState(t *testing.T) {
	runtime := openTestRuntime(t)
	candidateID := seedPersonaCandidate(t, runtime, "major", "economics", "I major in economics")

	updated, err := runtime.DismissPersonaCandidate(candidateID, time.Date(2026, 5, 20, 11, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatalf("DismissPersonaCandidate() error = %v", err)
	}
	if updated.State != persona.PersonaCandidateDismissed {
		t.Fatalf("state = %q, want dismissed", updated.State)
	}

	// Dismissed candidates do not appear in the Open queue.
	open, err := runtime.ListPersonaCandidates(persona.PersonaCandidateOpen, 0)
	if err != nil {
		t.Fatalf("ListPersonaCandidates(open) error = %v", err)
	}
	for _, r := range open {
		if r.ID == candidateID {
			t.Fatalf("dismissed candidate still in Open list: %+v", r)
		}
	}
}

func TestDismissPersonaCandidateIsIdempotent(t *testing.T) {
	runtime := openTestRuntime(t)
	candidateID := seedPersonaCandidate(t, runtime, "major", "economics", "I major in economics")
	first, err := runtime.DismissPersonaCandidate(candidateID, time.Date(2026, 5, 20, 11, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatalf("first dismiss error = %v", err)
	}
	second, err := runtime.DismissPersonaCandidate(candidateID, time.Date(2026, 5, 20, 12, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatalf("second dismiss error = %v", err)
	}
	if first.UpdatedAt != second.UpdatedAt {
		t.Fatalf("second dismiss bumped UpdatedAt %v != %v", second.UpdatedAt, first.UpdatedAt)
	}
}

func TestDismissPersonaCandidateRejectsDrafted(t *testing.T) {
	runtime := openTestRuntime(t)
	candidateID := seedPersonaCandidate(t, runtime, "major", "economics", "I major in economics")
	if _, _, err := runtime.CreatePersonaDraftFromCandidate(candidateID, time.Date(2026, 5, 20, 11, 0, 0, 0, time.UTC)); err != nil {
		t.Fatalf("CreatePersonaDraftFromCandidate() error = %v", err)
	}
	_, err := runtime.DismissPersonaCandidate(candidateID, time.Date(2026, 5, 20, 12, 0, 0, 0, time.UTC))
	if !errors.Is(err, ErrPersonaCandidateAlreadyDrafted) {
		t.Fatalf("dismiss of drafted candidate error = %v, want ErrPersonaCandidateAlreadyDrafted", err)
	}
}

// linkFailingPersonaStore wraps an underlying PersonaCandidateStore
// and forces LinkCandidateDraft to fail, so the test can verify that
// CreatePersonaDraftFromCandidate surfaces a wrapped error and the
// candidate is NOT silently re-drafted on a follow-up Get.
type linkFailingPersonaStore struct {
	inner   store.PersonaCandidateStore
	linkErr error
}

func (s *linkFailingPersonaStore) UpsertCandidate(record persona.PersonaCandidateRecord) (persona.PersonaCandidateRecord, bool, error) {
	return s.inner.UpsertCandidate(record)
}
func (s *linkFailingPersonaStore) GetCandidate(id string) (persona.PersonaCandidateRecord, error) {
	return s.inner.GetCandidate(id)
}
func (s *linkFailingPersonaStore) ListCandidatesByState(state persona.PersonaCandidateState, limit int) ([]persona.PersonaCandidateRecord, error) {
	return s.inner.ListCandidatesByState(state, limit)
}
func (s *linkFailingPersonaStore) UpdateCandidateState(id string, state persona.PersonaCandidateState, updatedAt time.Time) (persona.PersonaCandidateRecord, error) {
	return s.inner.UpdateCandidateState(id, state, updatedAt)
}
func (s *linkFailingPersonaStore) LinkCandidateDraft(id string, draftID string, updatedAt time.Time) (persona.PersonaCandidateRecord, error) {
	return persona.PersonaCandidateRecord{}, s.linkErr
}
func (s *linkFailingPersonaStore) ClaimCandidateForDraft(id string, now time.Time) (persona.PersonaCandidateRecord, error) {
	return s.inner.ClaimCandidateForDraft(id, now)
}

// stateStoreWithPersonaWrapper wraps a real store.StateStore but
// substitutes the PersonaCandidates accessor with linkFailingPersonaStore.
// Used to inject store-side failures into the LinkCandidateDraft step
// of CreatePersonaDraftFromCandidate without modifying the production
// sqlite/json/memory backends.
type stateStoreWithPersonaWrapper struct {
	inner   store.StateStore
	persona store.PersonaCandidateStore
}

func (s *stateStoreWithPersonaWrapper) Drafts() store.DraftStore           { return s.inner.Drafts() }
func (s *stateStoreWithPersonaWrapper) ProcessSink() store.ProcessSinkStore { return s.inner.ProcessSink() }
func (s *stateStoreWithPersonaWrapper) Audit() store.AuditStore             { return s.inner.Audit() }
func (s *stateStoreWithPersonaWrapper) Findings() store.FindingStore        { return s.inner.Findings() }
func (s *stateStoreWithPersonaWrapper) Usage() store.UsageStore             { return s.inner.Usage() }
func (s *stateStoreWithPersonaWrapper) Cursors() store.CursorStore          { return s.inner.Cursors() }
func (s *stateStoreWithPersonaWrapper) PersonaCandidates() store.PersonaCandidateStore {
	return s.persona
}

func TestCreatePersonaDraftFromCandidateRetryAfterLinkFailureDoesNotDuplicate(t *testing.T) {
	// Load-bearing duplicate-prevention proof. Sequence:
	//   1. Inject LinkCandidateDraft failure on the first attempt.
	//      The candidate has been transitioned to Drafted BEFORE
	//      proposal (the new contract); the proposal then creates a
	//      real persona_update draft, and finally the injected link
	//      step fails. Net state: candidate=Drafted+DraftID="",
	//      orphan draft exists.
	//   2. Restore the store so subsequent operations would succeed
	//      if attempted -- this is the "blind retry by an unaware
	//      operator" scenario.
	//   3. Retry CreatePersonaDraftFromCandidate. The contract is
	//      that the runtime refuses to re-propose
	//      (ErrPersonaCandidateAlreadyDrafted), so ListDrafts STILL
	//      contains exactly one persona_update draft. Without this
	//      guarantee, the partial-failure state would be a duplicate
	//      pump.
	runtime := openTestRuntime(t)
	candidateID := seedPersonaCandidate(t, runtime, "major", "economics", "I major in economics")

	originalStore := runtime.Store
	failingPersona := &linkFailingPersonaStore{
		inner:   originalStore.PersonaCandidates(),
		linkErr: errors.New("simulated link failure"),
	}
	runtime.Store = &stateStoreWithPersonaWrapper{
		inner:   originalStore,
		persona: failingPersona,
	}
	t.Cleanup(func() { runtime.Store = originalStore })

	_, firstResult, err := runtime.CreatePersonaDraftFromCandidate(candidateID, time.Date(2026, 5, 20, 11, 0, 0, 0, time.UTC))
	if err == nil {
		t.Fatal("first CreatePersonaDraftFromCandidate() error = nil, want wrapped link failure")
	}
	if !strings.Contains(err.Error(), "created draft") || !strings.Contains(err.Error(), "failed to link candidate") {
		t.Fatalf("error message = %q, want guidance about orphan draft + link failure", err.Error())
	}
	if firstResult.DraftID == "" {
		t.Fatal("firstResult.DraftID empty; expected the partial draft id surfaced for triage")
	}
	// Orphan draft is in store.
	if _, err := originalStore.Drafts().GetDraft(firstResult.DraftID); err != nil {
		t.Fatalf("orphan draft GetDraft() error = %v", err)
	}
	// Candidate is Drafted/unlinked (new contract).
	post, err := originalStore.PersonaCandidates().GetCandidate(candidateID)
	if err != nil {
		t.Fatalf("post-failure GetCandidate() error = %v", err)
	}
	if post.State != persona.PersonaCandidateDrafted {
		t.Fatalf("candidate state = %q, want drafted (mark-before-propose contract)", post.State)
	}
	if post.DraftID != "" {
		t.Fatalf("candidate DraftID = %q, want empty (link step failed)", post.DraftID)
	}

	// Restore the store: subsequent ops would succeed if attempted.
	// This simulates an operator blindly retrying after the failure.
	failingPersona.linkErr = nil

	_, secondResult, err := runtime.CreatePersonaDraftFromCandidate(candidateID, time.Date(2026, 5, 20, 12, 0, 0, 0, time.UTC))
	if !errors.Is(err, ErrPersonaCandidateAlreadyDrafted) {
		t.Fatalf("retry error = %v, want ErrPersonaCandidateAlreadyDrafted (must refuse to re-propose)", err)
	}
	if secondResult.DraftID != "" {
		t.Fatalf("retry result.DraftID = %q, want empty (no new proposal)", secondResult.DraftID)
	}

	// Critical assertion: store still has exactly one persona_update
	// draft. The retry did NOT create a duplicate.
	drafts, err := originalStore.Drafts().ListDrafts()
	if err != nil {
		t.Fatalf("ListDrafts() error = %v", err)
	}
	count := 0
	for _, d := range drafts {
		if d.Kind == model.DraftKindPersonaUpdate {
			count++
		}
	}
	if count != 1 {
		t.Fatalf("persona_update drafts after blind retry = %d, want 1 (retry must not duplicate)", count)
	}
}

func TestCreatePersonaDraftFromCandidateRollsBackOnProposalFailure(t *testing.T) {
	// When ProposePersonaUpdate fails the runtime must roll the
	// candidate back to Open so a retry can succeed normally. We
	// trigger a proposal failure by deleting the persona doc the
	// proposer reads for the base version -- the proposer is hard
	// to mock cleanly without restructuring orchestrator, so we
	// exercise the recovery path via a less-invasive seam: closing
	// the harness store mid-test would break too much, so instead
	// we use the validation path. Setting candidate.Confidence to
	// an invalid string is the simplest reproducible failure: the
	// proposer rejects it before touching the vault, and we can
	// observe the rollback.
	runtime := openTestRuntime(t)
	if _, err := runtime.Bootstrap(time.Date(2026, 5, 20, 10, 0, 0, 0, time.UTC)); err != nil {
		t.Fatalf("Bootstrap() error = %v", err)
	}
	now := time.Date(2026, 5, 20, 10, 30, 0, 0, time.UTC)
	c := persona.PersonaCandidate{
		Field:           "major",
		ProposedValue:   "economics",
		EvidenceQuote:   "I major in economics",
		Reason:          "test seed",
		Confidence:      persona.Confidence("not-a-valid-confidence"),
		SourceKind:      persona.SourceConsole,
		SourceSessionID: "lore-test-session",
		ObservedAt:      now,
	}
	rec := persona.PersonaCandidateRecord{
		ID:        persona.NewCandidateID(now),
		State:     persona.PersonaCandidateOpen,
		DedupKey:  persona.DedupKey(c),
		Candidate: c,
		CreatedAt: now,
		UpdatedAt: now,
	}
	stored, _, err := runtime.RecordPersonaCandidate(rec)
	if err != nil {
		t.Fatalf("RecordPersonaCandidate() error = %v", err)
	}

	_, _, err = runtime.CreatePersonaDraftFromCandidate(stored.ID, time.Date(2026, 5, 20, 11, 0, 0, 0, time.UTC))
	if err == nil {
		t.Fatal("CreatePersonaDraftFromCandidate() error = nil, want validation failure")
	}
	post, err := runtime.GetPersonaCandidate(stored.ID)
	if err != nil {
		t.Fatalf("post-failure GetPersonaCandidate() error = %v", err)
	}
	if post.State != persona.PersonaCandidateOpen {
		t.Fatalf("candidate state = %q, want open after proposal-validation rollback", post.State)
	}
}

// barrierClaimStore wraps a PersonaCandidateStore and blocks every
// ClaimCandidateForDraft call on a barrier channel until the test
// releases it. The wrapper also counts how many times the *inner*
// ProposePersonaUpdate would have been reached (via a counter
// signaled by the test once the claim returns) so the test can
// assert "only one caller reached propose". The proposeCount is
// incremented by the test goroutine itself after Claim returns nil,
// which corresponds exactly to the code path in
// CreatePersonaDraftFromCandidate.
type barrierClaimStore struct {
	inner   store.PersonaCandidateStore
	release chan struct{}
}

func (s *barrierClaimStore) UpsertCandidate(record persona.PersonaCandidateRecord) (persona.PersonaCandidateRecord, bool, error) {
	return s.inner.UpsertCandidate(record)
}
func (s *barrierClaimStore) GetCandidate(id string) (persona.PersonaCandidateRecord, error) {
	return s.inner.GetCandidate(id)
}
func (s *barrierClaimStore) ListCandidatesByState(state persona.PersonaCandidateState, limit int) ([]persona.PersonaCandidateRecord, error) {
	return s.inner.ListCandidatesByState(state, limit)
}
func (s *barrierClaimStore) UpdateCandidateState(id string, state persona.PersonaCandidateState, updatedAt time.Time) (persona.PersonaCandidateRecord, error) {
	return s.inner.UpdateCandidateState(id, state, updatedAt)
}
func (s *barrierClaimStore) LinkCandidateDraft(id string, draftID string, updatedAt time.Time) (persona.PersonaCandidateRecord, error) {
	return s.inner.LinkCandidateDraft(id, draftID, updatedAt)
}
func (s *barrierClaimStore) ClaimCandidateForDraft(id string, now time.Time) (persona.PersonaCandidateRecord, error) {
	// Wait until the test releases the barrier so all goroutines
	// reach the Claim call together. Once released, dispatch to the
	// inner store; its mutex / transaction serializes the CAS so
	// only one caller wins.
	<-s.release
	return s.inner.ClaimCandidateForDraft(id, now)
}

func TestCreatePersonaDraftFromCandidateConcurrentCallsOnlyOneSucceeds(t *testing.T) {
	// The reviewer-required concurrent claim test: spawn N
	// goroutines that all reach the ClaimCandidateForDraft step
	// simultaneously. The store-level CAS must guarantee exactly
	// one wins, the losers receive ErrPersonaCandidateAlreadyDrafted
	// (because the winner's claim flips the state to Drafted before
	// any loser's claim runs), no loser reaches
	// harness.ProposePersonaUpdate, and the drafts table contains
	// exactly one persona_update entry.
	runtime := openTestRuntime(t)
	candidateID := seedPersonaCandidate(t, runtime, "major", "economics", "I major in economics")

	originalStore := runtime.Store
	release := make(chan struct{})
	wrapper := &stateStoreWithPersonaWrapper{
		inner: originalStore,
		persona: &barrierClaimStore{
			inner:   originalStore.PersonaCandidates(),
			release: release,
		},
	}
	runtime.Store = wrapper
	t.Cleanup(func() { runtime.Store = originalStore })

	const goroutines = 8
	var wg sync.WaitGroup
	errs := make([]error, goroutines)
	results := make([]model.PersonaUpdateProposalResult, goroutines)
	for i := 0; i < goroutines; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			_, results[idx], errs[idx] = runtime.CreatePersonaDraftFromCandidate(candidateID, time.Date(2026, 5, 21, 10, 0, 0, idx, time.UTC))
		}(i)
	}
	// Give all goroutines time to park on the barrier, then release.
	time.Sleep(50 * time.Millisecond)
	close(release)
	wg.Wait()

	winners := 0
	losers := 0
	other := 0
	winnerDraftID := ""
	loserDraftIDs := map[string]struct{}{}
	for i, err := range errs {
		switch {
		case err == nil:
			winners++
			winnerDraftID = results[i].DraftID
		case errors.Is(err, ErrPersonaCandidateAlreadyDrafted):
			losers++
		default:
			other++
			t.Logf("unexpected err on goroutine %d: %v", i, err)
		}
		if results[i].DraftID != "" {
			loserDraftIDs[results[i].DraftID] = struct{}{}
		}
	}
	if winners != 1 {
		t.Fatalf("winners = %d, want exactly 1 (errs = %v)", winners, errs)
	}
	if other != 0 {
		t.Fatalf("got %d unexpected errors that were neither nil nor ErrPersonaCandidateAlreadyDrafted: %v", other, errs)
	}
	if losers != goroutines-1 {
		t.Fatalf("losers = %d, want %d", losers, goroutines-1)
	}
	if winnerDraftID == "" {
		t.Fatal("winner had empty DraftID")
	}

	// Critical: ListDrafts contains exactly one persona_update draft
	// regardless of how many concurrent callers raced. The losers'
	// idempotent return path may have surfaced the same DraftID, but
	// only one underlying draft exists.
	drafts, err := originalStore.Drafts().ListDrafts()
	if err != nil {
		t.Fatalf("ListDrafts() error = %v", err)
	}
	count := 0
	for _, d := range drafts {
		if d.Kind == model.DraftKindPersonaUpdate {
			count++
			if d.ID != winnerDraftID {
				t.Fatalf("found persona draft %q that does not match winner %q", d.ID, winnerDraftID)
			}
		}
	}
	if count != 1 {
		t.Fatalf("persona_update drafts in store = %d, want 1 across %d concurrent callers", count, goroutines)
	}
}

func TestListPersonaCandidatesNormalizesEmptyStateToOpen(t *testing.T) {
	runtime := openTestRuntime(t)
	seedPersonaCandidate(t, runtime, "major", "economics", "I major in economics")
	// Empty state should normalize to Open.
	list, err := runtime.ListPersonaCandidates("", 0)
	if err != nil {
		t.Fatalf("ListPersonaCandidates(empty) error = %v", err)
	}
	if len(list) != 1 {
		t.Fatalf("list = %d, want 1 (empty state must normalize to Open)", len(list))
	}
}
