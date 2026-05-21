package app

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"obsidian-harness/internal/model"
	"obsidian-harness/internal/persona"
	"obsidian-harness/internal/store"
)

// ErrPersonaCandidateAlreadyDrafted is returned by
// CreatePersonaDraftFromCandidate when the candidate has already been
// promoted to a draft. P5 deliberately rejects re-drafting the same
// candidate so two parallel review tracks cannot diverge; if the
// operator wants a fresh draft they must dismiss the first.
var ErrPersonaCandidateAlreadyDrafted = errors.New("app: persona candidate already drafted")

// ErrPersonaCandidateDismissed is returned by
// CreatePersonaDraftFromCandidate when the candidate has been
// dismissed. Dismissed candidates remain in store as dedup evidence
// but must not be reopened into drafts directly; the operator must
// re-extract by repeating the underlying user turn.
var ErrPersonaCandidateDismissed = errors.New("app: persona candidate is dismissed")

// ErrPersonaCandidatePartialStateRequired is returned by
// RecoverPersonaCandidateLink and ForceDismissPartialPersonaCandidate
// when the candidate is NOT in the partial state these recovery
// commands target (State=Drafted && DraftID==""). A candidate in Open,
// Drafted+linked, or Dismissed has no partial-failure scar to mend.
var ErrPersonaCandidatePartialStateRequired = errors.New("app: persona candidate must be drafted with empty DraftID for recovery")

// ErrPersonaCandidateLinkedStateRequired is returned by
// RetryRejectedPersonaDraft when the candidate is NOT in the linked
// drafted state (State=Drafted && DraftID!=""). Retrying a rejected
// draft only makes sense when there is a previous draft to retry.
var ErrPersonaCandidateLinkedStateRequired = errors.New("app: persona candidate must be drafted with non-empty DraftID for retry-rejected")

// ErrPersonaDraftNotTerminalForRetry is returned by
// RetryRejectedPersonaDraft when the linked draft has not reached a
// terminal review state (rejected, expired, or superseded). A retry
// must not race a still-pending review.
var ErrPersonaDraftNotTerminalForRetry = errors.New("app: linked persona draft is not in a terminal rejected/expired/superseded state")

// ErrPersonaDraftKindMismatch is returned by
// RecoverPersonaCandidateLink when the --link target is not a
// persona_update draft. Linking a candidate to an unrelated draft
// would silently break the candidate -> draft contract.
var ErrPersonaDraftKindMismatch = errors.New("app: target draft is not a persona_update draft")

// RecordPersonaCandidate persists one persona candidate via the
// runtime's store. Returns the stored record, an isNew flag (false
// when the DedupKey already mapped to an existing row, see
// store.PersonaCandidateStore.UpsertCandidate), and any store-side
// error. The console fire-and-forget extraction goroutine (P4) calls
// this -- callers expecting to bill the work should NOT depend on
// the bool, only the error.
func (r *Runtime) RecordPersonaCandidate(record persona.PersonaCandidateRecord) (persona.PersonaCandidateRecord, bool, error) {
	if r == nil || r.Store == nil {
		return persona.PersonaCandidateRecord{}, false, fmt.Errorf("app: runtime is not initialized")
	}
	return r.Store.PersonaCandidates().UpsertCandidate(record)
}

// GetPersonaCandidate returns the candidate with the given ID, or
// ErrNotFound when no such candidate exists. Thin passthrough to
// store; exists so CLI / TUI callers do not have to reach into the
// store package directly.
func (r *Runtime) GetPersonaCandidate(id string) (persona.PersonaCandidateRecord, error) {
	if r == nil || r.Store == nil {
		return persona.PersonaCandidateRecord{}, fmt.Errorf("app: runtime is not initialized")
	}
	return r.Store.PersonaCandidates().GetCandidate(strings.TrimSpace(id))
}

// ListPersonaCandidates returns up to limit candidates in the given
// state (newest UpdatedAt first). Empty state is normalized to
// PersonaCandidateOpen so callers can pass the zero value to mean
// "default queue".
func (r *Runtime) ListPersonaCandidates(state persona.PersonaCandidateState, limit int) ([]persona.PersonaCandidateRecord, error) {
	if r == nil || r.Store == nil {
		return nil, fmt.Errorf("app: runtime is not initialized")
	}
	return r.Store.PersonaCandidates().ListCandidatesByState(persona.NormalizeCandidateState(state), limit)
}

