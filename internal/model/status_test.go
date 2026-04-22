package model

import "testing"

func TestMergeStatusUsesHighestPriority(t *testing.T) {
	got := MergeStatus(StatusOK, StatusAdapterDisconnected, StatusBlocked)
	if got != StatusBlocked {
		t.Fatalf("expected %q, got %q", StatusBlocked, got)
	}
}

func TestMergeOutcomesDeduplicatesReasons(t *testing.T) {
	got := MergeOutcomes(
		NewOutcome(StatusBlocked, ReasonModelUnavailable),
		NewOutcome(StatusAdapterDisconnected, ReasonAdapterDown, ReasonModelUnavailable),
	)
	if got.Status != StatusBlocked {
		t.Fatalf("expected status %q, got %q", StatusBlocked, got.Status)
	}
	if len(got.ReasonCodes) != 2 {
		t.Fatalf("expected 2 unique reason codes, got %d", len(got.ReasonCodes))
	}
}
