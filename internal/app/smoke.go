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

type GovernedNoteSmokeResult struct {
	Proposal model.MarkdownNoteProposalResult
	Review   DraftReview
	Applied  model.Draft
	Target   model.VaultDocument
	Checks   []SmokeCheck
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

func (r *Runtime) SmokeGovernedMarkdownNoteIntake(now time.Time) (GovernedNoteSmokeResult, error) {
	if _, err := r.Bootstrap(now); err != nil {
		return GovernedNoteSmokeResult{}, fmt.Errorf("bootstrap: %w", err)
	}

	targetPath := "03-notes/smoke/governed-note-intake.md"
	proposal := model.MarkdownNoteProposal{
		TargetPath:  targetPath,
		Title:       "Governed Note Intake Smoke",
		Content:     "# Governed Note Intake Smoke\n\n- External agent content enters Lore as a proposal.\n- Local Lore applies only after review and approval.",
		SourceKind:  "development",
		Evidence:    "deterministic smoke test content",
		Reason:      "verify governed markdown note intake flow",
		Source:      "runtime_smoke",
		ObservedAt:  now,
		TaskContext: "business-level governed intake smoke",
		Topic:       "Lore governance",
		DedupeKey:   "governed-note-intake-smoke",
	}
	proposalResult, err := r.Harness.ProposeMarkdownNote(proposal, now)
	if err != nil {
		return GovernedNoteSmokeResult{}, fmt.Errorf("markdown note proposal: %w", err)
	}

	targetAbs := filepath.Join(r.Config.Paths.VaultRoot, filepath.FromSlash(targetPath))
	_, statAfterProposalErr := os.Stat(targetAbs)
	review, err := r.ReviewDraft(proposalResult.DraftID)
	if err != nil {
		return GovernedNoteSmokeResult{}, fmt.Errorf("review draft: %w", err)
	}
	if _, err := r.Harness.ApproveDraft(proposalResult.DraftID, now.Add(time.Minute)); err != nil {
		return GovernedNoteSmokeResult{}, fmt.Errorf("approve draft: %w", err)
	}
	applied, err := r.Harness.ApplyDraft(proposalResult.DraftID, now.Add(2*time.Minute))
	if err != nil {
		return GovernedNoteSmokeResult{}, fmt.Errorf("apply draft: %w", err)
	}
	targetDoc, err := r.Harness.VaultRead(targetPath)
	if err != nil {
		return GovernedNoteSmokeResult{}, fmt.Errorf("read applied note: %w", err)
	}
	auditRecords, err := r.Store.Audit().ListAudit(64)
	if err != nil {
		return GovernedNoteSmokeResult{}, fmt.Errorf("audit list: %w", err)
	}

	result := GovernedNoteSmokeResult{
		Proposal: proposalResult,
		Review:   review,
		Applied:  applied,
		Target:   targetDoc,
	}
	result.Checks = append(result.Checks,
		SmokeCheck{
			Name:   "proposal_created_pending_draft",
			OK:     proposalResult.Status == "draft_created" && proposalResult.DraftID != "" && proposalResult.ReviewRequired && review.Draft.State == model.DraftPendingReview,
			Detail: fmt.Sprintf("draft=%s state=%s", proposalResult.DraftID, review.Draft.State),
		},
		SmokeCheck{
			Name:   "proposal_does_not_write_target",
			OK:     os.IsNotExist(statAfterProposalErr),
			Detail: targetPath,
		},
		SmokeCheck{
			Name:   "review_loaded_candidate",
			OK:     review.Draft.Kind == model.DraftKindMarkdownNoteWrite && review.Draft.Target.Path == targetPath && strings.Contains(review.Draft.ProposedContent, "Governed Note Intake Smoke"),
			Detail: review.Draft.Target.Path,
		},
		SmokeCheck{
			Name:   "approved_apply_writes_note",
			OK:     applied.State == model.DraftApplied && targetDoc.Path == targetPath && strings.Contains(targetDoc.Content, "Local Lore applies only after review and approval."),
			Detail: fmt.Sprintf("draft=%s target=%s", applied.ID, targetDoc.Path),
		},
		SmokeCheck{
			Name:   "audit_chain_present",
			OK:     auditHasDraftChain(auditRecords, proposalResult.DraftID, targetPath),
			Detail: fmt.Sprintf("draft=%s target=%s audit_records=%d", proposalResult.DraftID, targetPath, len(auditRecords)),
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

func (r GovernedNoteSmokeResult) OK() bool {
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

func auditHasDraftChain(records []model.AuditRecord, draftID string, target string) bool {
	if strings.TrimSpace(draftID) == "" || strings.TrimSpace(target) == "" {
		return false
	}
	var created, approved, applied bool
	for _, record := range records {
		if record.CorrelationID != draftID || record.Target != target {
			continue
		}
		switch record.Kind {
		case model.AuditDraftCreated:
			created = record.Metadata["draft_id"] == draftID
		case model.AuditDraftStateChange:
			if record.Metadata["state"] == string(model.DraftApproved) {
				approved = true
			}
		case model.AuditDraftApplied:
			applied = record.Metadata["draft_id"] == draftID && record.Metadata["state"] == string(model.DraftApplied)
		}
	}
	return created && approved && applied
}
