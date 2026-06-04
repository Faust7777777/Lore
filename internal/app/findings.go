package app

import (
	"fmt"
	"strings"
	"time"

	"obsidian-harness/internal/model"
)

func (r *Runtime) ListFindings(limit int) ([]model.Finding, error) {
	return r.Store.Findings().ListFindings(limit)
}

func (r *Runtime) ResolveFinding(id string) (model.Finding, error) {
	return r.updateFindingState(id, model.FindingResolved, "resolve", time.Now())
}

func (r *Runtime) IgnoreFinding(id string) (model.Finding, error) {
	return r.updateFindingState(id, model.FindingIgnored, "ignore", time.Now())
}

func (r *Runtime) updateFindingState(id string, state model.FindingState, action string, at time.Time) (model.Finding, error) {
	finding, err := r.Store.Findings().UpdateFindingState(strings.TrimSpace(id), state, at)
	if err != nil {
		return model.Finding{}, err
	}
	if err := r.Store.Audit().AppendAudit(model.AuditRecord{
		ID:            findingStateAuditID(action, finding.ID, at),
		Kind:          model.AuditFindingStateChange,
		Actor:         "local_lore",
		CorrelationID: finding.ID,
		Target:        finding.Target.Path,
		OccurredAt:    at,
		Metadata: map[string]string{
			"finding_id": finding.ID,
			"state":      string(finding.State),
			"action":     action,
		},
	}); err != nil {
		// The state change already committed; do not report it as a total
		// failure (an empty finding). Return the updated finding plus a
		// descriptive error so the caller knows the primary effect landed
		// even though the audit trail is incomplete. Mirrors the harness's
		// best-effort recordAudit rather than inverting the result.
		return finding, fmt.Errorf("finding %s moved to %s but the audit record failed: %w", finding.ID, finding.State, err)
	}
	return finding, nil
}

func findingStateAuditID(action string, findingID string, at time.Time) string {
	action = strings.NewReplacer(" ", "-", "/", "-", "\\", "-").Replace(strings.TrimSpace(action))
	findingID = strings.NewReplacer(" ", "-", "/", "-", "\\", "-").Replace(strings.TrimSpace(findingID))
	findingID = truncateFindingIDRunes(findingID, 80)
	return fmt.Sprintf("finding-state-%s-%s-%d", action, findingID, at.UnixNano())
}

func truncateFindingIDRunes(value string, limit int) string {
	if limit <= 0 {
		return ""
	}
	runes := []rune(value)
	if len(runes) <= limit {
		return value
	}
	return string(runes[:limit])
}
