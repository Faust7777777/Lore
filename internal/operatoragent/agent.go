package operatoragent

import (
	"time"

	"obsidian-harness/internal/model"
)

type Action string

const (
	ActionUnknown              Action = "unknown"
	ActionHelp                 Action = "help"
	ActionShowStatus           Action = "show_status"
	ActionListDrafts           Action = "list_drafts"
	ActionReviewDraft          Action = "review_draft"
	ActionApproveDraft         Action = "approve_draft"
	ActionRejectDraft          Action = "reject_draft"
	ActionRequestDraftRevision Action = "request_draft_revision"
	ActionApplyDraft           Action = "apply_draft"
	ActionShowProcessSinkDay   Action = "show_process_sink_day"
)

type Decision struct {
	Action          Action
	DraftID         string
	UseFocusedDraft bool
	PendingOnly     bool
	AgentID         string
	Day             time.Time
}

type ConversationTurn struct {
	Role    string
	Content string
}

type WorkingSetItem struct {
	Kind   string
	Path   string
	Source string
}

type Context struct {
	CurrentDraftID string
	DefaultAgentID string
	Now            time.Time
	History        []ConversationTurn
	WorkingSet     []WorkingSetItem
	CoreContext    model.CoreContext
}

type Agent interface {
	Decide(input string, ctx Context) (Decision, error)
}

type ToolDefinition struct {
	Name        string
	Description string
	Arguments   string
}

type ToolResult struct {
	Content string
}

type ToolCallTrace struct {
	Name      string
	Arguments map[string]any
	Status    string
	Error     string
}

type ToolRuntime interface {
	DescribeTools(ctx Context) []ToolDefinition
	CallTool(name string, arguments map[string]any) (ToolResult, error)
}

type ModelCallUsage struct {
	Provider         string    `json:"provider,omitempty"`
	Model            string    `json:"model,omitempty"`
	PromptTokens     int       `json:"prompt_tokens"`
	CompletionTokens int       `json:"completion_tokens"`
	StartedAt        time.Time `json:"started_at"`
}

// TurnStopReason names why an operator-agent turn terminated. Values
// are stable across releases; consumers (console, sessionlog, future
// TUI progress UI) compare against these constants.
//
// Today's coverage:
//   - TurnStopFinal      - terminal success (final envelope, legacy
//                          decision, or shell-confirm short circuit).
//   - TurnStopMaxSteps   - loop hit maxLoopSteps without terminating.
//   - TurnStopModelError - model response could not be parsed,
//                          validated, or completed (includes
//                          ChatCompletion API failures).
//   - TurnStopToolError  - reserved for B5; not emitted by Respond
//                          yet. Defined now so the enum stays stable
//                          when bounded tool-failure recovery lands.
type TurnStopReason string

const (
	TurnStopFinal      TurnStopReason = "final"
	TurnStopMaxSteps   TurnStopReason = "max_steps"
	TurnStopToolError  TurnStopReason = "tool_error"
	TurnStopModelError TurnStopReason = "model_error"
)

type Response struct {
	Final      string
	Decision   *Decision
	Trace      []ToolCallTrace
	Usage      []ModelCallUsage
	StopReason TurnStopReason
	StepCount  int
}

// UsageError wraps an error returned by Respond after one or more
// successful ChatCompletion calls, carrying the per-call usage so the
// caller can still bill cost for billed-but-failed turns. The wrapped
// error is preserved verbatim via Unwrap; Error() defers to it without
// adding a prefix so user-visible messages stay unchanged from
// pre-existing failure modes.
//
// Callers should test for usage carry-over with errors.As:
//
//	var usageErr *UsageError
//	if errors.As(err, &usageErr) {
//	    runtime.RecordUsage(translate(usageErr.Usage))
//	}
type UsageError struct {
	Err   error
	Usage []ModelCallUsage
}

func (e *UsageError) Error() string {
	if e == nil || e.Err == nil {
		return ""
	}
	return e.Err.Error()
}

func (e *UsageError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Err
}

type LoopAgent interface {
	Respond(input string, ctx Context, runtime ToolRuntime) (Response, error)
}
