package console

import (
	"strings"
	"testing"
	"time"

	"obsidian-harness/internal/app"
	"obsidian-harness/internal/model"
	"obsidian-harness/internal/operatoragent"
)

type fakeRuntime struct {
	managed     model.ManagedStatusView
	drafts      []model.Draft
	review      app.DraftReview
	processSink app.ProcessSinkDayView
}

func (f *fakeRuntime) ManagedStatus() (model.ManagedStatusView, error) {
	return f.managed, nil
}

func (f *fakeRuntime) ListDrafts() ([]model.Draft, error) {
	return f.drafts, nil
}

func (f *fakeRuntime) ReviewDraft(id string) (app.DraftReview, error) {
	f.review.Draft.ID = id
	return f.review, nil
}

func (f *fakeRuntime) ApproveDraft(id string) (model.Draft, error) {
	return model.Draft{ID: id, State: model.DraftApproved, Target: model.DocumentRef{Path: "progress.md"}, Title: "approved"}, nil
}

func (f *fakeRuntime) RejectDraft(id string) (model.Draft, error) {
	return model.Draft{ID: id, State: model.DraftRejected, Target: model.DocumentRef{Path: "progress.md"}, Title: "rejected"}, nil
}

func (f *fakeRuntime) RequestDraftRevision(id string) (model.Draft, error) {
	return model.Draft{ID: id, State: model.DraftRevisionRequested, Target: model.DocumentRef{Path: "progress.md"}, Title: "revision"}, nil
}

func (f *fakeRuntime) ApplyDraft(id string) (model.Draft, error) {
	return model.Draft{ID: id, State: model.DraftApplied, Target: model.DocumentRef{Path: "progress.md"}, Title: "applied"}, nil
}

func (f *fakeRuntime) ProcessSinkDay(agentID string, day time.Time) (app.ProcessSinkDayView, error) {
	f.processSink.AgentID = agentID
	f.processSink.Day = day
	return f.processSink, nil
}

type fakeAgent struct {
	decisions []operatoragent.Decision
	inputs    []string
	contexts  []operatoragent.Context
}

func (f *fakeAgent) Decide(input string, ctx operatoragent.Context) (operatoragent.Decision, error) {
	f.inputs = append(f.inputs, input)
	f.contexts = append(f.contexts, ctx)
	if len(f.decisions) == 0 {
		return operatoragent.Decision{}, nil
	}
	decision := f.decisions[0]
	f.decisions = f.decisions[1:]
	return decision, nil
}

func TestSessionHandleUsesInjectedAgentForStatusAndHelp(t *testing.T) {
	agent := &fakeAgent{
		decisions: []operatoragent.Decision{
			{Action: operatoragent.ActionShowStatus},
			{Action: operatoragent.ActionHelp},
		},
	}
	session := NewSessionWithAgent("test", agent)
	session.Now = func() time.Time { return time.Date(2026, 4, 22, 9, 0, 0, 0, time.Local) }

	runtime := &fakeRuntime{
		managed: model.ManagedStatusView{
			Ready:     true,
			WorkDir:   "work",
			VaultRoot: "vault",
			Health:    model.HealthSnapshot{Outcome: model.NewOutcome(model.StatusOK), Message: "healthy"},
		},
	}

	statusView, err := session.Handle("show me the current status", runtime)
	if err != nil {
		t.Fatalf("Handle(status) error = %v", err)
	}
	if !strings.Contains(statusView, "Managed Status") {
		t.Fatalf("status view = %q, want Managed Status", statusView)
	}

	helpView, err := session.Handle("help", runtime)
	if err != nil {
		t.Fatalf("Handle(help) error = %v", err)
	}
	if !strings.Contains(helpView, "Natural Language Console") {
		t.Fatalf("help view = %q, want Natural Language Console", helpView)
	}
	if len(agent.inputs) != 2 {
		t.Fatalf("agent inputs = %d, want 2", len(agent.inputs))
	}
}

func TestSessionHandleDraftFlowUsesFocusedDraft(t *testing.T) {
	agent := &fakeAgent{
		decisions: []operatoragent.Decision{
			{Action: operatoragent.ActionReviewDraft},
			{Action: operatoragent.ActionApproveDraft, UseFocusedDraft: true},
		},
	}
	session := NewSessionWithAgent("test", agent)
	session.Now = func() time.Time { return time.Date(2026, 4, 22, 9, 0, 0, 0, time.Local) }

	runtime := &fakeRuntime{
		drafts: []model.Draft{{
			ID:    "draft-1",
			State: model.DraftPendingReview,
			Kind:  model.DraftKindProgressSync,
			Target: model.DocumentRef{
				Path: "progress.md",
			},
			Title: "pending draft",
		}},
		review: app.DraftReview{
			Draft: model.Draft{
				ID:    "draft-1",
				State: model.DraftPendingReview,
				Kind:  model.DraftKindProgressSync,
				Target: model.DocumentRef{
					Path:  "progress.md",
					Class: model.DocClassProgressIndex,
				},
				Summary:         "review this",
				ProposedContent: "patch",
			},
			BaseVersionMatches: true,
		},
	}

	reviewView, err := session.Handle("review something", runtime)
	if err != nil {
		t.Fatalf("Handle(review) error = %v", err)
	}
	if !strings.Contains(reviewView, "Draft Review") {
		t.Fatalf("review view = %q, want Draft Review", reviewView)
	}

	approveView, err := session.Handle("approve the focused one", runtime)
	if err != nil {
		t.Fatalf("Handle(approve) error = %v", err)
	}
	if !strings.Contains(approveView, "approved") {
		t.Fatalf("approve view = %q, want approved", approveView)
	}
	if len(agent.contexts) < 2 || agent.contexts[1].CurrentDraftID != "draft-1" {
		t.Fatalf("second agent context = %+v, want CurrentDraftID draft-1", agent.contexts)
	}
}

func TestSessionHandleProcessSinkDayUsesAgentDecision(t *testing.T) {
	now := time.Date(2026, 4, 22, 11, 0, 0, 0, time.Local)
	agent := &fakeAgent{
		decisions: []operatoragent.Decision{{
			Action:  operatoragent.ActionShowProcessSinkDay,
			AgentID: "codex",
			Day:     now,
		}},
	}
	session := NewSessionWithAgent("test", agent)
	session.Now = func() time.Time { return now }

	runtime := &fakeRuntime{
		processSink: app.ProcessSinkDayView{
			Checkpoints: []model.CheckpointDoc{{
				Window: model.SessionWindow{
					AgentID:     "codex",
					SessionID:   "s1",
					WindowStart: now.Truncate(24 * time.Hour).Add(9 * time.Hour),
					WindowEnd:   now.Truncate(24 * time.Hour).Add(9*time.Hour + 30*time.Minute),
				},
				State: model.CheckpointMaterialized,
				Title: "checkpoint",
			}},
		},
	}

	view, err := session.Handle("show codex daily report", runtime)
	if err != nil {
		t.Fatalf("Handle(process sink) error = %v", err)
	}
	if !strings.Contains(view, "Process Sink Day") || !strings.Contains(view, "codex") {
		t.Fatalf("view = %q, want Process Sink Day codex", view)
	}
}
