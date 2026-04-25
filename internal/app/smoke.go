package app

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"obsidian-harness/internal/model"
)

type SmokeCheck struct {
	Name   string
	OK     bool
	Detail string
}

type P0SmokeResult struct {
	Managed    model.ManagedStatusView
	Draft      model.Draft
	Checkpoint model.CheckpointDoc
	Report     model.DailyReport
	Checks     []SmokeCheck
}

func (r *Runtime) SmokeP0(now time.Time) (P0SmokeResult, error) {
	if _, err := r.Bootstrap(now); err != nil {
		return P0SmokeResult{}, fmt.Errorf("bootstrap: %w", err)
	}

	managed, err := r.ManagedStatus()
	if err != nil {
		return P0SmokeResult{}, fmt.Errorf("managed status: %w", err)
	}

	draft, err := r.DemoP0A(now)
	if err != nil {
		return P0SmokeResult{}, fmt.Errorf("p0-a: %w", err)
	}

	targetDoc, err := r.Harness.VaultRead(draft.Target.Path)
	targetReady := err == nil && strings.TrimSpace(targetDoc.Content) != ""

	p0b, err := r.DemoP0B(now.Add(30 * time.Minute))
	if err != nil {
		return P0SmokeResult{}, fmt.Errorf("p0-b: %w", err)
	}

	processDay, err := r.ProcessSinkDay(p0b.Report.AgentID, p0b.Report.ReportDay)
	if err != nil {
		return P0SmokeResult{}, fmt.Errorf("process-sink day: %w", err)
	}

	resolved, resolveErr := r.Harness.VaultResolve("demo week", "", 5)
	statePath := filepath.Join(r.Config.Paths.StateDir, "store.db")
	_, stateErr := os.Stat(statePath)

	auditRecords, err := r.Store.Audit().ListAudit(32)
	if err != nil {
		return P0SmokeResult{}, fmt.Errorf("audit list: %w", err)
	}

	result := P0SmokeResult{
		Managed:    managed,
		Draft:      draft,
		Checkpoint: p0b.Checkpoint,
		Report:     p0b.Report,
	}

	coreDocsReady := managed.Ready && allCoreDocsExist(managed.CoreDocs)
	result.Checks = append(result.Checks,
		SmokeCheck{
			Name:   "managed_core_ready",
			OK:     coreDocsReady,
			Detail: fmt.Sprintf("%d core docs ready", len(managed.CoreDocs)),
		},
		SmokeCheck{
			Name:   "p0a_draft_applied",
			OK:     draft.State == model.DraftApplied,
			Detail: fmt.Sprintf("draft %s state=%s", draft.ID, draft.State),
		},
		SmokeCheck{
			Name:   "p0a_target_written",
			OK:     targetReady,
			Detail: draft.Target.Path,
		},
		SmokeCheck{
			Name:   "p0b_checkpoint_materialized",
			OK:     p0b.Checkpoint.State == model.CheckpointMaterialized && strings.TrimSpace(p0b.Checkpoint.Content) != "",
			Detail: p0b.Checkpoint.Path,
		},
		SmokeCheck{
			Name:   "p0b_daily_report_written",
			OK:     processDay.Report != nil && reportContainsWindowKey(processDay.Report.WindowKeys, p0b.Checkpoint.WindowKey),
			Detail: p0b.Report.Path,
		},
		SmokeCheck{
			Name:   "vault_resolve_unique",
			OK:     resolveErr == nil && resolved.Status == "unique" && strings.Contains(filepath.ToSlash(resolved.SelectedPath), "/04-") && strings.HasSuffix(filepath.ToSlash(resolved.SelectedPath), "/demo-week.md"),
			Detail: fmt.Sprintf("status=%s selected=%s", resolved.Status, resolved.SelectedPath),
		},
		SmokeCheck{
			Name:   "sqlite_state_present",
			OK:     stateErr == nil,
			Detail: statePath,
		},
		SmokeCheck{
			Name:   "audit_chain_present",
			OK:     auditHasKinds(auditRecords, model.AuditDraftCreated, model.AuditDraftApplied, model.AuditCheckpointWrite, model.AuditDailyRollup),
			Detail: fmt.Sprintf("%d audit records", len(auditRecords)),
		},
	)

	return result, nil
}

func (r P0SmokeResult) OK() bool {
	for _, check := range r.Checks {
		if !check.OK {
			return false
		}
	}
	return true
}

func allCoreDocsExist(coreDocs []model.ManagedCoreStatus) bool {
	if len(coreDocs) == 0 {
		return false
	}
	for _, doc := range coreDocs {
		if !doc.Exists {
			return false
		}
	}
	return true
}

func reportContainsWindowKey(windowKeys []string, want string) bool {
	for _, key := range windowKeys {
		if key == want {
			return true
		}
	}
	return false
}

func auditHasKinds(records []model.AuditRecord, wants ...model.AuditKind) bool {
	if len(wants) == 0 {
		return true
	}
	seen := make(map[model.AuditKind]bool, len(wants))
	for _, record := range records {
		seen[record.Kind] = true
	}
	for _, want := range wants {
		if !seen[want] {
			return false
		}
	}
	return true
}
