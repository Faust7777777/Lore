package orchestrator

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"obsidian-harness/internal/bootstrap"
	"obsidian-harness/internal/config"
	"obsidian-harness/internal/domain/docclass"
	"obsidian-harness/internal/domain/drafts"
	"obsidian-harness/internal/domain/processsink"
	"obsidian-harness/internal/model"
	hruntime "obsidian-harness/internal/runtime"
	"obsidian-harness/internal/store"
	"obsidian-harness/internal/vault"
)

var (
	ErrUnsupportedDocument = errors.New("orchestrator: unsupported document class")
	ErrDraftNotReady       = errors.New("orchestrator: draft is not ready for requested operation")
	ErrUnsupportedDraft    = errors.New("orchestrator: unsupported draft kind for apply")
	ErrInvalidDraftPatch   = errors.New("orchestrator: invalid draft patch payload")
	ErrDirectWriteDenied   = errors.New("orchestrator: direct vault write denied by governance policy")
)

type Harness struct {
	cfg        config.Config
	store      store.StateStore
	broker     hruntime.Broker
	auditor    *hruntime.Auditor
	health     *hruntime.HealthService
	classifier docclass.Classifier
	sink       *processsink.Service
}

func New(cfg config.Config, state store.StateStore) (*Harness, error) {
	classifier, err := docclass.NewClassifier(docclass.RecommendedRules())
	if err != nil {
		return nil, err
	}

	broker := hruntime.NewInMemoryBroker()
	return &Harness{
		cfg:        cfg,
		store:      state,
		broker:     broker,
		auditor:    hruntime.NewAuditor(state.Audit(), broker),
		health:     hruntime.NewHealthService(),
		classifier: classifier,
		sink: processsink.NewService(
			state.ProcessSink(),
			fileWriter{tempSuffix: cfg.Vault.TempSuffix},
			cfg.Paths.ProcessSinkDir,
		),
	}, nil
}

func (h *Harness) BootstrapManagedVault(now time.Time) ([]model.DocumentRef, error) {
	templates := bootstrap.DefaultManagedTemplates(now)
	created := make([]model.DocumentRef, 0, len(templates))
	for _, template := range templates {
		absolutePath := filepath.Join(h.cfg.Paths.VaultRoot, template.Ref.Path)
		if _, err := os.Stat(absolutePath); err == nil {
			continue
		} else if !os.IsNotExist(err) {
			return nil, err
		}

		if _, err := vault.WriteFileAtomic(absolutePath, []byte(template.Content), h.cfg.Vault.TempSuffix); err != nil {
			return nil, err
		}
		created = append(created, template.Ref)
	}
	return created, nil
}

func (h *Harness) UpdateDependencies(modelAvailable bool, adapterConnected bool) model.HealthSnapshot {
	snapshot := h.health.Update(hruntime.DependencyProbe{
		ModelAvailable:   modelAvailable,
		AdapterConnected: adapterConnected,
	})
	h.recordAudit(model.AuditRecord{
		ID:         auditID("health", snapshot.CheckedAt),
		Kind:       model.AuditRuntimeHealth,
		Actor:      "runtime",
		Target:     "dependencies",
		OccurredAt: snapshot.CheckedAt,
		Metadata: map[string]string{
			"status":  string(snapshot.Outcome.Status),
			"message": snapshot.Message,
		},
	})
	return snapshot
}

func (h *Harness) StatusSnapshot() model.HealthSnapshot {
	return h.health.Snapshot()
}

