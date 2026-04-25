package model

import "testing"

func TestNormalizeAuditRecordFillsStructuredFields(t *testing.T) {
	record := NormalizeAuditRecord(AuditRecord{
		ID:    "audit-1",
		Kind:  AuditMCPRead,
		Actor: "mcp:lore-agent",
	})

	if record.Actor != "mcp:lore-agent" {
		t.Fatalf("Actor = %q, want legacy actor preserved", record.Actor)
	}
	if record.ActorType != "mcp" {
		t.Fatalf("ActorType = %q, want mcp", record.ActorType)
	}
	if record.ActorID != "lore-agent" {
		t.Fatalf("ActorID = %q, want lore-agent", record.ActorID)
	}
	if record.ResultStatus != AuditResultOK {
		t.Fatalf("ResultStatus = %q, want ok", record.ResultStatus)
	}
	if record.CorrelationID != "audit-1" {
		t.Fatalf("CorrelationID = %q, want audit-1", record.CorrelationID)
	}
}

func TestNormalizeAuditRecordPreservesExplicitStructuredFields(t *testing.T) {
	record := NormalizeAuditRecord(AuditRecord{
		ID:            "audit-1",
		Actor:         "legacy",
		ActorType:     "operator",
		ActorID:       "reviewer-1",
		ResultStatus:  AuditResultError,
		CorrelationID: "corr-1",
	})

	if record.ActorType != "operator" {
		t.Fatalf("ActorType = %q, want operator", record.ActorType)
	}
	if record.ActorID != "reviewer-1" {
		t.Fatalf("ActorID = %q, want reviewer-1", record.ActorID)
	}
	if record.ResultStatus != AuditResultError {
		t.Fatalf("ResultStatus = %q, want error", record.ResultStatus)
	}
	if record.CorrelationID != "corr-1" {
		t.Fatalf("CorrelationID = %q, want corr-1", record.CorrelationID)
	}
}
