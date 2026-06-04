package app

import (
	"time"

	"obsidian-harness/internal/model"
	"obsidian-harness/internal/persona"
)

// OperatorQueue is the unified "what needs my attention" view: the pending
// governance items otherwise scattered across `lore draft`, `lore
// findings`, and `lore persona`, plus a glance at today's model spend. It
// is read-only -- the operator acts on items through those existing
// commands. Surfacing them in one place is the B-line answer to the
// audit's top pain point ("scattered entry points / no daily operator
// queue").
type OperatorQueue struct {
	Day                   time.Time
	PendingDrafts         []model.Draft
	OpenFindings          []model.Finding
	OpenPersonaCandidates []persona.PersonaCandidateRecord
	TodayUsage            model.UsageSummary
}

// ActionItemCount is the number of items awaiting an operator decision
// (drafts to review + findings to triage + candidates to review). Today's
// usage is informational and excluded.
func (q OperatorQueue) ActionItemCount() int {
	return len(q.PendingDrafts) + len(q.OpenFindings) + len(q.OpenPersonaCandidates)
}

// OperatorQueue gathers the pending operator items for the given day. Each
// source is queried through the same app methods the dedicated commands
// use (ListDrafts / ListFindings / ListPersonaCandidates / SummarizeUsage),
// so the queue can never disagree with them.
func (r *Runtime) OperatorQueue(now time.Time) (OperatorQueue, error) {
	drafts, err := r.ListDrafts()
	if err != nil {
		return OperatorQueue{}, err
	}
	pending := make([]model.Draft, 0)
	for _, d := range drafts {
		if d.State == model.DraftPendingReview {
			pending = append(pending, d)
		}
	}

	findings, err := r.ListFindings(0)
	if err != nil {
		return OperatorQueue{}, err
	}
	open := make([]model.Finding, 0)
	for _, f := range findings {
		if f.State == model.FindingOpen {
			open = append(open, f)
		}
	}

	candidates, err := r.ListPersonaCandidates(persona.PersonaCandidateOpen, 0)
	if err != nil {
		return OperatorQueue{}, err
	}

	// Usage is a best-effort glance, not an action item: a usage-store
	// hiccup must not hide the operator's pending drafts / findings /
	// candidates (the point of the queue). Zero on error.
	usage, _ := r.SummarizeUsage(now)

	return OperatorQueue{
		Day:                   now,
		PendingDrafts:         pending,
		OpenFindings:          open,
		OpenPersonaCandidates: candidates,
		TodayUsage:            usage,
	}, nil
}
