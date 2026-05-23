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
// substitutes its Drafts() with a wrapper whose UpdateDraftState
// always returns the injected error. Used by
// TestApplyDraftEmitsGovernanceFindingWhenStateUpdateFails to
// drive the architect-flagged "vault write succeeded but state
// persistence failed" recovery path without simulating an actual
// disk failure on the store.
type stateUpdateFailingStore struct {
	inner     store.StateStore
	updateErr error
}

func (s *stateUpdateFailingStore) Drafts() store.DraftStore {
	return &failingDraftStore{inner: s.inner.Drafts(), updateErr: s.updateErr}
}
func (s *stateUpdateFailingStore) ProcessSink() store.ProcessSinkStore { return s.inner.ProcessSink() }
func (s *stateUpdateFailingStore) Audit() store.AuditStore             { return s.inner.Audit() }
func (s *stateUpdateFailingStore) Findings() store.FindingStore        { return s.inner.Findings() }
func (s *stateUpdateFailingStore) Usage() store.UsageStore             { return s.inner.Usage() }
func (s *stateUpdateFailingStore) Cursors() store.CursorStore          { return s.inner.Cursors() }
func (s *stateUpdateFailingStore) PersonaCandidates() store.PersonaCandidateStore {
	return s.inner.PersonaCandidates()
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

// Compile-time guard that stateUpdateFailingStore satisfies
// store.StateStore. Without this, a future interface addition
// would only fail at the New() call site inside the test.
var _ store.StateStore = (*stateUpdateFailingStore)(nil)
var _ store.DraftStore = (*failingDraftStore)(nil)

// Reference fmt so the file-level imports include it even if no
// other reference remains. Kept as a tiny placeholder so a
// future refactor that drops one of the fmt usages still
// compiles cleanly.
var _ = fmt.Sprintf
