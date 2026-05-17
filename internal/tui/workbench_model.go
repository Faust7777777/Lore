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
	ToolTrace     []operatoragent.ToolCallTrace
	Conversation  WorkbenchConversation
	QuickActions  []string
	Controls      []string
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

func NewWorkbenchViewModel(version string, managed model.ManagedStatusView, drafts []model.Draft, processSink app.ProcessSinkDayView, focusedReview *app.DraftReview, toolTrace []operatoragent.ToolCallTrace, history []operatoragent.ConversationTurn, localExec bool, shellEnabled bool, lastOutput string, sessionID string, transcriptPath string) WorkbenchViewModel {
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
		ToolTrace:     append([]operatoragent.ToolCallTrace(nil), toolTrace...),
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
