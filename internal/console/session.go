package console

import (
	"encoding/json"
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
	VaultResolve(query string, relDir string, limit int) (model.VaultResolveResult, error)
	VaultBacklinks(relPath string, limit int) ([]model.SearchHit, error)
	DocClassify(relPath string) model.DocClassificationView
	ContextPack(targetPath string, task string, limit int) (model.ContextPack, error)
	WriteLowRiskNote(relPath string, content string, overwrite bool) (model.VaultDocument, error)
	WorkDirPath() string
	VaultRootPath() string
	StateDirPath() string
}

type TranscriptInfo struct {
	SessionID string
	Path      string
}
type TranscriptRecorder interface {
	RecordUser(text string) error
	RecordAssistant(text string) error
	RecordToolTrace(trace []operatoragent.ToolCallTrace) error
	RecordWorkingSet(items []operatoragent.WorkingSetItem) error
	RecordLocalCommand(command string) error
	RecordError(message string, recoverable bool) error
	SessionID() string
	Path() string
}

type Session struct {
	Version              string
	CurrentDraftID       string
	DefaultAgentID       string
	EnableLocalWorkTools bool
	PendingShellCommand  *pendingShellCommand
	LastInput            string
	LastToolTrace        []operatoragent.ToolCallTrace
	History              []operatoragent.ConversationTurn
	WorkingSet           []operatoragent.WorkingSetItem
	Now                  func() time.Time
	Agent                operatoragent.Agent
	Recorder             TranscriptRecorder
}
type pendingShellCommand struct {
	Command        string
	TimeoutSeconds int
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
	s.LastToolTrace = nil
	if output, handled, err := s.handlePendingShellConfirmation(runtime); handled {
		return output, err
	}
	if loopAgent, ok := s.Agent.(operatoragent.LoopAgent); ok {
		response, err := loopAgent.Respond(input, s.agentContext(), newToolRuntime(s, runtime))
		if err != nil {
			return "", err
		}
		s.LastToolTrace = append([]operatoragent.ToolCallTrace(nil), response.Trace...)
		s.recordToolTrace(s.LastToolTrace)
		s.rememberToolTargets(response.Trace, response.Final)
		s.recordWorkingSet()
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

func (s *Session) handlePendingShellConfirmation(runtime Runtime) (string, bool, error) {
	if s.PendingShellCommand == nil {
		return "", false, nil
	}

	switch parseShellConfirmationDecision(s.LastInput) {
	case shellConfirmationApproved:
		pending := *s.PendingShellCommand
		s.PendingShellCommand = nil
		result, err := runShellCommand(runtime.WorkDirPath(), pending.Command, pending.TimeoutSeconds)
		traceStatus := "ok"
		traceError := ""
		if err != nil {
			traceStatus = "error"
			traceError = strings.TrimSpace(err.Error())
		}
		s.LastToolTrace = []operatoragent.ToolCallTrace{{
			Name: "shell_exec",
			Arguments: map[string]any{
				"command":         pending.Command,
				"timeout_seconds": pending.TimeoutSeconds,
			},
			Status: traceStatus,
			Error:  traceError,
		}}
		output := strings.TrimSpace(result.Content)
		if output == "" {
			output = "Shell command finished."
		}
		s.rememberTurn(s.LastInput, output)
		if strings.HasSuffix(result.Content, "\n") {
			return result.Content, true, nil
		}
		return output + "\n", true, nil
	case shellConfirmationRejected:
		pending := *s.PendingShellCommand
		s.PendingShellCommand = nil
		s.LastToolTrace = []operatoragent.ToolCallTrace{{
			Name: "shell_exec",
			Arguments: map[string]any{
				"command":         pending.Command,
				"timeout_seconds": pending.TimeoutSeconds,
			},
			Status: "cancelled",
		}}
		output := strings.TrimSpace(fmt.Sprintf("Shell command cancelled.\ncommand: %s\n", pending.Command))
		s.rememberTurn(s.LastInput, output)
		return output + "\n", true, nil
	default:
		return "", false, nil
	}
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

func (s *Session) TranscriptInfo() TranscriptInfo {
	if s == nil || s.Recorder == nil {
		return TranscriptInfo{}
	}
	return TranscriptInfo{SessionID: s.Recorder.SessionID(), Path: s.Recorder.Path()}
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
		WorkingSet:     append([]operatoragent.WorkingSetItem(nil), s.WorkingSet...),
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
	s.recordUser(input)
	s.recordAssistant(output)
	if len(s.History) > 20 {
		s.History = append([]operatoragent.ConversationTurn(nil), s.History[len(s.History)-20:]...)
	}
}

func (s *Session) rememberToolTargets(trace []operatoragent.ToolCallTrace, final string) {
	for _, item := range trace {
		s.rememberToolArgumentPath(item.Name, item.Arguments)
	}
	s.rememberPathsFromText(final, "assistant")
	if len(s.WorkingSet) > 8 {
		s.WorkingSet = append([]operatoragent.WorkingSetItem(nil), s.WorkingSet[len(s.WorkingSet)-8:]...)
	}
}

func (s *Session) rememberToolArgumentPath(toolName string, arguments map[string]any) {
	for _, key := range []string{"path", "target_path"} {
		value, _ := arguments[key].(string)
		if isVaultMarkdownPath(value) {
			s.rememberWorkingSetItem(operatoragent.WorkingSetItem{Kind: "vault_path", Path: strings.TrimSpace(value), Source: strings.TrimSpace(toolName)})
		}
	}
}

func (s *Session) rememberPathsFromText(value string, source string) {
	var decoded any
	if err := json.Unmarshal([]byte(strings.TrimSpace(value)), &decoded); err == nil {
		s.rememberPathsFromJSON(decoded, source)
		return
	}
	for _, path := range markdownPathsFromText(value) {
		s.rememberWorkingSetItem(operatoragent.WorkingSetItem{Kind: "vault_path", Path: path, Source: source})
	}
}

func (s *Session) rememberPathsFromJSON(value any, source string) {
	switch typed := value.(type) {
	case map[string]any:
		if path, _ := typed["path"].(string); isVaultMarkdownPath(path) {
			s.rememberWorkingSetItem(operatoragent.WorkingSetItem{Kind: "vault_path", Path: strings.TrimSpace(path), Source: source})
		}
		if path, _ := typed["target_path"].(string); isVaultMarkdownPath(path) {
			s.rememberWorkingSetItem(operatoragent.WorkingSetItem{Kind: "vault_path", Path: strings.TrimSpace(path), Source: source})
		}
		for _, child := range typed {
			s.rememberPathsFromJSON(child, source)
		}
	case []any:
		for _, child := range typed {
			s.rememberPathsFromJSON(child, source)
		}
	}
}

func (s *Session) rememberWorkingSetItem(item operatoragent.WorkingSetItem) {
	path := strings.TrimSpace(item.Path)
	if !isVaultMarkdownPath(path) {
		return
	}
	item.Path = path
	item.Kind = withDefault(strings.TrimSpace(item.Kind), "vault_path")
	item.Source = strings.TrimSpace(item.Source)

	filtered := s.WorkingSet[:0]
	for _, existing := range s.WorkingSet {
		if !strings.EqualFold(strings.TrimSpace(existing.Path), path) {
			filtered = append(filtered, existing)
		}
	}
	s.WorkingSet = append(filtered, item)
}

func markdownPathsFromText(value string) []string {
	fields := strings.FieldsFunc(value, func(r rune) bool {
		switch r {
		case ' ', '\n', '\r', '\t', '`', '"', '\'', '，', '。', '、', '：', ':', '；', ';', '(', ')', '（', '）', '[', ']', '【', '】':
			return true
		default:
			return false
		}
	})
	paths := make([]string, 0)
	for _, field := range fields {
		candidate := strings.Trim(strings.TrimSpace(field), ".,!?！？")
		if isVaultMarkdownPath(candidate) {
			paths = append(paths, candidate)
		}
	}
	return paths
}

func isVaultMarkdownPath(value string) bool {
	value = strings.TrimSpace(strings.ReplaceAll(value, "\\", "/"))
	return value != "" && strings.HasSuffix(strings.ToLower(value), ".md") && !strings.HasPrefix(value, "/") && !strings.Contains(value, ":")
}

func withDefault(value string, fallback string) string {
	if value != "" {
		return value
	}
	return fallback
}

func (s *Session) recordUser(text string) {
	if s.Recorder != nil && strings.TrimSpace(text) != "" {
		_ = s.Recorder.RecordUser(text)
	}
}

func (s *Session) recordAssistant(text string) {
	if s.Recorder != nil && strings.TrimSpace(text) != "" {
		_ = s.Recorder.RecordAssistant(text)
	}
}

func (s *Session) recordToolTrace(trace []operatoragent.ToolCallTrace) {
	if s.Recorder != nil && len(trace) > 0 {
		_ = s.Recorder.RecordToolTrace(trace)
	}
}

func (s *Session) recordWorkingSet() {
	if s.Recorder != nil && len(s.WorkingSet) > 0 {
		_ = s.Recorder.RecordWorkingSet(s.WorkingSet)
	}
}

func (s *Session) recordError(err error, recoverable bool) {
	if s.Recorder != nil && err != nil {
		_ = s.Recorder.RecordError(err.Error(), recoverable)
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
  - shell commands always ask for confirmation before execution
  - this console requires a configured model-backed operator agent
`) + "\n"
}

func defaultAgentID(value string) string {
	if value == "" {
		return "codex"
	}
	return value
}

type shellConfirmationDecision int

const (
	shellConfirmationUnknown shellConfirmationDecision = iota
	shellConfirmationApproved
	shellConfirmationRejected
)

func parseShellConfirmationDecision(input string) shellConfirmationDecision {
	value := strings.ToLower(strings.TrimSpace(input))
	switch value {
	case "y", "yes", "ok", "okay", "confirm", "confirmed", "run", "run it", "execute", "execute it", "go ahead", "continue", "sure", "\u786e\u8ba4", "\u6267\u884c", "\u8fd0\u884c", "\u7ee7\u7eed", "\u597d", "\u53ef\u4ee5":
		return shellConfirmationApproved
	case "n", "no", "cancel", "stop", "skip", "abort", "\u53d6\u6d88", "\u4e0d\u7528", "\u4e0d\u8981", "\u7b97\u4e86", "\u522b\u6267\u884c", "\u505c\u6b62":
		return shellConfirmationRejected
	default:
		return shellConfirmationUnknown
	}
}
