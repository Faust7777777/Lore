package model

import "time"

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