func (h *Harness) ObserveDocumentChange(relPath string, content []byte, at time.Time) (model.Draft, error) {
	normalizedPath := cleanRelPath(relPath)
	classification := h.classifier.Classify(normalizedPath)
	if !classification.IsPlan() {
		return model.Draft{}, ErrUnsupportedDocument
	}

	targetPath := h.cfg.Vault.ManagedCore.ProgressIndex
	targetAbs := filepath.Join(h.cfg.Paths.VaultRoot, targetPath)
	_, baseVersion, err := vault.ReadFileWithHash(targetAbs)
	if err != nil {
		return model.Draft{}, err
	}

	draft := model.Draft{
		ID:    fmt.Sprintf("draft-%d", at.UnixNano()),
		Kind:  model.DraftKindProgressSync,
		State: model.DraftPendingReview,
		Target: model.DocumentRef{
			Path:        targetPath,
			Class:       model.DocClassProgressIndex,
			BaseVersion: baseVersion,
		},
		Title:           fmt.Sprintf("Progress sync for %s", filepath.Base(normalizedPath)),
		Summary:         fmt.Sprintf("Update progress index row for %s (%s)", normalizedPath, summarizeContent(content)),
		ProposedContent: renderProgressPatch(normalizedPath, classification.Class, at),
		EvidenceRefs:    []string{normalizedPath},
		CreatedAt:       at,
		UpdatedAt:       at,
	}

	if err := h.store.Drafts().SaveDraft(draft); err != nil {
		return model.Draft{}, err
	}
	if err := h.broker.Publish(context.Background(), hruntime.Event{
		ID:         draft.ID,
		Type:       hruntime.EventDraftCreated,
		Source:     "observe_document_change",
		OccurredAt: at,
		Payload:    draft,
	}); err != nil {
		return model.Draft{}, err
	}
	h.recordAudit(model.AuditRecord{
		ID:            auditID("draft-created", at),
		Kind:          model.AuditDraftCreated,
		CorrelationID: draft.ID,
		Actor:         "runtime",
		Target:        draft.Target.Path,
		OccurredAt:    at,
		Metadata: map[string]string{
			"draft_id": draft.ID,
			"source":   normalizedPath,
		},
	})
	return draft, nil
}

func (h *Harness) ProposePersonaUpdate(proposal model.PersonaUpdateProposal, at time.Time) (model.PersonaUpdateProposalResult, error) {
	proposal.Field = strings.TrimSpace(proposal.Field)
	proposal.CurrentValue = strings.TrimSpace(proposal.CurrentValue)
	proposal.ProposedValue = strings.TrimSpace(proposal.ProposedValue)
	proposal.Evidence = strings.TrimSpace(proposal.Evidence)
	proposal.Reason = strings.TrimSpace(proposal.Reason)
	proposal.Confidence = strings.TrimSpace(proposal.Confidence)
	proposal.Source = strings.TrimSpace(proposal.Source)
	if proposal.Field == "" || proposal.ProposedValue == "" || proposal.Evidence == "" || proposal.Reason == "" || proposal.Confidence == "" || proposal.Source == "" || proposal.ObservedAt.IsZero() {
		return model.PersonaUpdateProposalResult{}, fmt.Errorf("orchestrator: persona update proposal requires field, proposed_value, evidence, reason, confidence, source, and observed_at")
	}
	switch proposal.Confidence {
	case "low", "medium", "high":
	default:
		return model.PersonaUpdateProposalResult{}, fmt.Errorf("orchestrator: persona update proposal confidence must be low, medium, or high")
	}

	targetPath := h.cfg.Vault.ManagedCore.Persona
	targetAbs := filepath.Join(h.cfg.Paths.VaultRoot, targetPath)
	_, baseVersion, err := vault.ReadFileWithHash(targetAbs)
	if err != nil {
		return model.PersonaUpdateProposalResult{}, err
	}
	payload, err := json.MarshalIndent(proposal, "", "  ")
	if err != nil {
		return model.PersonaUpdateProposalResult{}, err
	}

	draft := model.Draft{
		ID:    fmt.Sprintf("draft-%d", at.UnixNano()),
		Kind:  model.DraftKindPersonaUpdate,
		State: model.DraftPendingReview,
		Target: model.DocumentRef{
			Path:        targetPath,
			Class:       model.DocClassPersona,
			BaseVersion: baseVersion,
		},
		Title:           "Persona update proposal: " + proposal.Field,
		Summary:         fmt.Sprintf("Propose persona field %s = %s. Proposal creation is not apply; the persona document is unchanged until reviewed and applied by Lore.", proposal.Field, proposal.ProposedValue),
		ProposedContent: string(payload),
		EvidenceRefs:    []string{proposal.Evidence},
		CreatedAt:       at,
		UpdatedAt:       at,
	}

	if err := h.store.Drafts().SaveDraft(draft); err != nil {
		return model.PersonaUpdateProposalResult{}, err
	}
	if err := h.broker.Publish(context.Background(), hruntime.Event{
		ID:         draft.ID,
		Type:       hruntime.EventDraftCreated,
		Source:     "persona_update_propose",
		OccurredAt: at,
		Payload:    draft,
	}); err != nil {
		return model.PersonaUpdateProposalResult{}, err
	}
	h.recordAudit(model.AuditRecord{
		ID:            auditID("persona-update-proposed", at),
		Kind:          model.AuditDraftCreated,
		CorrelationID: draft.ID,
		Actor:         "external_agent",
		Target:        draft.Target.Path,
		OccurredAt:    at,
		Metadata: map[string]string{
			"draft_id": draft.ID,
			"field":    proposal.Field,
			"source":   proposal.Source,
		},
	})
	return model.PersonaUpdateProposalResult{
		Status:         "draft_created",
		DraftID:        draft.ID,
		Target:         draft.Target.Path,
		ReviewRequired: true,
	}, nil
}

