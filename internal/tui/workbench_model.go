package tui

import (
	"fmt"
	"strings"
	"time"

	"obsidian-harness/internal/app"
	"obsidian-harness/internal/model"
	"obsidian-harness/internal/operatoragent"
)

type WorkbenchViewModel struct {
	Header        WorkbenchHeader
	Snapshot      WorkbenchSnapshot
	ManagedCore   []model.ManagedCoreStatus
	PendingDrafts []model.Draft
	FocusedReview *app.DraftReview
	ProcessSink   app.ProcessSinkDayView
	Findings      []model.Finding
	ToolTrace     []operatoragent.ToolCallTrace
	TurnSteps     []TurnStep
	Conversation  WorkbenchConversation
	QuickActions  []string
	Controls      []string
}

// TurnStep represents one tool call step within the last agent turn,
// built from ToolCallTrace for timeline rendering in the TUI.
type TurnStep struct {
	Index              int
	Tool               string
	Status             string
	ObservationExcerpt string
	Error              string
	Arguments          map[string]any
}

type WorkbenchHeader struct {
	Title    string
	Subtitle string
	Banner   string
}

type WorkbenchSnapshot struct {
	Version         string
	Ready           bool
	Profile         string
	GitEnabled      bool
	ShellEnabled    bool
	PendingDrafts   int
	TotalDrafts     int
	Checkpoints     int
	WorkDir         string
	VaultRoot       string
	HealthStatus    string
	HealthMessage   string
	AgentID         string
	Day             time.Time
	DailyReportPath string
	SessionID       string
	TranscriptPath  string
}

type WorkbenchConversation struct {
	Turns      []operatoragent.ConversationTurn
	LastOutput string
}

func NewWorkbenchViewModel(version string, managed model.ManagedStatusView, drafts []model.Draft, processSink app.ProcessSinkDayView, focusedReview *app.DraftReview, findings []model.Finding, toolTrace []operatoragent.ToolCallTrace, history []operatoragent.ConversationTurn, localExec bool, shellEnabled bool, lastOutput string, sessionID string, transcriptPath string) WorkbenchViewModel {
	pendingDrafts := filterDraftsByStates(drafts, model.DraftPendingReview, model.DraftApproved)
	dailyReportPath := "missing"
	if processSink.Report != nil {
		dailyReportPath = processSink.Report.Path
	}

	return WorkbenchViewModel{
		Header: WorkbenchHeader{
			Title:    "Lore Workbench",
			Subtitle: "Natural-language workbench for managed knowledge ops, draft review, and process-sink monitoring.",
			Banner:   modeBanner(localExec, shellEnabled, processSink.AgentID, processSink.Day),
		},
		Snapshot: WorkbenchSnapshot{
			Version:         version,
			Ready:           managed.Ready,
			Profile:         profileLabel(localExec),
			GitEnabled:      localExec,
			ShellEnabled:    shellEnabled,
			PendingDrafts:   len(pendingDrafts),
			TotalDrafts:     len(drafts),
			Checkpoints:     len(processSink.Checkpoints),
			WorkDir:         managed.WorkDir,
			VaultRoot:       managed.VaultRoot,
			HealthStatus:    string(managed.Health.Outcome.Status),
			HealthMessage:   managed.Health.Message,
			AgentID:         processSink.AgentID,
			Day:             processSink.Day,
			DailyReportPath: dailyReportPath,
			SessionID:       strings.TrimSpace(sessionID),
			TranscriptPath:  strings.TrimSpace(transcriptPath),
		},
		ManagedCore:   append([]model.ManagedCoreStatus(nil), managed.CoreDocs...),
		PendingDrafts: append([]model.Draft(nil), pendingDrafts...),
		FocusedReview: focusedReview,
		ProcessSink:   processSink,
		Findings:      append([]model.Finding(nil), findings...),
		ToolTrace:     append([]operatoragent.ToolCallTrace(nil), toolTrace...),
		TurnSteps:     buildTurnSteps(toolTrace),
		Conversation: WorkbenchConversation{
			Turns:      append([]operatoragent.ConversationTurn(nil), history...),
			LastOutput: strings.TrimSpace(lastOutput),
		},
		QuickActions: quickActions(localExec, shellEnabled),
		Controls: []string{
			"Type a natural language request.",
			"Special commands: /refresh, /status, /drafts, /quit",
		},
	}
}

func (s WorkbenchSnapshot) DraftSummary() string {
	return fmt.Sprintf("%d reviewable / %d total", s.PendingDrafts, s.TotalDrafts)
}

func buildTurnSteps(trace []operatoragent.ToolCallTrace) []TurnStep {
	if len(trace) == 0 {
		return nil
	}
	steps := make([]TurnStep, len(trace))
	for i, tc := range trace {
		steps[i] = TurnStep{
			Index:     i + 1,
			Tool:      tc.Name,
			Status:    tc.Status,
			Error:     tc.Error,
			Arguments: tc.Arguments,
			// ObservationExcerpt left empty until B-line populates ToolCallTrace with tool result
		}
	}
	return steps
}

// stepArgSummary returns a human-readable one-line summary of key arguments
// for the most common tool types. Falls back to empty string for unknown tools.
func stepArgSummary(tool string, args map[string]any) string {
	if args == nil {
		return ""
	}
	switch tool {
	case "vault_resolve":
		if q, ok := args["query"].(string); ok {
			return "query=" + q
		}
	case "vault_read":
		if p, ok := args["path"].(string); ok {
			return "path=" + p
		}
	case "vault_search_text":
		if q, ok := args["query"].(string); ok {
			return "query=" + q
		}
	case "vault_write_note":
		if p, ok := args["path"].(string); ok {
			return "path=" + p
		}
	case "draft_approve", "draft_reject", "draft_apply":
		if id, ok := args["draft_id"].(string); ok {
			return "draft=" + id
		}
	case "managed_status":
		return ""
	case "draft_review":
		if id, ok := args["draft_id"].(string); ok {
			return "draft=" + id
		}
	case "process_sink_day":
		if a, ok := args["agent_id"].(string); ok {
			return "agent=" + a
		}
	case "vault_backlinks":
		if p, ok := args["path"].(string); ok {
			return "path=" + p
		}
	}
	// Generic: show first string argument
	for k, v := range args {
		if s, ok := v.(string); ok && s != "" {
			return k + "=" + s
		}
	}
	return ""
}
