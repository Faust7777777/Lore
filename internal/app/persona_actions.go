package app

import (
	"fmt"
	"strings"

	"obsidian-harness/internal/persona"
)

// PersonaCandidateActions reports which lifecycle actions the operator
// can perform on a single persona candidate. Each CanX boolean is the
// authoritative "is this button live?" signal; the paired XReason
// string carries a short, TUI-displayable explanation when CanX is
// false. When CanX is true, the matching reason is empty.
//
// The four canonical actions mirror the runtime entry points:
//
//   - Draft   : CreatePersonaDraftFromCandidate (Open candidate -> persona_update draft).
//   - Dismiss : DismissPersonaCandidate (Open candidate -> tombstone).
//   - Recover : RecoverPersonaCandidateLink or ForceDismissPartialPersonaCandidate
//               (partial-orphan State=Drafted with empty DraftID -> linked or abandoned).
//   - Retry   : RetryRejectedPersonaDraft (linked terminal draft -> fresh proposal).
//
// Force-dismiss of a partial-orphan is folded into Recover, not
// Dismiss: from a TUI standpoint Recover opens a reconciliation
// sub-flow whose two paths (link / force-dismiss) share the same
// entry button, while Dismiss stays the clean-state abandon path.
type PersonaCandidateActions struct {
	CanDraft      bool
	DraftReason   string
	CanDismiss    bool
	DismissReason string
	CanRecover    bool
	RecoverReason string
	CanRetry      bool
	RetryReason   string
}

// PersonaCandidateActions returns the action-availability bundle for
// rec. For drafted-and-linked candidates the helper looks up the
// linked draft to derive Retry availability per the
// RetryRejectedPersonaDraft contract (only rejected / expired /
// superseded drafts qualify). Draft-lookup failures or missing
// runtime harness collapse every action to false with a populated
// reason so the TUI degrades to a read-only display rather than
// rendering buttons that would error on click.
//
// Single source of truth: both CLI hint blocks and the TUI button
// row must derive availability from this helper instead of
// re-implementing the state machine. Adding a new lifecycle action
// later means extending PersonaCandidateActions and this function
// together; the consumer surfaces stay decoupled.
func (r *Runtime) PersonaCandidateActions(rec persona.PersonaCandidateRecord) PersonaCandidateActions {
	actions := PersonaCandidateActions{}
	switch rec.State {
	case persona.PersonaCandidateOpen:
		actions.CanDraft = true
		actions.CanDismiss = true
		actions.RecoverReason = "candidate is open; recover only mends partial Drafted state"
		actions.RetryReason = "candidate has no linked draft yet"

	case persona.PersonaCandidateDrafted:
		if strings.TrimSpace(rec.DraftID) == "" {
			// Partial-orphan: mark-Drafted succeeded but the link step
			// did not. Recover is the entry point for both link and
			// force-dismiss sub-flows.
			actions.DraftReason = "already drafted in partial state; use recover to link or force-dismiss"
			actions.DismissReason = "partial state; use recover to link or force-dismiss"
			actions.CanRecover = true
			actions.RetryReason = "no linked draft to retry; use recover --link first"
			return actions
		}
		// Linked: derive Retry from the draft's terminal status.
		actions.DraftReason = "already drafted; act on draft " + rec.DraftID
		actions.RecoverReason = "already linked to draft " + rec.DraftID
		if r == nil || r.Harness == nil {
			msg := fmt.Sprintf("linked draft %s present; runtime harness unavailable", rec.DraftID)
			actions.DismissReason = msg
			actions.RetryReason = msg
			return actions
		}
		draft, err := r.Harness.GetDraft(rec.DraftID)
		if err != nil {
			msg := fmt.Sprintf("linked draft %s lookup failed: %v", rec.DraftID, err)
			actions.DismissReason = msg
			actions.RetryReason = msg
			return actions
		}
		actions.DismissReason = fmt.Sprintf("linked draft %s is %s; act on the draft instead", rec.DraftID, draft.State)
		if isTerminalDraftStateForRetry(draft.State) {
			actions.CanRetry = true
		} else {
			actions.RetryReason = fmt.Sprintf("linked draft %s is %s (not a terminal rejected/expired/superseded state)", rec.DraftID, draft.State)
		}

	case persona.PersonaCandidateDismissed:
		actions.DraftReason = "candidate is dismissed (tombstone)"
		actions.DismissReason = "already dismissed"
		actions.RecoverReason = "candidate is dismissed"
		actions.RetryReason = "candidate is dismissed"

	default:
		// Unknown state: refuse everything so the TUI does not present
		// buttons against a state machine the helper does not understand.
		msg := fmt.Sprintf("unknown persona candidate state %q", rec.State)
		actions.DraftReason = msg
		actions.DismissReason = msg
		actions.RecoverReason = msg
		actions.RetryReason = msg
	}
	return actions
}