func (h *Harness) ListDrafts() ([]model.Draft, error) {
	return h.store.Drafts().ListDrafts()
}

func (h *Harness) GetDraft(id string) (model.Draft, error) {
	return h.store.Drafts().GetDraft(id)
}

func (h *Harness) ApproveDraft(id string, at time.Time) (model.Draft, error) {
	draft, err := h.store.Drafts().GetDraft(id)
	if err != nil {
		return model.Draft{}, err
	}
	if draft.State != model.DraftPendingReview {
		return model.Draft{}, ErrDraftNotReady
	}

	return h.transitionDraftState(id, model.DraftApproved, "approve_draft", "draft-approved", "reviewer", at)
}

func (h *Harness) RejectDraft(id string, at time.Time) (model.Draft, error) {
	draft, err := h.store.Drafts().GetDraft(id)
	if err != nil {
		return model.Draft{}, err
	}
	if draft.State != model.DraftPendingReview {
		return model.Draft{}, ErrDraftNotReady
	}

	return h.transitionDraftState(id, model.DraftRejected, "reject_draft", "draft-rejected", "reviewer", at)
}

func (h *Harness) RequestDraftRevision(id string, at time.Time) (model.Draft, error) {
	draft, err := h.store.Drafts().GetDraft(id)
	if err != nil {
		return model.Draft{}, err
	}
	if draft.State != model.DraftPendingReview {
		return model.Draft{}, ErrDraftNotReady
	}

	return h.transitionDraftState(id, model.DraftRevisionRequested, "request_draft_revision", "draft-revision-requested", "reviewer", at)
}

func (h *Harness) ApplyDraft(id string, at time.Time) (model.Draft, error) {
	draft, err := h.store.Drafts().GetDraft(id)
	if err != nil {
		return model.Draft{}, err
	}
	if draft.State != model.DraftApproved {
		return model.Draft{}, ErrDraftNotReady
	}

	targetAbs := filepath.Join(h.cfg.Paths.VaultRoot, draft.Target.Path)
	current, hash, err := vault.ReadFileWithHash(targetAbs)
	if err != nil {
		return model.Draft{}, err
	}
	if hash != draft.Target.BaseVersion {
		if err := drafts.ValidateTransition(draft.State, model.DraftConflicted); err != nil {
			return model.Draft{}, ErrDraftNotReady
		}
		conflicted, updateErr := h.store.Drafts().UpdateDraftState(id, model.DraftConflicted, at)
		if updateErr == nil {
			_ = h.broker.Publish(context.Background(), hruntime.Event{
				ID:         conflicted.ID,
				Type:       hruntime.EventDraftStateChanged,
				Source:     "apply_draft_conflict",
				OccurredAt: at,
				Payload:    conflicted,
			})
		}
		return model.Draft{}, store.ErrConflict
	}

	next, err := applyDraftPatch(current, draft)
	if err != nil {
		return model.Draft{}, err
	}
	if _, err := vault.WriteFileAtomic(targetAbs, next, h.cfg.Vault.TempSuffix); err != nil {
		return model.Draft{}, err
	}

	applied, err := h.transitionDraftState(id, model.DraftApplied, "apply_draft", "draft-applied-state", "operator", at)
	if err != nil {
		return model.Draft{}, err
	}
	h.recordAudit(model.AuditRecord{
		ID:            auditID("draft-applied", at),
		Kind:          model.AuditDraftApplied,
		CorrelationID: applied.ID,
		Actor:         "operator",
		Target:        applied.Target.Path,
		OccurredAt:    at,
		Metadata: map[string]string{
			"draft_id": applied.ID,
			"state":    string(applied.State),
		},
	})
	return applied, nil
}

