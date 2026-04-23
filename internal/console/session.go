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
	SystemDocGet(name string) (model.VaultDocument, error)
	VaultRead(relPath string) (model.VaultDocument, error)
	VaultList(relDir string) ([]model.VaultEntry, error)
	VaultSearchText(query string, relDir string, limit int) ([]model.SearchHit, error)
	VaultBacklinks(relPath string, limit int) ([]model.SearchHit, error)
	DocClassify(relPath string) model.DocClassificationView
	ContextPack(targetPath string, task string, limit int) (model.ContextPack, error)
	WriteLowRiskNote(relPath string, content string, overwrite bool) (model.VaultDocument, error)
	WorkDirPath() string
	VaultRootPath() string
	StateDirPath() string
}

type Session struct {
	Version              string
	CurrentDraftID       string
	DefaultAgentID       string
	EnableLocalWorkTools bool
	LastInput            string
	History              []operatoragent.ConversationTurn
	Now                  func() time.Time
	Agent                operatoragent.Agent
}

func NewSession(version string) *Session {
	return NewSessionWithAgent(version, operatoragent.NewDefault())
}

func NewSessionWithAgent(version string, agent operatoragent.Agent) *Session {
	if agent == nil {
		agent = operatoragent.NewUnavailable(nil)
	}
	return &Session{
		Version:        strings.TrimSpace(version),
		DefaultAgentID: "codex",
		Now:            time.Now,
		Agent:          agent,
	}
}

func (s *Session) Handle(input string, runtime Runtime) (string, error) {
	s.LastInput = strings.TrimSpace(input)
	if loopAgent, ok := s.Agent.(operatoragent.LoopAgent); ok {
		response, err := loopAgent.Respond(input, s.agentContext(), newToolRuntime(s, runtime))
		if err != nil {
			return "", err
		}
		if response.Decision != nil {
			output, err := s.executeDecision(*response.Decision, runtime)
			if err != nil {
				return "", err
			}
			s.rememberTurn(input, output)
			return output, nil
		}
		output := strings.TrimSpace(response.Final)
		s.rememberTurn(input, output)
		if output == "" {
			return "", fmt.Errorf("operator agent returned an empty response")
		}
		if strings.HasSuffix(response.Final, "\n") {
			return response.Final, nil
		}
		return output + "\n", nil
	}

	decision, err := s.Agent.Decide(input, s.agentContext())
	if err != nil {
		return "", err
	}
	output, err := s.executeDecision(decision, runtime)
	if err != nil {
		return "", err
	}
	s.rememberTurn(input, output)
	return output, nil
}

func (s *Session) executeDecision(decision operatoragent.Decision, runtime Runtime) (string, error) {
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
		History:        append([]operatoragent.ConversationTurn(nil), s.History...),
	}
}

func (s *Session) rememberTurn(input string, output string) {
	if strings.TrimSpace(input) == "" && strings.TrimSpace(output) == "" {
		return
	}
	s.History = append(s.History,
		operatoragent.ConversationTurn{Role: "user", Content: strings.TrimSpace(input)},
		operatoragent.ConversationTurn{Role: "assistant", Content: strings.TrimSpace(output)},
	)
	if len(s.History) > 12 {
		s.History = append([]operatoragent.ConversationTurn(nil), s.History[len(s.History)-12:]...)
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
Lore Console
============
Examples:
  show status
  list drafts
  review draft draft-123
  approve current draft
  show codex daily report 2026-04-22
Notes:
  - Lore runs a bounded natural-language agent loop with internal tools
  - timed jobs still belong to runtime/scheduler, not this console
  - local workspace tools only appear in local-exec mode
  - shell still requires both local-exec mode and LORE_AGENT_ENABLE_SHELL=1
  - this console requires a configured model-backed operator agent
`) + "\n"
}

func defaultAgentID(value string) string {
	if value == "" {
		return "codex"
	}
	return value
}