// DismissPersonaCandidate transitions a candidate to the Dismissed
// state. Dismissed candidates stay in store so their DedupKey still
// blocks re-emission, but they are no longer surfaced in the Open
// review queue. Errors:
//
//   - store.ErrNotFound when the ID does not exist.
//   - ErrPersonaCandidateAlreadyDrafted when the candidate already
//     produced a draft (operator should act on the draft, not the
//     candidate).
func (r *Runtime) DismissPersonaCandidate(id string, now time.Time) (persona.PersonaCandidateRecord, error) {
	if r == nil || r.Store == nil {
		return persona.PersonaCandidateRecord{}, fmt.Errorf("app: runtime is not initialized")
	}
	id = strings.TrimSpace(id)
	current, err := r.Store.PersonaCandidates().GetCandidate(id)
	if err != nil {
		return persona.PersonaCandidateRecord{}, err
	}
	switch current.State {
	case persona.PersonaCandidateDrafted:
		return persona.PersonaCandidateRecord{}, ErrPersonaCandidateAlreadyDrafted
	case persona.PersonaCandidateDismissed:
		// Idempotent: already dismissed, return current.
		return current, nil
	}
	return r.Store.PersonaCandidates().UpdateCandidateState(id, persona.PersonaCandidateDismissed, now)
}

// buildPersonaProposalFromCandidate translates a PersonaCandidate
// into the harness PersonaUpdateProposal contract. Shared by
// CreatePersonaDraftFromCandidate and RetryRejectedPersonaDraft so
// both code paths emit byte-identical proposals from the same
// candidate data; divergence here would mean a retry-rejected draft
// could fail validation that the original draft passed.
func buildPersonaProposalFromCandidate(c persona.PersonaCandidate) model.PersonaUpdateProposal {
	return model.PersonaUpdateProposal{
		Field:         c.Field,
		CurrentValue:  c.CurrentValue,
		ProposedValue: c.ProposedValue,
		Evidence:      c.EvidenceQuote,
		Reason:        c.Reason,
		Confidence:    string(c.Confidence),
		Source:        string(c.SourceKind),
		ObservedAt:    c.ObservedAt,
	}
}