func (h *Harness) WriteLowRiskNote(relPath string, content []byte, overwrite bool, at time.Time) (model.VaultDocument, error) {
	normalizedPath := cleanRelPath(relPath)
	if !isLowRiskWritePath(normalizedPath) {
		return model.VaultDocument{}, ErrDirectWriteDenied
	}
	if !h.allowsLowRiskDirectWrite(normalizedPath) {
		return model.VaultDocument{}, ErrDirectWriteDenied
	}

	targetAbs := filepath.Join(h.cfg.Paths.VaultRoot, filepath.FromSlash(normalizedPath))
	if !overwrite {
		if _, err := os.Stat(targetAbs); err == nil {
			return model.VaultDocument{}, os.ErrExist
		} else if !os.IsNotExist(err) {
			return model.VaultDocument{}, err
		}
	}

	hash, err := vault.WriteFileAtomic(targetAbs, content, h.cfg.Vault.TempSuffix)
	if err != nil {
		return model.VaultDocument{}, err
	}

	doc := model.VaultDocument{
		Path:        normalizedPath,
		DocClass:    model.DocClassNote,
		BaseVersion: hash,
		Content:     string(content),
		Attachments: vault.ExtractAttachmentRefs(string(content)),
	}
	h.recordAudit(model.AuditRecord{
		ID:         auditID("low-risk-write", at),
		Kind:       model.AuditLowRiskVaultWrite,
		Actor:      "operator",
		Target:     normalizedPath,
		OccurredAt: at,
		Metadata: map[string]string{
			"doc_class": string(doc.DocClass),
			"overwrite": fmt.Sprintf("%t", overwrite),
		},
	})
	return doc, nil
}

func (h *Harness) allowsLowRiskDirectWrite(relPath string) bool {
	if relPath == "" || vault.ShouldIgnoreRelativePath(relPath) {
		return false
	}
	if sameRelPath(relPath, h.cfg.Vault.ManagedCore.SystemDoc) ||
		sameRelPath(relPath, h.cfg.Vault.ManagedCore.ProgressIndex) ||
		sameRelPath(relPath, h.cfg.Vault.ManagedCore.Persona) ||
		sameRelPath(relPath, h.cfg.Vault.ManagedCore.AgentDoc) ||
		sameRelPath(relPath, h.cfg.Vault.ManagedCore.IdentityDoc) {
		return false
	}
	if isUnderRelPath(relPath, processSinkRelDir(h.cfg.Paths.VaultRoot, h.cfg.Paths.ProcessSinkDir)) {
		return false
	}

	classification := h.classifier.Classify(relPath)
	return classification.Class == model.DocClassUnknown || classification.Class == model.DocClassNote
}

func (h *Harness) transitionDraftState(id string, next model.DraftState, source string, auditPrefix string, actor string, at time.Time) (model.Draft, error) {
	draft, err := h.store.Drafts().GetDraft(id)
	if err != nil {
		return model.Draft{}, err
	}
	if err := drafts.ValidateTransition(draft.State, next); err != nil {
		return model.Draft{}, ErrDraftNotReady
	}

	updated, err := h.store.Drafts().UpdateDraftState(id, next, at)
	if err != nil {
		return model.Draft{}, err
	}
	_ = h.broker.Publish(context.Background(), hruntime.Event{
		ID:         updated.ID,
		Type:       hruntime.EventDraftStateChanged,
		Source:     source,
		OccurredAt: at,
		Payload:    updated,
	})
	h.recordAudit(model.AuditRecord{
		ID:            auditID(auditPrefix, at),
		Kind:          model.AuditDraftStateChange,
		CorrelationID: updated.ID,
		Actor:         actor,
		Target:        updated.Target.Path,
		OccurredAt:    at,
		Metadata: map[string]string{
			"draft_id": updated.ID,
			"state":    string(updated.State),
		},
	})
	return updated, nil
}

