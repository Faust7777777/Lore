package operatoragent

import "time"

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

type Context struct {
	CurrentDraftID string
	DefaultAgentID string
	Now            time.Time
	History        []ConversationTurn
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

type Response struct {
	Final    string
	Decision *Decision
	Trace    []ToolCallTrace
}

type LoopAgent interface {
	Respond(input string, ctx Context, runtime ToolRuntime) (Response, error)
}
