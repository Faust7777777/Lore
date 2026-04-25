package model

import "testing"

func TestMergeStatusUsesHighestPriority(t *testing.T) {
	got := MergeStatus(StatusOK, StatusAdapterDisconnected, StatusBlocked)
	if got != StatusBlocked {
		t.Fatalf("expected %q, got %q", StatusBlocked, got)
	}
}

func TestMergeStatusDecisionPriorityOrder(t *testing.T) {
	tests := []struct {
		name     string
		statuses []Status
		want     Status
	}{
		{
			name:     "blocked beats unexpected error",
			statuses: []Status{StatusError, StatusBlocked},
			want:     StatusBlocked,
		},
		{
			name:     "adapter disconnected beats conflict and error",
			statuses: []Status{StatusError, StatusConflict, StatusAdapterDisconnected},
			want:     StatusAdapterDisconnected,
		},
		{
			name:     "conflict beats unauthorized and unsupported",
			statuses: []Status{StatusUnsupported, StatusUnauthorized, StatusConflict},
			want:     StatusConflict,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := MergeStatus(tt.statuses...); got != tt.want {
				t.Fatalf("MergeStatus(%v) = %q, want %q", tt.statuses, got, tt.want)
			}
		})
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
