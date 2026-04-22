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

type Context struct {
	CurrentDraftID string
	DefaultAgentID string
	Now            time.Time
}

type Agent interface {
	Decide(input string, ctx Context) (Decision, error)
}