func (h *Harness) IngestSessionWindow(window model.SessionWindow, title string, content string, rawTranscript string, at time.Time) (model.CheckpointDoc, error) {
	doc, err := h.sink.WriteCheckpoint(window, title, content, rawTranscript, at)
	if err != nil {
		return model.CheckpointDoc{}, err
	}
	_ = h.broker.Publish(context.Background(), hruntime.Event{
		ID:         doc.WindowKey,
		Type:       hruntime.EventCheckpointWritten,
		Source:     "ingest_session_window",
		OccurredAt: at,
		Payload:    doc,
	})
	h.recordAudit(model.AuditRecord{
		ID:         auditID("checkpoint-write", at),
		Kind:       model.AuditCheckpointWrite,
		Actor:      "process_sink",
		Target:     doc.Path,
		OccurredAt: at,
		Metadata: map[string]string{
			"window_key": doc.WindowKey,
			"state":      string(doc.State),
		},
	})
	return doc, nil
}

func (h *Harness) RollupDaily(agentID string, day time.Time, at time.Time) (model.DailyReport, error) {
	report, err := h.sink.RollupDay(agentID, day, at)
	if err != nil {
		return model.DailyReport{}, err
	}
	return h.publishDailyRollup(agentID, report, at), nil
}

func (h *Harness) RollupDailyWithSummary(agentID string, day time.Time, title string, content string, at time.Time) (model.DailyReport, error) {
	report, err := h.sink.RollupDayWithSummary(agentID, day, title, content, at)
	if err != nil {
		return model.DailyReport{}, err
	}
	return h.publishDailyRollup(agentID, report, at), nil
}

func (h *Harness) publishDailyRollup(agentID string, report model.DailyReport, at time.Time) model.DailyReport {
	_ = h.broker.Publish(context.Background(), hruntime.Event{
		ID:         report.Path,
		Type:       hruntime.EventDailyRollupWritten,
		Source:     "rollup_daily",
		OccurredAt: at,
		Payload:    report,
	})
	h.recordAudit(model.AuditRecord{
		ID:         auditID("daily-rollup", at),
		Kind:       model.AuditDailyRollup,
		Actor:      "process_sink",
		Target:     report.Path,
		OccurredAt: at,
		Metadata: map[string]string{
			"agent_id": agentID,
			"day":      report.ReportDay.Format("2006-01-02"),
		},
	})
	return report
}

func isLowRiskWritePath(relPath string) bool {
	relPath = cleanRelPath(relPath)
	if relPath == "" || relPath == ".." || strings.HasPrefix(relPath, "../") {
		return false
	}
	if filepath.IsAbs(filepath.FromSlash(relPath)) || strings.Contains(relPath, ":") {
		return false
	}
	return strings.EqualFold(filepath.Ext(relPath), ".md")
}

func sameRelPath(left string, right string) bool {
	return strings.EqualFold(cleanRelPath(left), cleanRelPath(right))
}

func isUnderRelPath(pathValue string, rootValue string) bool {
	pathValue = cleanRelPath(pathValue)
	rootValue = cleanRelPath(rootValue)
	if pathValue == "" || rootValue == "" {
		return false
	}
	return pathValue == rootValue || strings.HasPrefix(pathValue, strings.TrimRight(rootValue, "/")+"/")
}

func processSinkRelDir(vaultRoot string, processSinkDir string) string {
	rel, err := filepath.Rel(filepath.Clean(vaultRoot), filepath.Clean(processSinkDir))
	if err != nil {
		return ""
	}
	return cleanRelPath(rel)
}

type fileWriter struct {
	tempSuffix string
}

func (w fileWriter) Write(path string, content []byte) error {
	_, err := vault.WriteFileAtomic(path, content, w.tempSuffix)
	return err
}

func applyDraftPatch(current []byte, draft model.Draft) ([]byte, error) {
	switch draft.Kind {
	case model.DraftKindProgressSync:
		return upsertProgressIndexRow(current, draft.ProposedContent)
	case model.DraftKindPersonaUpdate:
		return appendPersonaUpdateRecord(current, draft)
	default:
		return nil, ErrUnsupportedDraft
	}
}