// CreatePersonaDraftFromCandidate promotes a stored candidate into a
// persona_update draft by funneling it through the existing
// harness.ProposePersonaUpdate governance path -- the same path the
// external MCP persona_update_propose tool uses. This is deliberate:
// every persona update, regardless of origin (LLM mining vs external
// agent submission vs operator-initiated), lands as a Draft in
// pending_review state and goes through the same target / class /
// forge governance checks. P5 does NOT approve or apply the draft;
// that remains a local Lore/runtime action triggered by the operator.
//
// Duplicate-prevention contract (load-bearing): the candidate is
// transitioned to Drafted BEFORE the proposal call. A retry that
// sees State == Drafted refuses to re-propose regardless of whether
// DraftID is populated -- this is what makes blind retries safe.
//
//   - State == Drafted && DraftID != "" : idempotent return of the
//     existing draft (operator can keep re-invoking; no churn).
//   - State == Drafted && DraftID == "" : a prior attempt failed
//     somewhere between mark-Drafted and link-DraftID; the candidate
//     is in a partial state and the runtime refuses to propose again.
//     Operator must reconcile manually (find / dismiss the orphan
//     draft, or set DraftID on the candidate). Returns
//     ErrPersonaCandidateAlreadyDrafted to signal this.
//
// Side effects on first invocation:
//   - candidate transitions to Drafted (DraftID still empty) BEFORE
//     proposal so retries cannot duplicate.
//   - one Draft created via store.Drafts().SaveDraft (with summary
//     augmented to include evidence/reason/confidence/conflict from
//     the candidate).
//   - one AuditDraftCreated record (from ProposePersonaUpdate).
//   - one Event broadcast on the runtime broker.
//   - candidate.DraftID linked via store.LinkCandidateDraft.
//
// Errors:
//   - store.ErrNotFound when the candidate ID does not exist.
//   - ErrPersonaCandidateAlreadyDrafted when the candidate is in
//     Drafted state but DraftID is empty (partial-failure state from
//     a prior attempt; requires manual operator action).
//   - ErrPersonaCandidateDismissed when the candidate was dismissed
//     (cannot revive directly).
//   - any harness.ProposePersonaUpdate validation failure (e.g.
//     missing field, invalid confidence) -- the runtime rolls the
//     candidate state back to Open so retry can succeed normally.
//   - a wrapped error when the draft was created but
//     LinkCandidateDraft failed afterward; the candidate stays in
//     Drafted/unlinked and a retry refuses to re-propose, preventing
//     duplicate drafts. Operator must reconcile manually.
func (r *Runtime) CreatePersonaDraftFromCandidate(id string, now time.Time) (persona.PersonaCandidateRecord, model.PersonaUpdateProposalResult, error) {
	if r == nil || r.Store == nil || r.Harness == nil {
		return persona.PersonaCandidateRecord{}, model.PersonaUpdateProposalResult{}, fmt.Errorf("app: runtime is not initialized")
	}
	id = strings.TrimSpace(id)
	current, err := r.Store.PersonaCandidates().GetCandidate(id)
	if err != nil {
		return persona.PersonaCandidateRecord{}, model.PersonaUpdateProposalResult{}, err
	}
	// Fast pre-check: terminal states bypass the claim path entirely.
	// The same evaluator runs again after a claim-conflict so two
	// concurrent callers route through identical idempotency logic
	// regardless of who lost the race.
	if result, evalErr, handled := r.evaluateNonOpenPersonaCandidate(current); handled {
		if evalErr != nil {
			return persona.PersonaCandidateRecord{}, model.PersonaUpdateProposalResult{}, evalErr
		}
		return current, result, nil
	}

	// Atomic Open -> Drafted (DraftID still empty) compare-and-set
	// in the store. Two concurrent callers can both observe state =
	// Open above; only one of them wins this CAS. The loser receives
	// store.ErrConflict and re-routes through the non-open evaluator.
	claimed, err := r.Store.PersonaCandidates().ClaimCandidateForDraft(id, now)
	if err != nil {
		if errors.Is(err, store.ErrConflict) {
			recheck, gerr := r.Store.PersonaCandidates().GetCandidate(id)
			if gerr != nil {
				return persona.PersonaCandidateRecord{}, model.PersonaUpdateProposalResult{}, gerr
			}
			if result, evalErr, handled := r.evaluateNonOpenPersonaCandidate(recheck); handled {
				if evalErr != nil {
					return persona.PersonaCandidateRecord{}, model.PersonaUpdateProposalResult{}, evalErr
				}
				return recheck, result, nil
			}
			// Claim returned conflict but the recheck saw Open: a
			// peer must have transitioned through and back during
			// the window. Surface a clear error so the caller can
			// retry rather than silently double-propose.
			return persona.PersonaCandidateRecord{}, model.PersonaUpdateProposalResult{}, fmt.Errorf("persona candidate %s claim conflicted but recheck saw state=%q; retry", id, recheck.State)
		}
		return persona.PersonaCandidateRecord{}, model.PersonaUpdateProposalResult{}, err
	}

	proposal := buildPersonaProposalFromCandidate(claimed.Candidate)
	result, err := r.Harness.ProposePersonaUpdate(proposal, now)
	if err != nil {
		// Propose failed; no draft created, no audit emitted.
		// Best-effort roll back the pre-mark so the operator can
		// retry cleanly. If the rollback itself fails, the candidate
		// is stuck in Drafted/unlinked and a retry will refuse to
		// re-propose -- which still prevents duplicates, just with
		// a worse UX requiring manual reconciliation.
		_, _ = r.Store.PersonaCandidates().UpdateCandidateState(id, persona.PersonaCandidateOpen, now)
		return persona.PersonaCandidateRecord{}, model.PersonaUpdateProposalResult{}, err
	}

	// Augment the just-created draft's Summary with the candidate's
	// evidence / reason / confidence / conflict so reviewers see
	// those fields without having to decode ProposedContent JSON.
	// Failure here is non-fatal: the draft exists with the default
	// summary, and the candidate-link below still runs.
	if augErr := r.augmentPersonaDraftSummary(result.DraftID, claimed.Candidate); augErr != nil {
		// Intentionally swallowed: draft is usable, just less rich.
		_ = augErr
	}

	updated, err := r.Store.PersonaCandidates().LinkCandidateDraft(id, result.DraftID, now)
	if err != nil {
		// Partial-failure window: the draft exists, was audited, and
		// was broadcast, but the candidate -> draft link did not
		// persist. The candidate is in Drafted/unlinked; a retry
		// will refuse to re-propose (the empty-DraftID branch above
		// returns ErrPersonaCandidateAlreadyDrafted), so this state
		// is duplicate-safe at the cost of operator reconciliation:
		// the operator must locate the orphan draft and either
		// dismiss it or wire DraftID on the candidate manually.
		return persona.PersonaCandidateRecord{}, result, fmt.Errorf("created draft %s but failed to link candidate %s: %w; the candidate is Drafted/unlinked; retry will refuse to re-propose, reconcile by dismissing the orphan draft or setting DraftID on the candidate", result.DraftID, id, err)
	}
	return updated, result, nil
}

