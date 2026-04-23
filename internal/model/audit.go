package model

import "time"

type AuditKind string

const (
	AuditDraftCreated      AuditKind = "draft_created"
	AuditDraftStateChange  AuditKind = "draft_state_change"
	AuditDraftApplied      AuditKind = "draft_applied"
	AuditCheckpointWrite   AuditKind = "checkpoint_write"
	AuditDailyRollup       AuditKind = "daily_rollup"
	AuditMCPRead           AuditKind = "mcp_read"
	AuditLowRiskVaultWrite AuditKind = "low_risk_vault_write"
	AuditRuntimeHealth     AuditKind = "runtime_health"
)

type AuditRecord struct {
	ID         string            `json:"id"`
	Kind       AuditKind         `json:"kind"`
	Actor      string            `json:"actor"`
	Target     string            `json:"target"`
	OccurredAt time.Time         `json:"occurred_at"`
	Metadata   map[string]string `json:"metadata,omitempty"`
}
