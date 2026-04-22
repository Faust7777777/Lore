package model

import "time"

type DraftState string

const (
	DraftCreated           DraftState = "created"
	DraftPendingReview     DraftState = "pending_review"
	DraftApproved          DraftState = "approved"
	DraftApplied           DraftState = "applied"
	DraftRevisionRequested DraftState = "revision_requested"
	DraftRejected          DraftState = "rejected"
	DraftExpired           DraftState = "expired"
	DraftSuperseded        DraftState = "superseded"
	DraftConflicted        DraftState = "conflicted"
)

type DraftKind string

const (
	DraftKindProgressSync   DraftKind = "progress_sync"
	DraftKindPersonaUpdate  DraftKind = "persona_update"
	DraftKindWeaknessUpdate DraftKind = "weakness_update"
	DraftKindPlanAdjustment DraftKind = "plan_adjustment"
)

type Draft struct {
	ID              string      `json:"id"`
	Kind            DraftKind   `json:"kind"`
	State           DraftState  `json:"state"`
	Target          DocumentRef `json:"target"`
	Title           string      `json:"title"`
	Summary         string      `json:"summary"`
	ProposedContent string      `json:"proposed_content"`
	EvidenceRefs    []string    `json:"evidence_refs,omitempty"`
	Supersedes      string      `json:"supersedes,omitempty"`
	CreatedAt       time.Time   `json:"created_at"`
	UpdatedAt       time.Time   `json:"updated_at"`
}
