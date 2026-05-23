package model

import (
	"fmt"
	"time"
)

type FindingKind string

const (
	FindingOutOfBandVaultWrite    FindingKind = "out_of_band_vault_write"
	FindingGovernanceReviewNeeded FindingKind = "governance_review_needed"
)

type FindingState string

const (
	FindingOpen     FindingState = "open"
	FindingResolved FindingState = "resolved"
	FindingIgnored  FindingState = "ignored"
)

type FindingSeverity string

const (
	FindingSeverityInfo     FindingSeverity = "info"
	FindingSeverityWarning  FindingSeverity = "warning"
	FindingSeverityCritical FindingSeverity = "critical"
)

// ValidateFindingTransition reports whether a Finding may move
// from `from` to `to`. The Finding state machine is intentionally
// flat: Open is the only source state, and Resolved / Ignored
// are terminal. Without this guard the three store backends would
// silently overwrite a terminal state back to Open (or swap a
// resolved Finding into ignored) because UpdateFindingState used
// a plain Get-then-Save pattern. The architect's state-machine
// audit (docs/handoff-full-project-review-2026-05-23.md section 4
// and hermes-workspace/lore-state-machine-audit.md F-1) flagged
// this as the highest-severity state-machine gap in the codebase.
//
// Backends MUST call this before persisting a state change. The
// SQLite backend additionally uses an `UPDATE ... WHERE state=?`
// CAS so a concurrent peer cannot race past the validation.
//
// Returns nil for the two legal transitions (open -> resolved,
// open -> ignored); a typed error for every other combination.
func ValidateFindingTransition(from, to FindingState) error {
	if from == "" {
		return fmt.Errorf("findings: source state is empty; expected %q", FindingOpen)
	}
	if from != FindingOpen {
		return fmt.Errorf("findings: %w: cannot leave terminal state %q", ErrFindingTerminalState, from)
	}
	switch to {
	case FindingResolved, FindingIgnored:
		return nil
	default:
		return fmt.Errorf("findings: %w: cannot transition %q -> %q", ErrFindingIllegalTransition, from, to)
	}
}

// ErrFindingTerminalState is wrapped by ValidateFindingTransition
// when the source state is already resolved / ignored. Callers
// that need to distinguish "terminal" from "unknown target" can
// errors.Is against this sentinel.
var ErrFindingTerminalState = fmt.Errorf("finding is in a terminal state")

// ErrFindingIllegalTransition is wrapped by ValidateFindingTransition
// when the source state is open but the target is neither
// resolved nor ignored.
var ErrFindingIllegalTransition = fmt.Errorf("finding transition is not legal")

type Finding struct {
	ID         string            `json:"id"`
	Kind       FindingKind       `json:"kind"`
	State      FindingState      `json:"state"`
	Severity   FindingSeverity   `json:"severity"`
	Target     DocumentRef       `json:"target"`
	Title      string            `json:"title"`
	Summary    string            `json:"summary"`
	Detail     string            `json:"detail,omitempty"`
	Source     string            `json:"source"`
	AuditID    string            `json:"audit_id,omitempty"`
	DetectedAt time.Time         `json:"detected_at"`
	UpdatedAt  time.Time         `json:"updated_at"`
	Metadata   map[string]string `json:"metadata,omitempty"`
}
