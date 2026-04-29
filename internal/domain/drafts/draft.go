package drafts

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"obsidian-harness/internal/model"
)

type Kind = model.DraftKind
type State = model.DraftState
type Target = model.DocumentRef

const (
	KindProgressUpdate = model.DraftKindProgressSync
	KindPersonaUpdate  = model.DraftKindPersonaUpdate
	KindWeaknessCard   = model.DraftKindWeaknessUpdate
	KindPlanAdjustment = model.DraftKindPlanAdjustment
)

const (
	StateDraft      = model.DraftCreated
	StateReview     = model.DraftPendingReview
	StateApproved   = model.DraftApproved
	StateApplied    = model.DraftApplied
	StateRevision   = model.DraftRevisionRequested
	StateRejected   = model.DraftRejected
	StateExpired    = model.DraftExpired
	StateSuperseded = model.DraftSuperseded
	StateConflicted = model.DraftConflicted
)

type Source string

const (
	SourcePrimaryAgent  Source = "primary_agent"
	SourceExternalAgent Source = "external_agent"
	SourceOperator      Source = "operator"
)

type Review struct {
	Reviewer   string
	Comment    string
	ReviewedAt time.Time
}

type Params struct {
	ID              string
	Kind            Kind
	Source          Source
	Target          Target
	Title           string
	Summary         string
	ProposedContent string
	EvidenceRefs    []string
	CreatedAt       time.Time
}

type Draft struct {
	model.Draft
	Source    Source
	Review    *Review
	AppliedBy string
}

var (
	ErrDraftIDRequired          = errors.New("drafts: draft id is required")
	ErrDraftKindRequired        = errors.New("drafts: draft kind is required")
	ErrDraftSourceRequired      = errors.New("drafts: draft source is required")
	ErrDraftSummaryRequired     = errors.New("drafts: draft summary is required")
	ErrTargetPathRequired       = errors.New("drafts: target path is required")
	ErrTargetBaseVersionMissing = errors.New("drafts: target base version is required")
)

func New(params Params) (*Draft, error) {
	if strings.TrimSpace(params.ID) == "" {
		return nil, ErrDraftIDRequired
	}
	if strings.TrimSpace(string(params.Kind)) == "" {
		return nil, ErrDraftKindRequired
	}
	if strings.TrimSpace(string(params.Source)) == "" {
		return nil, ErrDraftSourceRequired
	}
	if strings.TrimSpace(params.Summary) == "" {
		return nil, ErrDraftSummaryRequired
	}
	if strings.TrimSpace(params.Target.Path) == "" {
		return nil, ErrTargetPathRequired
	}
	if strings.TrimSpace(params.Target.BaseVersion) == "" {
		return nil, ErrTargetBaseVersionMissing
	}

	createdAt := params.CreatedAt
	return &Draft{
		Draft: model.Draft{
			ID:              strings.TrimSpace(params.ID),
			Kind:            params.Kind,
			State:           StateDraft,
			Target:          normalizeTarget(params.Target),
			Title:           withFallback(params.Title, params.Summary),
			Summary:         strings.TrimSpace(params.Summary),
			ProposedContent: strings.TrimSpace(params.ProposedContent),
			EvidenceRefs:    append([]string(nil), params.EvidenceRefs...),
			CreatedAt:       createdAt,
			UpdatedAt:       createdAt,
		},
		Source: params.Source,
	}, nil
}

func (d *Draft) SubmitForReview(at time.Time) error {
	if d == nil {
		return errors.New("drafts: nil draft")
	}
	if d.State != StateDraft {
		return invalidTransition(d.State, "submit_for_review")
	}

	d.State = StateReview
	d.UpdatedAt = at
	return nil
}

func (d *Draft) Approve(reviewer string, comment string, at time.Time) error {
	if d == nil {
		return errors.New("drafts: nil draft")
	}
	if d.State != StateReview {
		return invalidTransition(d.State, "approve")
	}
	if strings.TrimSpace(reviewer) == "" {
		return errors.New("drafts: reviewer is required")
	}

	d.State = StateApproved
	d.UpdatedAt = at
	d.Review = &Review{
		Reviewer:   strings.TrimSpace(reviewer),
		Comment:    strings.TrimSpace(comment),
		ReviewedAt: at,
	}
	return nil
}

func (d *Draft) Apply(applier string, at time.Time) error {
	if d == nil {
		return errors.New("drafts: nil draft")
	}
	if !d.ReadyToApply() {
		return invalidTransition(d.State, "apply")
	}
	if strings.TrimSpace(applier) == "" {
		return errors.New("drafts: applier is required")
	}

	d.State = StateApplied
	d.UpdatedAt = at
	d.AppliedBy = strings.TrimSpace(applier)
	return nil
}

func (d Draft) ReadyToApply() bool {
	return d.State == StateApproved
}

func CanTransition(from State, to State) bool {
	switch from {
	case StateDraft:
		return to == StateReview || to == StateExpired || to == StateSuperseded
	case StateReview:
		return to == StateApproved || to == StateRejected || to == StateRevision || to == StateExpired || to == StateSuperseded
	case StateApproved:
		return to == StateApplied || to == StateConflicted || to == StateExpired || to == StateSuperseded
	case StateRevision:
		return to == StateSuperseded || to == StateExpired
	default:
		return false
	}
}

func ValidateTransition(from State, to State) error {
	if CanTransition(from, to) {
		return nil
	}
	return invalidTransition(from, "transition_to_"+string(to))
}

func (d Draft) AllowsDirectWrite() bool {
	return false
}

func normalizeTarget(target Target) Target {
	return Target{
		Path:        strings.TrimSpace(target.Path),
		Class:       target.Class,
		BaseVersion: strings.TrimSpace(target.BaseVersion),
	}
}

func withFallback(value string, fallback string) string {
	value = strings.TrimSpace(value)
	if value != "" {
		return value
	}
	return strings.TrimSpace(fallback)
}

func invalidTransition(state State, action string) error {
	return fmt.Errorf("drafts: cannot %s when draft is in %s state", action, state)
}
