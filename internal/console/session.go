package console

import (
	"fmt"
	"strings"
	"time"

	"obsidian-harness/internal/app"
	"obsidian-harness/internal/model"
	"obsidian-harness/internal/operatoragent"
	"obsidian-harness/internal/tui"
)

type Runtime interface {
	ManagedStatus() (model.ManagedStatusView, error)
	ListDrafts() ([]model.Draft, error)
	ReviewDraft(id string) (app.DraftReview, error)
	ApproveDraft(id string) (model.Draft, error)
	RejectDraft(id string) (model.Draft, error)
	RequestDraftRevision(id string) (model.Draft, error)
	ApplyDraft(id string) (model.Draft, error)
	ProcessSinkDay(agentID string, day time.Time) (app.ProcessSinkDayView, error)
}

type Session struct {
	Version        string
	CurrentDraftID string
	DefaultAgentID string
	Now            func() time.Time
	Agent          operatoragent.Agent
}

func NewSession(version string) *Session {
	return NewSessionWithAgent(version, operatoragent.NewDefault())
}

func NewSessionWithAgent(version string, agent operatoragent.Agent) *Session {
	if agent == nil {
		agent = operatoragent.NewFallback()
	}
	return &Session{
		Version:        strings.TrimSpace(version),
		DefaultAgentID: "codex",
		Now:            time.Now,
		Agent:          agent,
	}
}

func (s *Session) Handle(input string, runtime Runtime) (string, error) {
	decision, err := s.Agent.Decide(input, s.agentContext())
	if err != nil {
		return "", err
	}

	switch decision.Action {
	case operatoragent.ActionHelp:
		return renderConsoleHelp(), nil
	case operatoragent.ActionShowStatus:
		managed, err := runtime.ManagedStatus()
		if err != nil {
			return "", err
		}
		return tui.RenderManagedStatus(s.Version, managed), nil
	case operatoragent.ActionListDrafts:
		drafts, err := runtime.ListDrafts()
		if err != nil {
			return "", err
		}
		return tui.RenderDraftList(filterDraftsForDecision(drafts, decision)), nil
	case operatoragent.ActionReviewDraft:
		draftID, err := s.resolveDraftID(decision, runtime, model.DraftPendingReview)
		if err != nil {
			return "", err
		}
		review, err := runtime.ReviewDraft(draftID)
		if err != nil {
			return "", err
		}
		s.CurrentDraftID = review.Draft.ID
		return tui.RenderDraftReview(review), nil
	case operatoragent.ActionApproveDraft:
		draftID, err := s.resolveDraftID(decision, runtime, model.DraftPendingReview)
		if err != nil {
			return "", err
		}
		draft, err := runtime.ApproveDraft(draftID)
		if err != nil {
			return "", err
		}
		s.CurrentDraftID = draft.ID
		return tui.RenderDraftActionResult("approve", draft), nil
	case operatoragent.ActionRejectDraft:
		draftID, err := s.resolveDraftID(decision, runtime, model.DraftPendingReview)
		if err != nil {
			return "", err
		}
		draft, err := runtime.RejectDraft(draftID)
		if err != nil {
			return "", err
		}
		s.CurrentDraftID = draft.ID
		return tui.RenderDraftActionResult("reject", draft), nil
	case operatoragent.ActionRequestDraftRevision:
		draftID, err := s.resolveDraftID(decision, runtime, model.DraftPendingReview)
		if err != nil {
			return "", err
		}
		draft, err := runtime.RequestDraftRevision(draftID)
		if err != nil {
			return "", err
		}
		s.CurrentDraftID = draft.ID
		return tui.RenderDraftActionResult("request-revision", draft), nil
	case operatoragent.ActionApplyDraft:
		draftID, err := s.resolveDraftID(decision, runtime, model.DraftApproved)
		if err != nil {
			return "", err
		}
		draft, err := runtime.ApplyDraft(draftID)
		if err != nil {
			return "", err
		}
		s.CurrentDraftID = draft.ID
		return tui.RenderDraftActionResult("apply", draft), nil
	case operatoragent.ActionShowProcessSinkDay:
		view, err := runtime.ProcessSinkDay(decision.AgentID, decision.Day)
		if err != nil {
			return "", err
		}
		return tui.RenderProcessSinkDay(view), nil
	default:
		return "", fmt.Errorf("unsupported operator action: %s", decision.Action)
	}
}

func (s *Session) agentContext() operatoragent.Context {
	now := time.Now()
	if s.Now != nil {
		now = s.Now()
	}
	return operatoragent.Context{
		CurrentDraftID: strings.TrimSpace(s.CurrentDraftID),
		DefaultAgentID: defaultAgentID(strings.TrimSpace(s.DefaultAgentID)),
		Now:            now,
	}
}

func (s *Session) resolveDraftID(decision operatoragent.Decision, runtime Runtime, preferredState model.DraftState) (string, error) {
	if decision.DraftID != "" {
		return decision.DraftID, nil
	}
	if decision.UseFocusedDraft && s.CurrentDraftID != "" {
		return s.CurrentDraftID, nil
	}

	drafts, err := runtime.ListDrafts()
	if err != nil {
		return "", err
	}
	matches := selectDraftCandidates(drafts, preferredState)
	if len(matches) == 1 {
		return matches[0].ID, nil
	}
	if len(matches) > 1 {
		return "", fmt.Errorf("multiple drafts match; please specify a draft id")
	}
	if len(drafts) == 1 {
		return drafts[0].ID, nil
	}
	return "", fmt.Errorf("draft id is required")
}

func selectDraftCandidates(drafts []model.Draft, preferredState model.DraftState) []model.Draft {
	if preferredState == "" {
		return drafts
	}
	out := make([]model.Draft, 0, len(drafts))
	for _, draft := range drafts {
		if draft.State == preferredState {
			out = append(out, draft)
		}
	}
	return out
}

func filterDraftsForDecision(drafts []model.Draft, decision operatoragent.Decision) []model.Draft {
	if !decision.PendingOnly {
		return drafts
	}
	out := make([]model.Draft, 0, len(drafts))
	for _, draft := range drafts {
		if draft.State == model.DraftPendingReview {
			out = append(out, draft)
		}
	}
	return out
}

func renderConsoleHelp() string {
	return strings.TrimSpace(`
Natural Language Console
========================
Examples:
  show status
  list drafts
  review draft draft-123
  approve current draft
  show codex daily report 2026-04-22
Notes:
  - the operator agent chooses one explicit action at a time
  - timed jobs still belong to runtime/scheduler, not this console
  - the built-in fallback understands basic English and Chinese prompts
`) + "\n"
}

func defaultAgentID(value string) string {
	if value == "" {
		return "codex"
	}
	return value
}
