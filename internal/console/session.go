package console

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
	"sync"
	"time"

	"obsidian-harness/internal/app"
	"obsidian-harness/internal/model"
	"obsidian-harness/internal/operatoragent"
	"obsidian-harness/internal/persona"
	"obsidian-harness/internal/tui"
)

// defaultPersonaExtractTimeout caps a single fire-and-forget persona
// extraction at 8 seconds. The console turn does not wait on
// extraction; this timeout only bounds how long the background
// goroutine itself runs before giving up on the LLM call. Tunable via
// Session.PersonaExtractTimeout.
const defaultPersonaExtractTimeout = 8 * time.Second

type Runtime interface {
	ManagedStatus() (model.ManagedStatusView, error)
	ListDrafts() ([]model.Draft, error)
	ReviewDraft(id string) (app.DraftReview, error)
	ApproveDraft(id string) (model.Draft, error)
	RejectDraft(id string) (model.Draft, error)
	RequestDraftRevision(id string) (model.Draft, error)
	SupersedeDraft(id string, update model.DraftSupersedeUpdate) (model.Draft, error)
	ApplyDraft(id string) (model.Draft, error)
	ProcessSinkDay(agentID string, day time.Time) (app.ProcessSinkDayView, error)
	ListFindings(limit int) ([]model.Finding, error)
	ResolveFinding(id string) (model.Finding, error)
	IgnoreFinding(id string) (model.Finding, error)
	SystemDocGet(name string) (model.VaultDocument, error)
	VaultRead(relPath string) (model.VaultDocument, error)
	VaultList(relDir string) ([]model.VaultEntry, error)
	VaultSearchText(query string, relDir string, limit int) ([]model.SearchHit, error)
	VaultResolve(query string, relDir string, limit int) (model.VaultResolveResult, error)
	VaultBacklinks(relPath string, limit int) ([]model.SearchHit, error)
	DocClassify(relPath string) model.DocClassificationView
	ContextPack(targetPath string, task string, limit int) (model.ContextPack, error)
	WriteLowRiskNote(relPath string, content string, overwrite bool) (model.VaultDocument, error)
	BuildCoreContext(limit int) (model.CoreContext, error)
	RecordUsage(records []model.UsageRecord) error
	RecordPersonaCandidate(record persona.PersonaCandidateRecord) (persona.PersonaCandidateRecord, bool, error)
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
	RecordModelUsage(usage []operatoragent.ModelCallUsage) error
	RecordTaskTurnEnd(stopReason string, stepCount int) error
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
	// LastTurnSteps holds an isolated snapshot of the structured
	// per-step record from the most recent loop-agent turn, suitable
	// for UI rendering by callers that read Session state rather than
	// re-invoke the agent. Cleared at the start of every Handle and
	// repopulated after Respond returns -- on both success and error
	// paths, so a failed turn still surfaces any tool calls that did
	// execute. The legacy non-loop Decide path leaves this empty.
	LastTurnSteps []operatoragent.TurnStep
	History       []operatoragent.ConversationTurn
	WorkingSet    []operatoragent.WorkingSetItem
	Now           func() time.Time
	Agent         operatoragent.Agent
	Recorder      TranscriptRecorder
	// PersonaExtractor mines persona candidates from each successful
	// loop-agent user turn. nil disables extraction entirely (no
	// goroutine spawned, no candidates recorded); console callers
	// wire it from runtime.PersonaExtractor when an LLM is configured.
	PersonaExtractor persona.PersonaCandidateExtractor
	// PersonaExtractTimeout caps a single fire-and-forget extraction.
	// Zero or negative falls back to defaultPersonaExtractTimeout.
	PersonaExtractTimeout time.Duration
	// PersonaExtractLogger receives one line per persona-extraction
	// failure (LLM call error, parser refusal, store write error). Nil
	// disables logging silently -- matching the legacy P4 behavior --
	// so unit tests and embedders that do not need observability stay
	// dependency-free. Console / TUI callers wire this from
	// runtime.PersonaExtractLogger which OpenRuntime points at
	// <StateDir>/logs/persona-extract.log so operators can `tail` the
	// file while running real-world sessions.
	//
	// Each line is a single \n-terminated UTF-8 record:
	//   <RFC3339Nano UTC>\tstage=<extract|store>\tsession=<id>\terror=<quoted>\n
	// The format is intentionally line-per-event tab-delimited so a
	// future `lore persona errors` reader or a plain `awk` / `grep`
	// pipeline can parse it without a structured log dependency.
	PersonaExtractLogger io.Writer
	// personaExtractWG tracks in-flight extraction goroutines so the
	// shell can call DrainPersonaExtractions before exit.
	personaExtractWG sync.WaitGroup
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
	// Clear LastTurnSteps at the top so that a turn which never
	// reaches the loop agent (legacy Decide, shell-confirmation
	// continuation, or a pre-LoopAgent early return) leaves the
	// field empty rather than displaying stale steps from a prior
	// successful turn.
	s.LastTurnSteps = nil
	if output, handled, err := s.handlePendingShellConfirmation(runtime); handled {
		return output, err
	}
	if loopAgent, ok := s.Agent.(operatoragent.LoopAgent); ok {
		response, err := loopAgent.Respond(input, s.agentContext(runtime), newToolRuntime(s, runtime))
		// Snapshot the turn's structured steps before any further
		// branching so error and success paths both leave consumers
		// with an accurate view. operatoragent guarantees Steps is
		// populated on both *UsageError post-tool-call failures and
		// on terminal final/decision returns. CloneTurnSteps deep
		// copies Arguments so subsequent UI rendering cannot leak
		// mutations back into the response value.
		s.LastTurnSteps = operatoragent.CloneTurnSteps(response.Steps)
		// Emit the task_turn_end transcript event once for this
		// loop-agent turn, regardless of which terminal Handle ends
		// at. Response.StopReason / StepCount are populated by B2 at
		// every Respond return path; the helper no-ops on empty
		// reason so legacy / pre-loop paths stay clean. Deferred
		// here (rather than inlined at each return) keeps the
		// existing control flow intact and guarantees the event
		// fires even when later steps (executeDecision,
		// persistResponseUsage) error out.
		defer s.recordTaskTurnEnd(response.StopReason, response.StepCount)
		if err != nil {
			// Failed turns may still carry usage from already-billed
			// ChatCompletion calls. Best-effort persist on the error
			// path: store/recorder failures here must not mask the
			// original Respond error the user is about to see, so
			// surface them only into the transcript error log.
			var usageErr *operatoragent.UsageError
			if errors.As(err, &usageErr) && len(usageErr.Usage) > 0 {
				if persistErr := s.persistResponseUsage(runtime, usageErr.Usage); persistErr != nil {
					s.recordError(persistErr, true)
				}
			}
			return "", err
		}
		s.LastToolTrace = append([]operatoragent.ToolCallTrace(nil), response.Trace...)
		s.recordToolTrace(s.LastToolTrace)
		s.rememberToolTargets(response.Trace, response.Final)
		s.recordWorkingSet()
		if err := s.persistResponseUsage(runtime, response.Usage); err != nil {
			return "", err
		}
		if response.Decision != nil {
			output, err := s.executeDecision(*response.Decision, runtime)
			if err != nil {
				return "", err
			}
			priorAssistant := s.priorAssistantContext()
			s.rememberTurn(input, output)
			s.launchPersonaExtraction(runtime, input, priorAssistant)
			return output, nil
		}
		output := strings.TrimSpace(response.Final)
		priorAssistant := s.priorAssistantContext()
		s.rememberTurn(input, output)
		if output == "" {
			return "", fmt.Errorf("operator agent returned an empty response")
		}
		s.launchPersonaExtraction(runtime, input, priorAssistant)
		if strings.HasSuffix(response.Final, "\n") {
			return response.Final, nil
		}
		return output + "\n", nil
	}

	decision, err := s.Agent.Decide(input, s.agentContext(runtime))
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
func (s *Session) agentContext(runtime Runtime) operatoragent.Context {
	now := time.Now()
	if s.Now != nil {
		now = s.Now()
	}
	coreContext := model.CoreContext{}
	if runtime != nil {
		if built, err := runtime.BuildCoreContext(6); err == nil {
			coreContext = built
		} else {
			coreContext.Notes = []string{"core context could not be loaded: " + err.Error()}
		}
	}
	return operatoragent.Context{
		CurrentDraftID: strings.TrimSpace(s.CurrentDraftID),
		DefaultAgentID: defaultAgentID(strings.TrimSpace(s.DefaultAgentID)),
		Now:            now,
		History:        append([]operatoragent.ConversationTurn(nil), s.History...),
		WorkingSet:     append([]operatoragent.WorkingSetItem(nil), s.WorkingSet...),
		CoreContext:    coreContext,
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
	for _, key := range []string{"path", "target_path", "selected_path"} {
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
		if s.rememberVaultResolveJSON(typed, source) {
			return
		}
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

func (s *Session) rememberVaultResolveJSON(value map[string]any, source string) bool {
	if _, ok := value["query"]; !ok {
		return false
	}
	_, hasMatches := value["matches"]
	_, hasReason := value["reason"]
	_, hasSelected := value["selected_path"]
	if !hasMatches && !hasReason && !hasSelected {
		return false
	}
	status, _ := value["status"].(string)
	switch strings.TrimSpace(status) {
	case "unique":
		path, _ := value["selected_path"].(string)
		if isVaultMarkdownPath(path) {
			s.rememberWorkingSetItem(operatoragent.WorkingSetItem{Kind: "vault_path", Path: strings.TrimSpace(path), Source: source})
		}
		return true
	case "ambiguous", "not_found":
		return true
	default:
		return false
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

// recordTaskTurnEnd emits a task_turn_end transcript event marking the
// completion of one loop-agent turn. Empty stop reason is a no-op so
// that legacy decide paths (which never populate Response.StopReason)
// produce no event. Recorder failures are intentionally swallowed:
// the event is observability metadata, not governance, and must not
// interfere with the user-facing Handle outcome.
func (s *Session) recordTaskTurnEnd(stopReason operatoragent.TurnStopReason, stepCount int) {
	if s.Recorder == nil || strings.TrimSpace(string(stopReason)) == "" {
		return
	}
	_ = s.Recorder.RecordTaskTurnEnd(string(stopReason), stepCount)
}

// composePersonaExcerpt assembles a short context snippet for the
// persona extractor by combining the current PersonaSummary with
// WeaknessSummary when present. Both are short markdown fragments
// produced by CoreContext; concatenating them gives the extractor a
// view of "who the user already is" plus "what limitations are
// already known" so the P1+P2 prompt rules can flag duplicates and
// conflicts. Returns empty string when both fields are empty so the
// extractor prompt omits the section cleanly.
func composePersonaExcerpt(coreContext model.CoreContext) string {
	persona := strings.TrimSpace(coreContext.PersonaSummary)
	weakness := strings.TrimSpace(coreContext.WeaknessSummary)
	switch {
	case persona == "" && weakness == "":
		return ""
	case persona == "":
		return weakness
	case weakness == "":
		return persona
	default:
		return persona + "\n\n" + weakness
	}
}

// priorAssistantContext returns the most recent assistant message in
// the conversation history, or "" when no prior turn exists. The
// persona extractor uses this strictly as disambiguation context so
// the LLM can interpret short user replies like "经济管理" against the
// preceding question; the prompt forbids extracting candidates from
// this section.
//
// Called BEFORE rememberTurn appends the current turn, so the
// returned string is the prior turn's assistant, not the current
// one.
func (s *Session) priorAssistantContext() string {
	if n := len(s.History); n > 0 && s.History[n-1].Role == "assistant" {
		return s.History[n-1].Content
	}
	return ""
}

// launchPersonaExtraction starts a fire-and-forget goroutine that
// asks the configured PersonaExtractor to mine candidates from the
// just-completed user turn, then writes them to the runtime's store.
// The console turn does NOT wait on the result -- Handle returns to
// the user immediately, the goroutine progresses in parallel.
//
// No goroutine is spawned when PersonaExtractor is nil (no LLM
// configured) or userText is empty (legacy edge); the caller is
// expected to have guarded both, but defensive checks here keep the
// contract crisp for direct callers.
//
// Failures (LLM error, timeout, store error) are silently swallowed:
// extraction is observability for the persona review queue, not
// governance -- a failed mining attempt must never surface to the
// user nor break the chat turn. Cost accounting still flows through
// the extractor's usage sink (set up by app.OpenRuntime) so a failed
// call that nonetheless billed tokens lands in the usage store.
func (s *Session) launchPersonaExtraction(runtime Runtime, userText, priorAssistant string) {
	if s == nil || s.PersonaExtractor == nil {
		return
	}
	if strings.TrimSpace(userText) == "" {
		return
	}
	sessionID := ""
	if s.Recorder != nil {
		sessionID = s.Recorder.SessionID()
	}
	agentID := defaultAgentID(strings.TrimSpace(s.DefaultAgentID))
	now := time.Now()
	if s.Now != nil {
		now = s.Now()
	}
	timeout := s.PersonaExtractTimeout
	if timeout <= 0 {
		timeout = defaultPersonaExtractTimeout
	}

	// Snapshot the current persona / system-rules context so the
	// extractor's P1+P2 prompt rules ("do not emit if already
	// matching current persona", "mark conflict vs current persona")
	// have something to compare against. BuildCoreContext failures
	// are non-fatal: we still launch extraction with empty context
	// rather than dropping the turn -- the parser will simply mark
	// fewer conflicts. Snapshot synchronously here so the goroutine
	// gets a stable view, decoupled from any subsequent vault
	// mutations during the extraction window.
	coreContext, _ := runtime.BuildCoreContext(6)

	input := persona.PersonaExtractionInput{
		UserText:              userText,
		AssistantContext:      priorAssistant,
		SourceKind:            persona.SourceConsole,
		SourceSessionID:       sessionID,
		SourceAgentID:         agentID,
		ObservedAt:            now,
		CurrentPersonaExcerpt: composePersonaExcerpt(coreContext),
		SystemRulesExcerpt:    strings.TrimSpace(coreContext.SystemRulesSummary),
	}
	extractor := s.PersonaExtractor

	s.personaExtractWG.Add(1)
	go func() {
		defer s.personaExtractWG.Done()
		ctx, cancel := context.WithTimeout(context.Background(), timeout)
		defer cancel()
		result, err := extractor.Extract(ctx, input)
		if err != nil {
			s.logPersonaExtractError("extract", sessionID, err)
			return
		}
		// Parser warnings explain the most common "zero candidates"
		// outcome that is not a transport error: paraphrased
		// evidence_quote, low confidence, empty evidence, conflict
		// vs. current persona, etc. The parser returns these in
		// result.Warnings with err==nil so the extract-stage branch
		// above does not see them. Surface them as stage=parse_warning
		// only when no candidate landed, so the operator's tail of
		// the log focuses on the diagnostic cases and a successful
		// extraction does not flood the log with informational notes
		// the parser also emits for partial discards.
		if len(result.Candidates) == 0 {
			for _, warning := range result.Warnings {
				s.logPersonaExtractError("parse_warning", sessionID, errors.New(warning))
			}
		}
		for _, candidate := range result.Candidates {
			record := persona.PersonaCandidateRecord{
				ID:        persona.NewCandidateID(now),
				State:     persona.PersonaCandidateOpen,
				DedupKey:  persona.DedupKey(candidate),
				Candidate: candidate,
				CreatedAt: now,
				UpdatedAt: now,
			}
			if _, _, storeErr := runtime.RecordPersonaCandidate(record); storeErr != nil {
				s.logPersonaExtractError("store", sessionID, storeErr)
			}
		}
	}()
}

// logPersonaExtractError emits a single tab-delimited line to
// PersonaExtractLogger describing a failure inside the fire-and-forget
// extraction goroutine. The format is documented on the Session field:
// timestamp\tstage=...\tsession=...\terror="..."\n. Nil logger or nil
// session is a silent no-op so legacy P4 callers / unit tests that do
// not wire a logger keep their existing behavior.
func (s *Session) logPersonaExtractError(stage, sessionID string, err error) {
	if s == nil || s.PersonaExtractLogger == nil || err == nil {
		return
	}
	msg := strings.ReplaceAll(err.Error(), "\n", " ")
	fmt.Fprintf(
		s.PersonaExtractLogger,
		"%s\tstage=%s\tsession=%s\terror=%q\n",
		time.Now().UTC().Format(time.RFC3339Nano),
		stage,
		sessionID,
		msg,
	)
}

// DrainPersonaExtractions blocks up to timeout for in-flight
// fire-and-forget persona extractions to finish. Returns true on
// clean drain, false when the timeout elapsed and goroutines were
// abandoned. Abandoned goroutines may still complete after this
// method returns; they will quietly write to the store if the runtime
// is still usable, or no-op if the runtime has been closed.
//
// Console / TUI shells call this immediately before process exit so
// that the user's most recent turn has a fair chance to land in the
// review queue. The timeout should be short (a couple of seconds) so
// shell exit is never noticeably delayed by a stuck LLM.
func (s *Session) DrainPersonaExtractions(timeout time.Duration) bool {
	if s == nil {
		return true
	}
	done := make(chan struct{})
	go func() {
		s.personaExtractWG.Wait()
		close(done)
	}()
	select {
	case <-done:
		return true
	case <-time.After(timeout):
		return false
	}
}

// persistResponseUsage translates per-loop-step ModelCallUsage entries
// emitted by the operator agent into model.UsageRecord values and
// delegates to runtime.RecordUsage. AgentID falls back to the session
// default ("codex" unless overridden); SessionID is sourced from the
// active transcript recorder when one is attached. An empty usage slice
// is a no-op; persistence errors propagate so that store-side failures
// are not silently dropped.
func (s *Session) persistResponseUsage(runtime Runtime, usage []operatoragent.ModelCallUsage) error {
	if len(usage) == 0 {
		return nil
	}
	sessionID := ""
	if s.Recorder != nil {
		sessionID = s.Recorder.SessionID()
		// Record usage to the transcript before persisting to the store
		// so that even if the store write fails the JSONL retains
		// evidence of the model call. Transcript errors are
		// non-blocking by the same convention as recordToolTrace.
		_ = s.Recorder.RecordModelUsage(usage)
	}
	records := make([]model.UsageRecord, 0, len(usage))
	for _, u := range usage {
		records = append(records, model.UsageRecord{
			Provider:         u.Provider,
			Model:            u.Model,
			AgentID:          s.DefaultAgentID,
			SessionID:        sessionID,
			PromptTokens:     u.PromptTokens,
			CompletionTokens: u.CompletionTokens,
			RecordedAt:       u.StartedAt,
			Purpose:          model.UsagePurposeChat,
		})
	}
	return runtime.RecordUsage(records)
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
