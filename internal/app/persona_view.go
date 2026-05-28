package app

import (
	"time"

	"obsidian-harness/internal/persona"
)

// PersonaCandidateView is the TUI/CLI-facing projection of a stored
// persona candidate. It flattens the nested PersonaCandidateRecord ->
// PersonaCandidate shape into one struct, hides store-level metadata
// the UI does not need, and embeds the action-availability bundle so
// a list call returns everything the TUI needs to render a row + its
// button states in a single roundtrip.
//
// Stable JSON contract: field names mirror the on-the-wire shape
// `lore persona candidates list --json` already emits via cli, so
// JSON consumers can migrate to this view incrementally without
// reshaping their parsers. New optional fields land with omitempty so
// the existing shape stays additive.
type PersonaCandidateView struct {
	ID            string                        `json:"id"`
	State         persona.PersonaCandidateState `json:"state"`
	Field         string                        `json:"field"`
	ProposedValue string                        `json:"proposed_value"`
	CurrentValue  string                        `json:"current_value,omitempty"`
	EvidenceQuote string                        `json:"evidence_quote"`
	Reason        string                        `json:"reason,omitempty"`
	Confidence    persona.Confidence            `json:"confidence"`
	Source        persona.SourceKind            `json:"source"`
	Session       string                        `json:"session,omitempty"`
	ObservedAt    time.Time                     `json:"observed_at"`
	DraftID       string                        `json:"draft_id,omitempty"`
	Conflict      bool                          `json:"conflict"`
	// DedupKey is exposed for debug / detail panels so an operator
	// inspecting a candidate can see why a re-extraction was suppressed.
	// TUI list rows should treat it as secondary; detail pages may
	// surface it under an explicit "Debug" affordance.
	DedupKey string                  `json:"dedup_key"`
	Actions  PersonaCandidateActions `json:"actions"`
}

// newPersonaCandidateView assembles a view from a record + its
// pre-computed actions. Pre-computation matters in the list path so
// the Runtime can dispatch a single draft lookup per candidate rather
// than re-resolving inside the converter.
func newPersonaCandidateView(rec persona.PersonaCandidateRecord, actions PersonaCandidateActions) PersonaCandidateView {
	return PersonaCandidateView{
		ID:            rec.ID,
		State:         rec.State,
		Field:         rec.Candidate.Field,
		ProposedValue: rec.Candidate.ProposedValue,
		CurrentValue:  rec.Candidate.CurrentValue,
		EvidenceQuote: rec.Candidate.EvidenceQuote,
		Reason:        rec.Candidate.Reason,
		Confidence:    rec.Candidate.Confidence,
		Source:        rec.Candidate.SourceKind,
		Session:       rec.Candidate.SourceSessionID,
		ObservedAt:    rec.Candidate.ObservedAt,
		DraftID:       rec.DraftID,
		Conflict:      rec.Candidate.Conflict,
		DedupKey:      rec.DedupKey,
		Actions:       actions,
	}
}

// ListPersonaCandidateViews returns up to limit candidates in the
// given state, each enriched with its PersonaCandidateActions bundle
// derived from the record's current state and (for linked candidates)
// the linked draft's review status. Semantics for state / limit match
// ListPersonaCandidates verbatim.
//
// Lives alongside ListPersonaCandidates rather than replacing it so
// the record-returning surface stays available for automation /
// debugging consumers that need the raw store shape. Both methods
// share the same store query path; there is no second route.
func (r *Runtime) ListPersonaCandidateViews(state persona.PersonaCandidateState, limit int) ([]PersonaCandidateView, error) {
	records, err := r.ListPersonaCandidates(state, limit)
	if err != nil {
		return nil, err
	}
	views := make([]PersonaCandidateView, 0, len(records))
	for _, rec := range records {
		views = append(views, newPersonaCandidateView(rec, r.PersonaCandidateActions(rec)))
	}
	return views, nil
}

// GetPersonaCandidateView returns the candidate with the given ID as
// a view + its PersonaCandidateActions bundle. Same error contract as
// GetPersonaCandidate (store.ErrNotFound surfaces verbatim).
func (r *Runtime) GetPersonaCandidateView(id string) (PersonaCandidateView, error) {
	rec, err := r.GetPersonaCandidate(id)
	if err != nil {
		return PersonaCandidateView{}, err
	}
	return newPersonaCandidateView(rec, r.PersonaCandidateActions(rec)), nil
}
