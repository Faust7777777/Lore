package cli

import (
	"testing"
	"time"

	"obsidian-harness/internal/app"
	"obsidian-harness/internal/console"
	"obsidian-harness/internal/model"
	"obsidian-harness/internal/operatoragent"
)

type workbenchRuntimeStub struct {
	managed     model.ManagedStatusView
	drafts      []model.Draft
	processSink app.ProcessSinkDayView
	review      app.DraftReview
}

func (s workbenchRuntimeStub) ManagedStatus() (model.ManagedStatusView, error) {
	return s.managed, nil
}

func (s workbenchRuntimeStub) ListDrafts() ([]model.Draft, error) {
	return append([]model.Draft(nil), s.drafts...), nil
}

func (s workbenchRuntimeStub) ReviewDraft(id string) (app.DraftReview, error) {
	return s.review, nil
}

func (s workbenchRuntimeStub) ApproveDraft(id string) (model.Draft, error) {
	return model.Draft{}, nil
}

func (s workbenchRuntimeStub) RejectDraft(id string) (model.Draft, error) {
	return model.Draft{}, nil
}

func (s workbenchRuntimeStub) RequestDraftRevision(id string) (model.Draft, error) {
	return model.Draft{}, nil
}

func (s workbenchRuntimeStub) SupersedeDraft(id string, update model.DraftSupersedeUpdate) (model.Draft, error) {
	return model.Draft{}, nil
}

func (s workbenchRuntimeStub) ApplyDraft(id string) (model.Draft, error) {
	return model.Draft{}, nil
}

func (s workbenchRuntimeStub) ProcessSinkDay(agentID string, day time.Time) (app.ProcessSinkDayView, error) {
	return s.processSink, nil
}

func (s workbenchRuntimeStub) SystemDocGet(name string) (model.VaultDocument, error) {
	return model.VaultDocument{}, nil
}

func (s workbenchRuntimeStub) VaultRead(relPath string) (model.VaultDocument, error) {
	return model.VaultDocument{}, nil
}

func (s workbenchRuntimeStub) VaultList(relDir string) ([]model.VaultEntry, error) {
	return nil, nil
}

func (s workbenchRuntimeStub) VaultSearchText(query string, relDir string, limit int) ([]model.SearchHit, error) {
	return nil, nil
}

func (s workbenchRuntimeStub) VaultResolve(query string, relDir string, limit int) (model.VaultResolveResult, error) {
	return model.VaultResolveResult{}, nil
}
func (s workbenchRuntimeStub) VaultBacklinks(relPath string, limit int) ([]model.SearchHit, error) {
	return nil, nil
}

func (s workbenchRuntimeStub) DocClassify(relPath string) model.DocClassificationView {
	return model.DocClassificationView{}
}

func (s workbenchRuntimeStub) ContextPack(targetPath string, task string, limit int) (model.ContextPack, error) {
	return model.ContextPack{}, nil
}

func (s workbenchRuntimeStub) WriteLowRiskNote(relPath string, content string, overwrite bool) (model.VaultDocument, error) {
	return model.VaultDocument{}, nil
}

func (s workbenchRuntimeStub) BuildCoreContext(limit int) (model.CoreContext, error) {
	return model.CoreContext{}, nil
}

func (s workbenchRuntimeStub) RecordUsage(records []model.UsageRecord) error {
	return nil
}

func (s workbenchRuntimeStub) WorkDirPath() string {
	return s.managed.WorkDir
}

func (s workbenchRuntimeStub) VaultRootPath() string {
	return s.managed.VaultRoot
}

func (s workbenchRuntimeStub) StateDirPath() string {
	return "state"
}

func TestLoadWorkbenchViewModelBuildsSnapshotFromRuntimeAndSession(t *testing.T) {
	day := time.Date(2026, 4, 23, 0, 0, 0, 0, time.Local)
	runtime := workbenchRuntimeStub{
		managed: model.ManagedStatusView{
			Ready:     true,
			WorkDir:   "work",
			VaultRoot: "vault",
			Health:    model.HealthSnapshot{Outcome: model.NewOutcome(model.StatusOK), Message: "healthy"},
			CoreDocs: []model.ManagedCoreStatus{
				{Name: "system", Exists: true, Path: "00-system/system.md"},
			},
		},
		drafts: []model.Draft{
			{ID: "draft-1", State: model.DraftPendingReview},
			{ID: "draft-2", State: model.DraftApproved},
		},
		processSink: app.ProcessSinkDayView{
			AgentID: "codex",
			Day:     day,
			Checkpoints: []model.CheckpointDoc{{
				Window: model.SessionWindow{
					AgentID:     "codex",
					SessionID:   "session-1",
					WindowStart: day.Add(9 * time.Hour),
					WindowEnd:   day.Add(9*time.Hour + 30*time.Minute),
				},
				State: model.CheckpointMaterialized,
				Title: "checkpoint",
			}},
		},
		review: app.DraftReview{
			Draft: model.Draft{
				ID:              "draft-1",
				State:           model.DraftPendingReview,
				Summary:         "summary",
				ProposedContent: "- [x] done",
			},
			BaseVersionMatches: true,
		},
	}

	session := console.NewSession("test")
	session.CurrentDraftID = "draft-1"
	session.LastToolTrace = []operatoragent.ToolCallTrace{{
		Name:   "managed_status",
		Status: "ok",
	}}
	session.History = []operatoragent.ConversationTurn{
		{Role: "user", Content: "show status"},
		{Role: "assistant", Content: "ready"},
	}

	viewModel, err := loadWorkbenchViewModel("test", runtime, session, "codex", day, true, false, "ready")
	if err != nil {
		t.Fatalf("loadWorkbenchViewModel returned error: %v", err)
	}

	if got, want := viewModel.Snapshot.Profile, "local-exec"; got != want {
		t.Fatalf("viewModel.Snapshot.Profile = %q, want %q", got, want)
	}
	if got, want := len(viewModel.PendingDrafts), 2; got != want {
		t.Fatalf("len(viewModel.PendingDrafts) = %d, want %d (pending_review + approved)", got, want)
	}
	if viewModel.FocusedReview == nil {
		t.Fatalf("viewModel.FocusedReview = nil, want non-nil")
	}
	if got, want := len(viewModel.ToolTrace), 1; got != want {
		t.Fatalf("len(viewModel.ToolTrace) = %d, want %d", got, want)
	}
	if got, want := len(viewModel.Conversation.Turns), 2; got != want {
		t.Fatalf("len(viewModel.Conversation.Turns) = %d, want %d", got, want)
	}
	if got, want := viewModel.Conversation.LastOutput, "ready"; got != want {
		t.Fatalf("viewModel.Conversation.LastOutput = %q, want %q", got, want)
	}
}
