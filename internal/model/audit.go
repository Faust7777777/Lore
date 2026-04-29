package model

import (
	"strings"
	"time"
)

type AuditKind string

const (
	AuditDraftCreated        AuditKind = "draft_created"
	AuditDraftStateChange    AuditKind = "draft_state_change"
	AuditDraftApplied        AuditKind = "draft_applied"
	AuditCheckpointWrite     AuditKind = "checkpoint_write"
	AuditDailyRollup         AuditKind = "daily_rollup"
	AuditMCPRead             AuditKind = "mcp_read"
	AuditLowRiskVaultWrite   AuditKind = "low_risk_vault_write"
	AuditOutOfBandVaultWrite AuditKind = "out_of_band_vault_write"
	AuditGovernanceFinding   AuditKind = "governance_finding"
	AuditFindingStateChange  AuditKind = "finding_state_change"
	AuditRuntimeHealth       AuditKind = "runtime_health"
)

type AuditResultStatus string

const (
	AuditResultOK    AuditResultStatus = "ok"
	AuditResultError AuditResultStatus = "error"
)

type AuditRecord struct {
	ID            string            `json:"id"`
	Kind          AuditKind         `json:"kind"`
	Actor         string            `json:"actor"`
	ActorType     string            `json:"actor_type,omitempty"`
	ActorID       string            `json:"actor_id,omitempty"`
	ResultStatus  AuditResultStatus `json:"result_status,omitempty"`
	CorrelationID string            `json:"correlation_id,omitempty"`
	Target        string            `json:"target"`
	OccurredAt    time.Time         `json:"occurred_at"`
	Metadata      map[string]string `json:"metadata,omitempty"`
}

func NormalizeAuditRecord(record AuditRecord) AuditRecord {
	if strings.TrimSpace(record.Actor) == "" {
		record.Actor = "system"
	}
	if strings.TrimSpace(record.ActorType) == "" || strings.TrimSpace(record.ActorID) == "" {
		record.ActorType, record.ActorID = splitAuditActor(record.Actor)
	}
	if record.ResultStatus == "" {
		record.ResultStatus = AuditResultOK
	}
	if strings.TrimSpace(record.CorrelationID) == "" {
		record.CorrelationID = record.ID
	}
	return record
}

func splitAuditActor(actor string) (string, string) {
	actor = strings.TrimSpace(actor)
	if actor == "" {
		return "system", "system"
	}
	actorType, actorID, found := strings.Cut(actor, ":")
	if found {
		actorType = strings.TrimSpace(actorType)
		actorID = strings.TrimSpace(actorID)
		if actorType != "" && actorID != "" {
			return actorType, actorID
		}
	}
	return actor, actor
}