// evaluateNonOpenPersonaCandidate inspects a candidate that is not in
// the Open state and returns the idempotent / refusal response that
// CreatePersonaDraftFromCandidate should emit. handled is true when
// the caller MUST return (either with the supplied result + nil err
// for idempotent reuse, or with the supplied err for a refusal). The
// helper runs in two spots: the fast pre-check path before the
// claim attempt, and the conflict-recovery path after the claim CAS
// loses the race -- both must route through identical logic so
// concurrent callers cannot observe inconsistent semantics.
func (r *Runtime) evaluateNonOpenPersonaCandidate(current persona.PersonaCandidateRecord) (model.PersonaUpdateProposalResult, error, bool) {
	switch current.State {
	case persona.PersonaCandidateDrafted:
		if strings.TrimSpace(current.DraftID) == "" {
			return model.PersonaUpdateProposalResult{}, ErrPersonaCandidateAlreadyDrafted, true
		}
		existing, err := r.Store.Drafts().GetDraft(current.DraftID)
		if err != nil {
			return model.PersonaUpdateProposalResult{}, fmt.Errorf("candidate %s already drafted (DraftID=%s) but draft lookup failed: %w", current.ID, current.DraftID, err), true
		}
		return model.PersonaUpdateProposalResult{
			Status:         "draft_created",
			DraftID:        current.DraftID,
			Target:         existing.Target.Path,
			ReviewRequired: existing.State == model.DraftPendingReview,
		}, nil, true
	case persona.PersonaCandidateDismissed:
		return model.PersonaUpdateProposalResult{}, ErrPersonaCandidateDismissed, true
	}
	return model.PersonaUpdateProposalResult{}, nil, false
}

// augmentPersonaDraftSummary fetches a freshly-created persona update
// draft and appends evidence / reason / confidence / conflict lines
// to its Summary so reviewers see the candidate's full justification
// without decoding ProposedContent JSON. Returns store.ErrNotFound
// when the draft has disappeared between proposal and augmentation
// (should not happen under normal flow).
func (r *Runtime) augmentPersonaDraftSummary(draftID string, candidate persona.PersonaCandidate) error {
	if r == nil || r.Store == nil {
		return fmt.Errorf("app: runtime is not initialized")
	}
	draft, err := r.Store.Drafts().GetDraft(draftID)
	if err != nil {
		return err
	}
	var b strings.Builder
	b.WriteString(strings.TrimSpace(draft.Summary))
	b.WriteString("\n\nCandidate evidence:\n")
	b.WriteString("- evidence_quote: ")
	b.WriteString(strings.TrimSpace(candidate.EvidenceQuote))
	if reason := strings.TrimSpace(candidate.Reason); reason != "" {
		b.WriteString("\n- reason: ")
		b.WriteString(reason)
	}
	b.WriteString("\n- confidence: ")
	b.WriteString(string(candidate.Confidence))
	b.WriteString("\n- conflict: ")
	if candidate.Conflict {
		b.WriteString("true")
	} else {
		b.WriteString("false")
	}
	draft.Summary = b.String()
	return r.Store.Drafts().SaveDraft(draft)
}

