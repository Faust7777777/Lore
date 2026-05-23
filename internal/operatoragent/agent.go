package operatoragent

import (
	"context"
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

type ContextToolRuntime interface {
	ToolRuntime
	CallToolContext(ctx context.Context, name string, arguments map[string]any) (ToolResult, error)
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
//     decision, or shell-confirm short circuit).
//   - TurnStopMaxSteps   - loop hit maxLoopSteps without terminating.
//   - TurnStopModelError - model response could not be parsed,
//     validated, or completed (includes
//     ChatCompletion API failures).
//   - TurnStopToolError  - tool execution failed before Lore could
//     safely reinject the result into the model
//     context, including turn cancellation during
//     a tool call.
type TurnStopReason string

const (
	TurnStopFinal      TurnStopReason = "final"
	TurnStopMaxSteps   TurnStopReason = "max_steps"
	TurnStopToolError  TurnStopReason = "tool_error"
	TurnStopModelError TurnStopReason = "model_error"
)

// TurnStep is a structured per-step record of one tool call inside a
// single Respond turn. Unlike ToolCallTrace (which is wire-shaped for
// transcript replay and survives the older agent loop), TurnStep is
// shaped for direct consumption by progress UIs: it preserves
// arguments and adds an ObservationExcerpt -- a safely truncated
// preview of what the tool returned -- so a renderer can show
// "found path X, read first 1KB, then generated final" without
// having to re-fetch tool outputs.
//
// Steps are emitted only for actually-executed tool calls; pre-tool
// validation errors (multiple ToolCalls, empty name) terminate before
// a step is appended.
type TurnStep struct {
	// Index is 1-based for human-facing display ordering.
	Index int
	// Tool is the tool name as dispatched (e.g. "vault_resolve").
	Tool string
	// Arguments is a defensive copy of the arguments passed to the
	// tool, suitable for rendering and JSON serialization.
	Arguments map[string]any
	// Status mirrors ToolCallTrace.Status: "ok", "error", or
	// "pending" (the shell_exec confirmation path).
	Status string
	// ObservationExcerpt is a safe, length-bounded preview of the
	// tool's textual return value. Binary content is replaced with a
	// short marker; oversized content is rune-safely truncated and
	// annotated with the dropped byte count.
	ObservationExcerpt string
	// Error captures the tool's error message when Status == "error",
	// empty otherwise. Mirrors ToolCallTrace.Error.
	Error string
}

type Response struct {
	Final      string
	Decision   *Decision
	Trace      []ToolCallTrace
	Usage      []ModelCallUsage
	StopReason TurnStopReason
	StepCount  int
	// Steps is the structured per-step record of every tool call
	// executed during this turn. Index is 1-based. Empty for turns
	// that finalize without invoking any tool.
	Steps []TurnStep
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

type ContextLoopAgent interface {
	LoopAgent
	RespondContext(ctx context.Context, input string, agentCtx Context, runtime ToolRuntime) (Response, error)
}
