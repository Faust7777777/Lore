package model

import "sort"

type Status string

const (
	StatusOK                  Status = "ok"
	StatusBlocked             Status = "blocked"
	StatusAdapterDisconnected Status = "adapter_disconnected"
	StatusConflict            Status = "conflict"
	StatusUnauthorized        Status = "unauthorized"
	StatusUnsupported         Status = "unsupported"
	StatusError               Status = "error"
)

type ReasonCode string

const (
	ReasonNone               ReasonCode = "none"
	ReasonModelUnavailable   ReasonCode = "model_unavailable"
	ReasonAdapterDown        ReasonCode = "adapter_down"
	ReasonBaseVersionChanged ReasonCode = "base_version_changed"
	ReasonPolicyDenied       ReasonCode = "policy_denied"
	ReasonUnsupportedAction  ReasonCode = "unsupported_action"
	ReasonUnexpectedFailure  ReasonCode = "unexpected_failure"
)

type Outcome struct {
	Status      Status       `json:"status"`
	ReasonCodes []ReasonCode `json:"reason_codes,omitempty"`
}

var statusPriority = map[Status]int{
	StatusOK:                  0,
	StatusError:               1,
	StatusUnsupported:         2,
	StatusUnauthorized:        3,
	StatusConflict:            4,
	StatusAdapterDisconnected: 5,
	StatusBlocked:             6,
}

func NewOutcome(status Status, reasons ...ReasonCode) Outcome {
	return Outcome{
		Status:      normalizeStatus(status),
		ReasonCodes: uniqueReasonCodes(reasons),
	}
}

func MergeStatus(statuses ...Status) Status {
	if len(statuses) == 0 {
		return StatusOK
	}

	merged := StatusOK
	for _, status := range statuses {
		current := normalizeStatus(status)
		if statusPriority[current] > statusPriority[merged] {
			merged = current
		}
	}

	return merged
}

func MergeOutcomes(outcomes ...Outcome) Outcome {
	if len(outcomes) == 0 {
		return NewOutcome(StatusOK)
	}

	statuses := make([]Status, 0, len(outcomes))
	reasons := make([]ReasonCode, 0, len(outcomes))
	for _, outcome := range outcomes {
		statuses = append(statuses, outcome.Status)
		reasons = append(reasons, outcome.ReasonCodes...)
	}

	return NewOutcome(MergeStatus(statuses...), reasons...)
}

func normalizeStatus(status Status) Status {
	if _, ok := statusPriority[status]; !ok {
		return StatusError
	}
	return status
}

func uniqueReasonCodes(codes []ReasonCode) []ReasonCode {
	if len(codes) == 0 {
		return nil
	}

	seen := make(map[ReasonCode]struct{}, len(codes))
	unique := make([]ReasonCode, 0, len(codes))
	for _, code := range codes {
		if code == "" {
			continue
		}
		if _, ok := seen[code]; ok {
			continue
		}
		seen[code] = struct{}{}
		unique = append(unique, code)
	}

	sort.Slice(unique, func(i, j int) bool { return unique[i] < unique[j] })
	return unique
}