// RecoverPersonaCandidateLink repairs the partial-failure state that
// CreatePersonaDraftFromCandidate documents at lines 134-139: the
// proposal succeeded and a draft exists in the store, but the
// follow-up LinkCandidateDraft call did not persist. The candidate
// is stuck in Drafted with DraftID == "" and a normal retry refuses
// to re-propose, so without a recovery path the operator must reach
// into sqlite by hand.
//
// This method takes the orphan draft ID the operator has located
// (via `lore draft list` or audit log), verifies it exists and has
// kind == persona_update, then performs the deferred link. The
// candidate ends in Drafted with the supplied DraftID, indistinguishable
// from a clean first-attempt completion.
//
// Errors:
//   - store.ErrNotFound when the candidate ID does not exist.
//   - ErrPersonaCandidatePartialStateRequired when the candidate is
//     not in Drafted+empty (recover has nothing to repair).
//   - ErrPersonaDraftKindMismatch when the draft exists but is not a
//     persona_update draft (operator targeted the wrong draft).
//   - any harness draft lookup or store link error verbatim.
func (r *Runtime) RecoverPersonaCandidateLink(candidateID string, draftID string, now time.Time) (persona.PersonaCandidateRecord, error) {
	if r == nil || r.Store == nil || r.Harness == nil {
		return persona.PersonaCandidateRecord{}, fmt.Errorf("app: runtime is not initialized")
	}
	candidateID = strings.TrimSpace(candidateID)
	draftID = strings.TrimSpace(draftID)
	if draftID == "" {
		return persona.PersonaCandidateRecord{}, fmt.Errorf("app: draft id is required for recover --link")
	}
	current, err := r.Store.PersonaCandidates().GetCandidate(candidateID)
	if err != nil {
		return persona.PersonaCandidateRecord{}, err
	}
	if current.State != persona.PersonaCandidateDrafted || strings.TrimSpace(current.DraftID) != "" {
		return persona.PersonaCandidateRecord{}, ErrPersonaCandidatePartialStateRequired
	}
	draft, err := r.Harness.GetDraft(draftID)
	if err != nil {
		return persona.PersonaCandidateRecord{}, err
	}
	if draft.Kind != model.DraftKindPersonaUpdate {
		return persona.PersonaCandidateRecord{}, ErrPersonaDraftKindMismatch
	}
	return r.Store.PersonaCandidates().LinkCandidateDraft(candidateID, draftID, now)
}

// ForceDismissPartialPersonaCandidate is the abandonment counterpart
// to RecoverPersonaCandidateLink: instead of mending the partial
// scar by supplying the orphan DraftID, the operator decides to
// throw the partial state away (e.g. the orphan draft was already
// rejected via lore draft review, or the candidate is no longer
// worth pursuing). The candidate transitions to Dismissed, which
// keeps its DedupKey as a tombstone so the same fact does not
// re-emerge from the next console turn.
//
// This method ONLY operates on the partial state (State=Drafted &&
// DraftID==""). The normal Dismiss path (DismissPersonaCandidate)
// covers Open candidates and is idempotent on Dismissed; this method
// fills the gap that exists because DismissPersonaCandidate
// deliberately refuses Drafted candidates to avoid race-y reopens.
//
// Idempotent on already-dismissed: returning the existing record
// rather than failing matches the ergonomics of DismissPersonaCandidate
// so retrying the recover --force-dismiss CLI command is safe.
//
// Errors:
//   - store.ErrNotFound when the candidate ID does not exist.
//   - ErrPersonaCandidatePartialStateRequired when the candidate is
//     Open or Drafted+linked (those paths have their own commands).
func (r *Runtime) ForceDismissPartialPersonaCandidate(id string, now time.Time) (persona.PersonaCandidateRecord, error) {
	if r == nil || r.Store == nil {
		return persona.PersonaCandidateRecord{}, fmt.Errorf("app: runtime is not initialized")
	}
	id = strings.TrimSpace(id)
	current, err := r.Store.PersonaCandidates().GetCandidate(id)
	if err != nil {
		return persona.PersonaCandidateRecord{}, err
	}
	if current.State == persona.PersonaCandidateDismissed {
		return current, nil
	}
	if current.State != persona.PersonaCandidateDrafted || strings.TrimSpace(current.DraftID) != "" {
		return persona.PersonaCandidateRecord{}, ErrPersonaCandidatePartialStateRequired
	}
	return r.Store.PersonaCandidates().UpdateCandidateState(id, persona.PersonaCandidateDismissed, now)
}

// isTerminalDraftStateForRetry reports whether a draft is in a state
// that allows the operator to legitimately retry the candidate (the
// reviewer either rejected the original draft, it expired without
// review, or it was superseded by another draft). pending_review,
// approved, applied, revision_requested, and conflicted are all
// non-terminal for this purpose -- retrying then would either race
// the reviewer or duplicate already-accepted work.
func isTerminalDraftStateForRetry(state model.DraftState) bool {
	switch state {
	case model.DraftRejected, model.DraftExpired, model.DraftSuperseded:
		return true
	}
	return false
}

