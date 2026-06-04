package orchestrator

import (
	"context"
	"errors"
	"testing"
	"time"

	"obsidian-harness/internal/config"
	"obsidian-harness/internal/model"
	hruntime "obsidian-harness/internal/runtime"
	"obsidian-harness/internal/store"
	"obsidian-harness/internal/store/memory"
)

// publishFailingBroker satisfies hruntime.Broker but rejects every
// Publish, so a test can prove a draft proposal still succeeds -- draft
// saved + audit recorded -- when the event bus is down.
type publishFailingBroker struct{}

func (publishFailingBroker) Publish(context.Context, hruntime.Event) error {
	return errors.New("broker unavailable")
}

func (publishFailingBroker) Subscribe(hruntime.EventType, int) (<-chan hruntime.Event, func()) {
	return nil, func() {}
}

// TestProposeDraftToleratesBrokerPublishFailure pins the fix that made
// ProposePersonaUpdate / ProposeMarkdownNote treat the EventDraftCreated
// publish as best-effort. Both durably SaveDraft before publishing; a
// broker hiccup must not (a) fail the proposal -- the external agent would
// think it failed and retry, creating a duplicate draft -- nor (b) skip the
// audit record that follows the publish. Mirrors the best-effort publish in
// transitionDraftState / SupersedeDraft / IngestSessionWindow.
func TestProposeDraftToleratesBrokerPublishFailure(t *testing.T) {
	cfg := config.Default(t.TempDir())
	inner := memory.New()
	h, err := New(cfg, inner)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if _, err := h.BootstrapManagedVault(time.Date(2026, 6, 4, 9, 0, 0, 0, time.UTC)); err != nil {
		t.Fatalf("BootstrapManagedVault: %v", err)
	}
	// Swap in a broker whose Publish always fails (same-package field access).
	h.broker = publishFailingBroker{}
	at := time.Date(2026, 6, 4, 10, 0, 0, 0, time.UTC)

	t.Run("persona update", func(t *testing.T) {
		result, err := h.ProposePersonaUpdate(model.PersonaUpdateProposal{
			Field:         "major",
			ProposedValue: "economics",
			Evidence:      "i study economics",
			Reason:        "stated in session",
			Confidence:    "high",
			Source:        "test",
			ObservedAt:    at,
		}, at)
		if err != nil {
			t.Fatalf("ProposePersonaUpdate must succeed despite broker failure: %v", err)
		}
		assertDraftSavedAndAudited(t, h, inner, result.DraftID)
	})

	t.Run("markdown note", func(t *testing.T) {
		result, err := h.ProposeMarkdownNote(model.MarkdownNoteProposal{
			TargetPath: "03-notes/smoke/publish-fail.md",
			Title:      "Publish Fail Note",
			Content:    "# Publish Fail Note\n\nbody",
			SourceKind: "development",
			Evidence:   "deterministic test content",
			Reason:     "verify broker tolerance",
			Source:     "test",
			ObservedAt: at,
		}, at.Add(time.Second))
		if err != nil {
			t.Fatalf("ProposeMarkdownNote must succeed despite broker failure: %v", err)
		}
		assertDraftSavedAndAudited(t, h, inner, result.DraftID)
	})
}

func assertDraftSavedAndAudited(t *testing.T, h *Harness, inner store.StateStore, draftID string) {
	t.Helper()
	if draftID == "" {
		t.Fatal("expected a non-empty draft ID")
	}
	draft, err := h.GetDraft(draftID)
	if err != nil {
		t.Fatalf("draft must be persisted despite broker failure: %v", err)
	}
	if draft.State != model.DraftPendingReview {
		t.Fatalf("draft state = %s, want pending_review", draft.State)
	}
	records, err := inner.Audit().ListAudit(64)
	if err != nil {
		t.Fatalf("ListAudit: %v", err)
	}
	for _, rec := range records {
		if rec.CorrelationID == draftID && rec.Kind == model.AuditDraftCreated {
			return
		}
	}
	t.Fatalf("expected an AuditDraftCreated record for draft %s (audit was skipped after publish failure)", draftID)
}