func appendPersonaUpdateRecord(current []byte, draft model.Draft) ([]byte, error) {
	var proposal model.PersonaUpdateProposal
	if err := json.Unmarshal([]byte(draft.ProposedContent), &proposal); err != nil {
		return nil, ErrInvalidDraftPatch
	}
	proposal.Field = strings.TrimSpace(proposal.Field)
	proposal.CurrentValue = strings.TrimSpace(proposal.CurrentValue)
	proposal.ProposedValue = strings.TrimSpace(proposal.ProposedValue)
	proposal.Evidence = strings.TrimSpace(proposal.Evidence)
	proposal.Reason = strings.TrimSpace(proposal.Reason)
	proposal.Confidence = strings.TrimSpace(proposal.Confidence)
	proposal.Source = strings.TrimSpace(proposal.Source)
	if proposal.Field == "" || proposal.ProposedValue == "" || proposal.Evidence == "" || proposal.Reason == "" || proposal.Confidence == "" || proposal.Source == "" || proposal.ObservedAt.IsZero() {
		return nil, ErrInvalidDraftPatch
	}

	text := strings.ReplaceAll(string(current), "\r\n", "\n")
	text = strings.TrimRight(text, "\n")
	record := renderPersonaUpdateRecord(proposal, draft.ID)
	if strings.TrimSpace(text) == "" {
		return []byte("## Applied Persona Updates\n\n" + record), nil
	}
	lines := strings.Split(text, "\n")
	headerIndex := findMarkdownHeading(lines, "## Applied Persona Updates")
	if headerIndex < 0 {
		return []byte(text + "\n\n## Applied Persona Updates\n\n" + record), nil
	}
	insertAt := len(lines)
	for i := headerIndex + 1; i < len(lines); i++ {
		if strings.HasPrefix(strings.TrimSpace(lines[i]), "## ") {
			insertAt = i
			break
		}
	}
	lines = insertLine(lines, insertAt, strings.TrimRight(record, "\n"))
	return []byte(strings.Join(lines, "\n") + "\n"), nil
}

func renderPersonaUpdateRecord(proposal model.PersonaUpdateProposal, draftID string) string {
	var builder strings.Builder
	builder.WriteString("- field: ")
	builder.WriteString(markdownInline(proposal.Field))
	builder.WriteString("\n")
	if proposal.CurrentValue != "" {
		builder.WriteString("  current_value: ")
		builder.WriteString(markdownInline(proposal.CurrentValue))
		builder.WriteString("\n")
	}
	builder.WriteString("  proposed_value: ")
	builder.WriteString(markdownInline(proposal.ProposedValue))
	builder.WriteString("\n")
	builder.WriteString("  evidence: ")
	builder.WriteString(markdownInline(proposal.Evidence))
	builder.WriteString("\n")
	builder.WriteString("  reason: ")
	builder.WriteString(markdownInline(proposal.Reason))
	builder.WriteString("\n")
	builder.WriteString("  confidence: ")
	builder.WriteString(markdownInline(proposal.Confidence))
	builder.WriteString("\n")
	builder.WriteString("  source: ")
	builder.WriteString(markdownInline(proposal.Source))
	builder.WriteString("\n")
	builder.WriteString("  observed_at: ")
	builder.WriteString(proposal.ObservedAt.UTC().Format(time.RFC3339))
	builder.WriteString("\n")
	builder.WriteString("  draft_id: ")
	builder.WriteString(markdownInline(draftID))
	builder.WriteString("\n")
	return builder.String()
}

func findMarkdownHeading(lines []string, heading string) int {
	for i, line := range lines {
		if strings.TrimSpace(line) == heading {
			return i
		}
	}
	return -1
}

func markdownInline(value string) string {
	return strings.Join(strings.Fields(strings.TrimSpace(value)), " ")
}

func renderProgressPatch(relPath string, docClass model.DocClass, at time.Time) string {
	return fmt.Sprintf(
		"| %s | %s | %s | %s |",
		relPath,
		progressDocTypeLabel(docClass),
		"\u5df2\u540c\u6b65",
		at.Format("2006-01-02 15:04"),
	)
}

func summarizeContent(content []byte) string {
	preview := strings.TrimSpace(string(content))
	if preview == "" {
		return "no preview"
	}
	if len(preview) > 80 {
		return preview[:80] + "..."
	}
	return preview
}

func progressDocTypeLabel(docClass model.DocClass) string {
	switch docClass {
	case model.DocClassPlanWeek:
		return "\u5468\u6267\u884c"
	case model.DocClassPlanMaster:
		return "\u8ba1\u5212\u603b\u8868"
	case model.DocClassSystemDoc:
		return "\u7cfb\u7edf\u6587\u6863"
	case model.DocClassPersona:
		return "\u4eba\u7269\u753b\u50cf"
	default:
		return string(docClass)
	}
}