// RetryRejectedPersonaDraft handles the candidate-lifecycle case the
// P5+P6 contract intentionally left open: a candidate was promoted
// to a persona_update draft, the reviewer rejected (or it expired /
// was superseded), and the candidate is now stuck in Drafted with a
// DraftID that points at a dead draft. CreatePersonaDraftFromCandidate
// refuses to re-propose because it sees State=Drafted; without an
// explicit retry path the candidate's underlying fact is unrecoverable
// (its DedupKey would block re-extraction even if the user repeated
// the source utterance).
//
// This method gates the retry on three checks:
//
//   - candidate.State == Drafted (otherwise nothing to retry)
//   - candidate.DraftID != "" (otherwise recover --link is the right command)
//   - linked draft.State in {Rejected, Expired, Superseded}
//
// The third check is critical: retrying while the prior draft is
// still pending_review or already approved/applied would either race
// the reviewer or duplicate accepted work. A pending review must be
// reviewed; an approved / applied draft is not "retryable" -- the
// outcome already exists.
//
// On success: a fresh draft is created via the same
// harness.ProposePersonaUpdate path used by P5, augmented with the
// candidate's evidence summary, and LinkCandidateDraft overwrites
// the candidate's DraftID with the new draft ID. The previous
// (rejected/expired/superseded) draft is intentionally NOT mutated:
// it stays in the draft history as audit, and the candidate now
// points forward to the active retry.
//
// Errors:
//   - store.ErrNotFound when the candidate ID does not exist.
//   - ErrPersonaCandidateLinkedStateRequired when candidate.State is
//     not Drafted or DraftID is empty.
//   - ErrPersonaDraftNotTerminalForRetry when the linked draft is in
//     a non-terminal review state.
//   - any harness draft lookup, propose, or store link error verbatim;
//     the link error message includes the new draft ID so the operator
//     can reconcile manually (the new draft exists, just unlinked).
func (r *Runtime) RetryRejectedPersonaDraft(id string, now time.Time) (persona.PersonaCandidateRecord, model.PersonaUpdateProposalResult, error) {
	if r == nil || r.Store == nil || r.Harness == nil {
		return persona.PersonaCandidateRecord{}, model.PersonaUpdateProposalResult{}, fmt.Errorf("app: runtime is not initialized")
	}
	id = strings.TrimSpace(id)
	current, err := r.Store.PersonaCandidates().GetCandidate(id)
	if err != nil {
		return persona.PersonaCandidateRecord{}, model.PersonaUpdateProposalResult{}, err
	}
	if current.State != persona.PersonaCandidateDrafted || strings.TrimSpace(current.DraftID) == "" {
		return persona.PersonaCandidateRecord{}, model.PersonaUpdateProposalResult{}, ErrPersonaCandidateLinkedStateRequired
	}
	priorDraft, err := r.Harness.GetDraft(current.DraftID)
	if err != nil {
		return persona.PersonaCandidateRecord{}, model.PersonaUpdateProposalResult{}, err
	}
	if !isTerminalDraftStateForRetry(priorDraft.State) {
		return persona.PersonaCandidateRecord{}, model.PersonaUpdateProposalResult{}, fmt.Errorf("%w: linked draft %s is in state %q", ErrPersonaDraftNotTerminalForRetry, current.DraftID, priorDraft.State)
	}

	proposal := buildPersonaProposalFromCandidate(current.Candidate)
	result, err := r.Harness.ProposePersonaUpdate(proposal, now)
	if err != nil {
		return persona.PersonaCandidateRecord{}, model.PersonaUpdateProposalResult{}, err
	}
	if augErr := r.augmentPersonaDraftSummary(result.DraftID, current.Candidate); augErr != nil {
		// Non-fatal: the draft is usable, just less rich in summary.
		_ = augErr
	}
	updated, err := r.Store.PersonaCandidates().LinkCandidateDraft(id, result.DraftID, now)
	if err != nil {
		return persona.PersonaCandidateRecord{}, result, fmt.Errorf("retry created new persona draft %s but failed to relink candidate %s: %w; the candidate still points at the old terminal draft %s and the new draft is orphan, reconcile via `lore persona candidates recover --link %s` or by rejecting the orphan draft", result.DraftID, id, err, current.DraftID, result.DraftID)
	}
	return updated, result, nil
}
