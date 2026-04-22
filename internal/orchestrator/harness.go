package orchestrator

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"obsidian-harness/internal/bootstrap"
	"obsidian-harness/internal/config"
	"obsidian-harness/internal/domain/docclass"
	"obsidian-harness/internal/domain/processsink"
	"obsidian-harness/internal/model"
	hruntime "obsidian-harness/internal/runtime"
	"obsidian-harness/internal/store"
	"obsidian-harness/internal/vault"
)

var (
	ErrUnsupportedDocument = errors.New("orchestrator: unsupported document class")
	ErrDraftNotReady       = errors.New("orchestrator: draft is not ready for requested operation")
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
	_ = h.auditor.Record(context.Background(), model.AuditRecord{
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
	classification := h.classifier.Classify(relPath)
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
		Title:           fmt.Sprintf("Progress sync for %s", filepath.Base(relPath)),
		Summary:         fmt.Sprintf("Backfill progress after managed document change in %s", relPath),
		ProposedContent: renderProgressPatch(relPath, content, at),
		EvidenceRefs:    []string{relPath},
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
	_ = h.auditor.Record(context.Background(), model.AuditRecord{
		ID:         auditID("draft-created", at),
		Kind:       model.AuditDraftCreated,
		Actor:      "runtime",
		Target:     draft.Target.Path,
		OccurredAt: at,
		Metadata: map[string]string{
			"draft_id": draft.ID,
			"source":   relPath,
		},
	})
	return draft, nil
}

func (h *Harness) ApproveDraft(id string, at time.Time) (model.Draft, error) {
	draft, err := h.store.Drafts().GetDraft(id)
	if err != nil {
		return model.Draft{}, err
	}
	if draft.State != model.DraftPendingReview {
		return model.Draft{}, ErrDraftNotReady
	}

	updated, err := h.store.Drafts().UpdateDraftState(id, model.DraftApproved, at)
	if err != nil {
		return model.Draft{}, err
	}
	_ = h.broker.Publish(context.Background(), hruntime.Event{
		ID:         updated.ID,
		Type:       hruntime.EventDraftStateChanged,
		Source:     "approve_draft",
		OccurredAt: at,
		Payload:    updated,
	})
	_ = h.auditor.Record(context.Background(), model.AuditRecord{
		ID:         auditID("draft-approved", at),
		Kind:       model.AuditDraftStateChange,
		Actor:      "reviewer",
		Target:     updated.Target.Path,
		OccurredAt: at,
		Metadata: map[string]string{
			"draft_id": updated.ID,
			"state":    string(updated.State),
		},
	})
	return updated, nil
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

	next := appendPatch(current, draft.ProposedContent)
	if _, err := vault.WriteFileAtomic(targetAbs, next, h.cfg.Vault.TempSuffix); err != nil {
		return model.Draft{}, err
	}

	applied, err := h.store.Drafts().UpdateDraftState(id, model.DraftApplied, at)
	if err != nil {
		return model.Draft{}, err
	}
	_ = h.broker.Publish(context.Background(), hruntime.Event{
		ID:         applied.ID,
		Type:       hruntime.EventDraftStateChanged,
		Source:     "apply_draft",
		OccurredAt: at,
		Payload:    applied,
	})
	_ = h.auditor.Record(context.Background(), model.AuditRecord{
		ID:         auditID("draft-applied", at),
		Kind:       model.AuditDraftApplied,
		Actor:      "operator",
		Target:     applied.Target.Path,
		OccurredAt: at,
		Metadata: map[string]string{
			"draft_id": applied.ID,
			"state":    string(applied.State),
		},
	})
	return applied, nil
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
	_ = h.auditor.Record(context.Background(), model.AuditRecord{
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
	_ = h.broker.Publish(context.Background(), hruntime.Event{
		ID:         report.Path,
		Type:       hruntime.EventDailyRollupWritten,
		Source:     "rollup_daily",
		OccurredAt: at,
		Payload:    report,
	})
	_ = h.auditor.Record(context.Background(), model.AuditRecord{
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
	return report, nil
}

type fileWriter struct {
	tempSuffix string
}

func (w fileWriter) Write(path string, content []byte) error {
	_, err := vault.WriteFileAtomic(path, content, w.tempSuffix)
	return err
}

func appendPatch(current []byte, patch string) []byte {
	if len(current) == 0 {
		return []byte(strings.TrimSpace(patch) + "\n")
	}

	trimmed := strings.TrimRight(string(current), "\n")
	return []byte(trimmed + "\n\n" + strings.TrimSpace(patch) + "\n")
}

func renderProgressPatch(relPath string, content []byte, at time.Time) string {
	preview := strings.TrimSpace(string(content))
	if len(preview) > 120 {
		preview = preview[:120] + "..."
	}
	return fmt.Sprintf(
		"## Auto Progress Sync %s\n\n- Source: `%s`\n- Observed At: `%s`\n- Preview: %s\n",
		at.Format(time.RFC3339),
		relPath,
		at.Format("2006-01-02 15:04"),
		preview,
	)
}

func auditID(prefix string, at time.Time) string {
	return fmt.Sprintf("%s-%d", prefix, at.UnixNano())
}