func upsertProgressIndexRow(current []byte, row string) ([]byte, error) {
	row = strings.TrimSpace(row)
	cells, ok := parseMarkdownRow(row)
	if !ok || len(cells) < 4 {
		return nil, ErrInvalidDraftPatch
	}

	if len(current) == 0 {
		return []byte(progressIndexTableBlock(row) + "\n"), nil
	}

	text := strings.ReplaceAll(string(current), "\r\n", "\n")
	lines := strings.Split(strings.TrimRight(text, "\n"), "\n")
	headerIndex, separatorIndex, endIndex := findProgressTable(lines)
	if headerIndex < 0 {
		trimmed := strings.TrimSpace(text)
		if trimmed == "" {
			return []byte(progressIndexTableBlock(row) + "\n"), nil
		}
		return []byte(strings.TrimRight(text, "\n") + "\n\n## 闁煎浜滄慨鈺呭触鐏炵虎鍔勯悹浣规緲缂嶅硵n\n" + progressIndexTableBlock(row) + "\n"), nil
	}

	replaced := false
	for i := separatorIndex + 1; i < endIndex; i++ {
		existingCells, ok := parseMarkdownRow(lines[i])
		if !ok || len(existingCells) == 0 {
			continue
		}
		if existingCells[0] == cells[0] {
			lines[i] = row
			replaced = true
			break
		}
	}

	if !replaced {
		lines = insertLine(lines, endIndex, row)
	}
	return []byte(strings.Join(lines, "\n") + "\n"), nil
}

func progressIndexTableBlock(row string) string {
	return strings.Join([]string{
		"| 闁哄倸娲﹂妴?| 缂侇偉顕ч悗?| 闁绘鍩栭埀?| 闁哄牃鍋撻弶鈺傚灦濞插潡寮?|",
		"| --- | --- | --- | --- |",
		row,
	}, "\n")
}

func findProgressTable(lines []string) (int, int, int) {
	for i := 0; i < len(lines)-1; i++ {
		if !isProgressHeaderLine(lines[i]) || !isMarkdownSeparatorLine(lines[i+1]) {
			continue
		}

		end := i + 2
		for end < len(lines) && strings.HasPrefix(strings.TrimSpace(lines[end]), "|") {
			end++
		}
		return i, i + 1, end
	}
	return -1, -1, -1
}

func isProgressHeaderLine(line string) bool {
	cells, ok := parseMarkdownRow(line)
	if !ok || len(cells) < 4 {
		return false
	}
	expected := []string{"\u6587\u6863", "\u7c7b\u578b", "\u72b6\u6001", "\u6700\u8fd1\u66f4\u65b0"}
	for i, want := range expected {
		if cells[i] != want {
			return false
		}
	}
	return true
}

func isMarkdownSeparatorLine(line string) bool {
	cells, ok := parseMarkdownRow(line)
	if !ok {
		return false
	}
	for _, cell := range cells {
		if strings.Trim(cell, "-: ") != "" {
			return false
		}
	}
	return true
}

func parseMarkdownRow(line string) ([]string, bool) {
	trimmed := strings.TrimSpace(line)
	if !strings.HasPrefix(trimmed, "|") {
		return nil, false
	}
	trimmed = strings.TrimPrefix(trimmed, "|")
	trimmed = strings.TrimSuffix(trimmed, "|")

	parts := strings.Split(trimmed, "|")
	cells := make([]string, 0, len(parts))
	for _, part := range parts {
		cells = append(cells, strings.TrimSpace(part))
	}
	return cells, true
}

func insertLine(lines []string, index int, line string) []string {
	if index < 0 {
		index = 0
	}
	if index > len(lines) {
		index = len(lines)
	}

	lines = append(lines, "")
	copy(lines[index+1:], lines[index:])
	lines[index] = line
	return lines
}

func (h *Harness) recordAudit(record model.AuditRecord) {
	if record.OccurredAt.IsZero() {
		record.OccurredAt = time.Now()
	}
	if err := h.auditor.Record(context.Background(), record); err != nil {
		h.health.MarkError("audit record failed: " + err.Error())
	}
}
func auditID(prefix string, at time.Time) string {
	return fmt.Sprintf("%s-%d", prefix, at.UnixNano())
}
