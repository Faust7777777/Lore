package orchestrator

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"obsidian-harness/internal/config"
	"obsidian-harness/internal/model"
	"obsidian-harness/internal/store"
	"obsidian-harness/internal/store/memory"
)

// stateUpdateFailingStore wraps an underlying StateStore but
// substitutes its Drafts() and Findings() accessors with
// wrappers whose mutation methods return injected errors. Used
// to drive both the architect-flagged
// "write-succeeded/state-fail" recovery path and the
// reviewer-flagged "Finding emit ALSO fails" path without
// simulating actual disk failures on the underlying store.
//
// Either error may be nil to leave that branch healthy; setting
// only updateErr exercises the standard recovery path, setting
// both exercises the double-failure path.
type stateUpdateFailingStore struct {
	inner      store.StateStore
	updateErr  error
	findingErr error
}

func (s *stateUpdateFailingStore) Drafts() store.DraftStore {
	return &failingDraftStore{inner: s.inner.Drafts(), updateErr: s.updateErr}
}
func (s *stateUpdateFailingStore) ProcessSink() store.ProcessSinkStore { return s.inner.ProcessSink() }
func (s *stateUpdateFailingStore) Audit() store.AuditStore             { return s.inner.Audit() }
func (s *stateUpdateFailingStore) Findings() store.FindingStore {
	if s.findingErr == nil {
		return s.inner.Findings()
	}
	return &failingFindingStore{inner: s.inner.Findings(), saveErr: s.findingErr}
}
func (s *stateUpdateFailingStore) Usage() store.UsageStore   { return s.inner.Usage() }
func (s *stateUpdateFailingStore) Cursors() store.CursorStore { return s.inner.Cursors() }
func (s *stateUpdateFailingStore) PersonaCandidates() store.PersonaCandidateStore {
	return s.inner.PersonaCandidates()
}

// failingFindingStore lets every read/list call through but
// rejects SaveFinding with the injected error. Used by the
// double-failure test below.
type failingFindingStore struct {
	inner   store.FindingStore
	saveErr error
}

func (f *failingFindingStore) SaveFinding(model.Finding) error { return f.saveErr }
func (f *failingFindingStore) GetFinding(id string) (model.Finding, error) {
	return f.inner.GetFinding(id)
}
func (f *failingFindingStore) ListFindings(limit int) ([]model.Finding, error) {
	return f.inner.ListFindings(limit)
}
func (f *failingFindingStore) UpdateFindingState(id string, state model.FindingState, updatedAt time.Time) (model.Finding, error) {
	return f.inner.UpdateFindingState(id, state, updatedAt)
}

type failingDraftStore struct {
	inner     store.DraftStore
	updateErr error
}

func (f *failingDraftStore) SaveDraft(d model.Draft) error            { return f.inner.SaveDraft(d) }
func (f *failingDraftStore) GetDraft(id string) (model.Draft, error)  { return f.inner.GetDraft(id) }
func (f *failingDraftStore) ListDrafts() ([]model.Draft, error)       { return f.inner.ListDrafts() }
func (f *failingDraftStore) UpdateDraftState(id string, state model.DraftState, updatedAt time.Time) (model.Draft, error) {
	return model.Draft{}, f.updateErr
}
func (f *failingDraftStore) SupersedeDraft(oldID string, newDraft model.Draft, updatedAt time.Time) (model.Draft, model.Draft, error) {
	return f.inner.SupersedeDraft(oldID, newDraft, updatedAt)
}

func TestApplyDraftEmitsGovernanceFindingWhenStateUpdateFails(t *testing.T) {
	// Architect-flagged P0: when ApplyDraft completes the vault
	// write but the store cannot persist State=applied, the
	// previous behavior left the vault changed while the audit
	// trail had no signal. After the fix we:
	//
	//   - return a self-describing error pointing the operator
	//     at `lore findings list`;
	//   - emit a governance Finding (kind=
	//     governance_review_needed, severity=critical) so the
	//     operator can see the inconsistency the next time they
	//     look at the findings dashboard.
	workDir := t.TempDir()
	cfg := config.Default(workDir)
	inner := memory.New()

	// Bootstrap the vault layout + create an approved draft on
	// the real inner store (the failing wrapper only mediates
	// the second Harness instance used for the actual Apply
	// call).
	bootstrap, err := New(cfg, inner)
	if err != nil {
		t.Fatalf("bootstrap New: %v", err)
	}
	if _, err := bootstrap.BootstrapManagedVault(time.Date(2026, 4, 22, 9, 0, 0, 0, time.UTC)); err != nil {
		t.Fatalf("BootstrapManagedVault: %v", err)
	}
	relPath := filepath.Join("0-排期", "04-执行", "week.md")
	draft, err := bootstrap.ObserveDocumentChange(relPath, []byte("first pass"), time.Date(2026, 4, 22, 10, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatalf("ObserveDocumentChange: %v", err)
	}
	if _, err := bootstrap.ApproveDraft(draft.ID, time.Date(2026, 4, 22, 10, 5, 0, 0, time.UTC)); err != nil {
		t.Fatalf("ApproveDraft: %v", err)
	}

	// Construct a second Harness sharing the same data but
	// routing through the failing wrapper. ApplyDraft will write
	// the vault successfully and then fail on UpdateDraftState.
	injected := errors.New("simulated state-update failure")
	failingHarness, err := New(cfg, &stateUpdateFailingStore{inner: inner, updateErr: injected})
	if err != nil {
		t.Fatalf("failing New: %v", err)
	}

	progressAbs := filepath.Join(cfg.Paths.VaultRoot, cfg.Vault.ManagedCore.ProgressIndex)
	before, err := os.ReadFile(progressAbs)
	if err != nil {
		t.Fatalf("ReadFile(progressAbs, before): %v", err)
	}

	_, applyErr := failingHarness.ApplyDraft(draft.ID, time.Date(2026, 4, 22, 10, 6, 0, 0, time.UTC))
	if applyErr == nil {
		t.Fatal("ApplyDraft should error when state update fails after vault write")
	}
	for _, want := range []string{
		"vault write succeeded but draft state update failed",
		"governance Finding was emitted",
		"lore findings list",
	} {
		if !strings.Contains(applyErr.Error(), want) {
			t.Fatalf("err = %v, missing %q", applyErr, want)
		}
	}
	if !errors.Is(applyErr, injected) {
		t.Fatalf("err = %v, should wrap the injected underlying error %v", applyErr, injected)
	}

	// Vault must have the new content (the write part of
	// ApplyDraft happened before the state update was attempted).
	after, err := os.ReadFile(progressAbs)
	if err != nil {
		t.Fatalf("ReadFile(progressAbs, after): %v", err)
	}
	// ObserveDocumentChange produces a progress_sync draft whose
	// patch updates the managed progress-index table rather than
	// echoing the raw payload. We assert that the post-apply
	// content differs from the pre-apply baseline AND that the
	// document's row carries the observation timestamp the patch
	// is supposed to write -- that combination proves the write
	// actually happened even though the state update was rejected.
	if string(after) == string(before) {
		t.Fatalf("expected vault content to change despite state-fail; before=%q after=%q", string(before), string(after))
	}
	if !strings.Contains(string(after), "2026-04-22 10:00") {
		t.Fatalf("vault content missing the proposed observation timestamp 2026-04-22 10:00:\n%s", string(after))
	}

	// Draft state must remain Approved (transition was rejected).
	stalled, err := inner.Drafts().GetDraft(draft.ID)
	if err != nil {
		t.Fatalf("post-fail GetDraft: %v", err)
	}
	if stalled.State != model.DraftApproved {
		t.Fatalf("draft state = %q, want approved (state update failed so it should not have transitioned)", stalled.State)
	}

	// A governance Finding must have landed on the inner store
	// with kind/severity/state/source set so `lore findings
	// list` shows it prominently.
	findings, err := inner.Findings().ListFindings(0)
	if err != nil {
		t.Fatalf("ListFindings: %v", err)
	}
	var found *model.Finding
	for i := range findings {
		if findings[i].Source == "apply_draft_state_fail" {
			found = &findings[i]
			break
		}
	}
	if found == nil {
		t.Fatalf("no apply_draft_state_fail finding emitted; findings = %+v", findings)
	}
	if found.Kind != model.FindingGovernanceReviewNeeded {
		t.Fatalf("finding kind = %q, want %q", found.Kind, model.FindingGovernanceReviewNeeded)
	}
	if found.Severity != model.FindingSeverityCritical {
		t.Fatalf("finding severity = %q, want critical", found.Severity)
	}
	if found.State != model.FindingOpen {
		t.Fatalf("finding state = %q, want open", found.State)
	}
	if found.Metadata["draft_id"] != draft.ID {
		t.Fatalf("finding draft_id = %q, want %q", found.Metadata["draft_id"], draft.ID)
	}
	if !strings.Contains(found.Detail, "simulated state-update failure") {
		t.Fatalf("finding Detail should embed the underlying error; got %q", found.Detail)
	}
	if !strings.Contains(found.Summary, draft.ID) {
		t.Fatalf("finding Summary should name the draft id; got %q", found.Summary)
	}
}

func TestApplyDraftHappyPathDoesNotEmitFinding(t *testing.T) {
	// Sanity check: the normal happy path must not leave a
	// stray finding behind. Regression guard in case a future
	// refactor accidentally fires recordApplyStateFailFinding
	// on the success path.
	workDir := t.TempDir()
	cfg := config.Default(workDir)
	inner := memory.New()
	h, err := New(cfg, inner)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if _, err := h.BootstrapManagedVault(time.Date(2026, 4, 22, 9, 0, 0, 0, time.UTC)); err != nil {
		t.Fatalf("BootstrapManagedVault: %v", err)
	}
	relPath := filepath.Join("0-排期", "04-执行", "week.md")
	draft, err := h.ObserveDocumentChange(relPath, []byte("happy path"), time.Date(2026, 4, 22, 10, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatalf("ObserveDocumentChange: %v", err)
	}
	if _, err := h.ApproveDraft(draft.ID, time.Date(2026, 4, 22, 10, 5, 0, 0, time.UTC)); err != nil {
		t.Fatalf("ApproveDraft: %v", err)
	}
	if _, err := h.ApplyDraft(draft.ID, time.Date(2026, 4, 22, 10, 6, 0, 0, time.UTC)); err != nil {
		t.Fatalf("ApplyDraft: %v", err)
	}

	findings, err := inner.Findings().ListFindings(0)
	if err != nil {
		t.Fatalf("ListFindings: %v", err)
	}
	for _, f := range findings {
		if f.Source == "apply_draft_state_fail" {
			t.Fatalf("happy-path ApplyDraft should NOT emit an apply_state_fail finding; got %+v", f)
		}
	}
}

func TestApplyDraftSurfacesFindingSaveFailureInError(t *testing.T) {
	// Reviewer-flagged Medium (round 2): if SaveFinding ALSO fails
	// while emitting the recovery Finding, the previous code path
	// silently swallowed the SaveFinding error and the returned
	// apply error still told the operator a Finding had been
	// emitted -- a false recovery signal. This test pins the
	// corrected behavior: the apply error now includes BOTH
	// failures and explicitly says no automatic recovery record
	// exists.
	workDir := t.TempDir()
	cfg := config.Default(workDir)
	inner := memory.New()

	bootstrap, err := New(cfg, inner)
	if err != nil {
		t.Fatalf("bootstrap New: %v", err)
	}
	if _, err := bootstrap.BootstrapManagedVault(time.Date(2026, 4, 22, 9, 0, 0, 0, time.UTC)); err != nil {
		t.Fatalf("BootstrapManagedVault: %v", err)
	}
	relPath := filepath.Join("0-排期", "04-执行", "week.md")
	draft, err := bootstrap.ObserveDocumentChange(relPath, []byte("first pass"), time.Date(2026, 4, 22, 10, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatalf("ObserveDocumentChange: %v", err)
	}
	if _, err := bootstrap.ApproveDraft(draft.ID, time.Date(2026, 4, 22, 10, 5, 0, 0, time.UTC)); err != nil {
		t.Fatalf("ApproveDraft: %v", err)
	}

	injectedState := errors.New("simulated state-update failure")
	injectedSave := errors.New("simulated SaveFinding failure")
	doubleFailHarness, err := New(cfg, &stateUpdateFailingStore{
		inner:      inner,
		updateErr:  injectedState,
		findingErr: injectedSave,
	})
	if err != nil {
		t.Fatalf("double-fail New: %v", err)
	}

	_, applyErr := doubleFailHarness.ApplyDraft(draft.ID, time.Date(2026, 4, 22, 10, 6, 0, 0, time.UTC))
	if applyErr == nil {
		t.Fatal("ApplyDraft should error in the double-failure scenario")
	}

	// Apply error must surface BOTH the original state-update
	// failure AND the SaveFinding failure, and must NOT claim a
	// Finding was emitted.
	if !errors.Is(applyErr, injectedState) {
		t.Fatalf("err = %v, must still wrap the original state-update error", applyErr)
	}
	for _, want := range []string{
		"vault write succeeded but draft state update failed",
		"ALSO failed to emit governance Finding",
		injectedSave.Error(),
		"no automatic recovery record exists",
	} {
		if !strings.Contains(applyErr.Error(), want) {
			t.Fatalf("err = %v, missing %q", applyErr, want)
		}
	}
	if strings.Contains(applyErr.Error(), "Finding was emitted, run `lore findings list`") {
		t.Fatalf("err = %v, must not claim a Finding was emitted when SaveFinding failed", applyErr)
	}

	// The findings table on the inner store must be empty -- the
	// failingFindingStore rejected the SaveFinding call so no
	// dashboard record landed.
	findings, err := inner.Findings().ListFindings(0)
	if err != nil {
		t.Fatalf("ListFindings: %v", err)
	}
	for _, f := range findings {
		if f.Source == "apply_draft_state_fail" {
			t.Fatalf("a finding leaked into store despite the injected SaveFinding failure: %+v", f)
		}
	}
}

func TestMarkDraftConflictedSurfacesStatePersistFailure(t *testing.T) {
	// Phase 5 governance hardening: the apply guard can detect a
	// conflict (the target changed out-of-band so its hash no longer
	// matches the draft's BaseVersion) and then fail to PERSIST
	// State=conflicted. The previous markDraftConflicted swallowed that
	// store error and still returned a clean store.ErrConflict, so the
	// operator was told "conflict" while `drafts list` kept showing the
	// draft as approved -- a silently divergent governance view that
	// violates the "mutations fail loud" invariant recorded in
	// docs/review-b-line-failure-semantics-2026-06-04.md.
	//
	// After the fix the returned error must still satisfy
	// errors.Is(store.ErrConflict) for callers that branch on it (e.g.
	// persona candidate re-routing), AND also wrap the underlying
	// persist error so the operator learns the conflict state was NOT
	// recorded and the draft is still approved.
	workDir := t.TempDir()
	cfg := config.Default(workDir)
	inner := memory.New()

	bootstrap, err := New(cfg, inner)
	if err != nil {
		t.Fatalf("bootstrap New: %v", err)
	}
	if _, err := bootstrap.BootstrapManagedVault(time.Date(2026, 4, 22, 9, 0, 0, 0, time.UTC)); err != nil {
		t.Fatalf("BootstrapManagedVault: %v", err)
	}
	relPath := filepath.Join("0-排期", "04-执行", "week.md")
	draft, err := bootstrap.ObserveDocumentChange(relPath, []byte("first pass"), time.Date(2026, 4, 22, 10, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatalf("ObserveDocumentChange: %v", err)
	}
	if _, err := bootstrap.ApproveDraft(draft.ID, time.Date(2026, 4, 22, 10, 5, 0, 0, time.UTC)); err != nil {
		t.Fatalf("ApproveDraft: %v", err)
	}

	// Mutate the draft's target (the managed progress index) out-of-band
	// so its hash no longer matches draft.Target.BaseVersion -- the apply
	// guard will then route through markDraftConflicted.
	progressAbs := filepath.Join(cfg.Paths.VaultRoot, filepath.FromSlash(cfg.Vault.ManagedCore.ProgressIndex))
	if err := os.WriteFile(progressAbs, []byte("# changed out of band\n"), 0o644); err != nil {
		t.Fatalf("WriteFile(progress out-of-band): %v", err)
	}

	injected := errors.New("simulated conflict-state persist failure")
	failingHarness, err := New(cfg, &stateUpdateFailingStore{inner: inner, updateErr: injected})
	if err != nil {
		t.Fatalf("failing New: %v", err)
	}

	_, applyErr := failingHarness.ApplyDraft(draft.ID, time.Date(2026, 4, 22, 10, 6, 0, 0, time.UTC))
	if applyErr == nil {
		t.Fatal("ApplyDraft should error when the conflict state cannot be persisted")
	}
	// Callers still branch on ErrConflict, so it must remain in the chain.
	if !errors.Is(applyErr, store.ErrConflict) {
		t.Fatalf("err = %v, must still satisfy errors.Is(store.ErrConflict)", applyErr)
	}
	// But it must NOT hide the persist failure behind a clean conflict.
	if !errors.Is(applyErr, injected) {
		t.Fatalf("err = %v, must wrap the underlying state-persist error %v", applyErr, injected)
	}
	if !strings.Contains(applyErr.Error(), "approved") {
		t.Fatalf("err = %v, should warn the operator the draft is still approved", applyErr)
	}

	// The draft must remain approved in the store (the persist failed),
	// matching what the error now tells the operator -- no silent claim
	// that it became conflicted.
	stalled, err := inner.Drafts().GetDraft(draft.ID)
	if err != nil {
		t.Fatalf("GetDraft: %v", err)
	}
	if stalled.State != model.DraftApproved {
		t.Fatalf("draft state = %q, want approved (conflict persist failed)", stalled.State)
	}
}

// Compile-time guard that stateUpdateFailingStore satisfies
// store.StateStore. Without this, a future interface addition
// would only fail at the New() call site inside the test.
var _ store.StateStore = (*stateUpdateFailingStore)(nil)
var _ store.DraftStore = (*failingDraftStore)(nil)
var _ store.FindingStore = (*failingFindingStore)(nil)

// Reference fmt so the file-level imports include it even if no
// other reference remains. Kept as a tiny placeholder so a
// future refactor that drops one of the fmt usages still
// compiles cleanly.
var _ = fmt.Sprintf
