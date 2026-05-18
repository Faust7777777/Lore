package tui

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"obsidian-harness/internal/app"
	"obsidian-harness/internal/model"
	"obsidian-harness/internal/operatoragent"
)

func RenderWorkbench(version string, managed model.ManagedStatusView, drafts []model.Draft, processSink app.ProcessSinkDayView, focusedReview *app.DraftReview, toolTrace []operatoragent.ToolCallTrace, history []operatoragent.ConversationTurn, localExec bool, shellEnabled bool, lastOutput string) string {
	viewModel := NewWorkbenchViewModel(version, managed, drafts, processSink, focusedReview, nil, toolTrace, history, localExec, shellEnabled, lastOutput, "", "")
	return RenderWorkbenchViewModel(viewModel)
}

func RenderWorkbenchViewModel(viewModel WorkbenchViewModel) string {
	var builder strings.Builder

	renderWorkbenchHeader(&builder, viewModel.Header)
	renderRuntimeSnapshotSection(&builder, viewModel.Snapshot)
	renderManagedCoreSection(&builder, viewModel.ManagedCore)
	renderPendingDraftsSection(&builder, viewModel.PendingDrafts)
	renderFocusedDraftSection(&builder, viewModel.FocusedReview)
	renderProcessSinkSection(&builder, viewModel.ProcessSink)
	renderToolTraceSection(&builder, viewModel.ToolTrace)
	renderConversationSection(&builder, viewModel.Conversation)
	renderLatestOutputSection(&builder, viewModel.Conversation.LastOutput)
	renderQuickActionsSection(&builder, viewModel.QuickActions)
	renderControlsSection(&builder, viewModel.Controls)

	return builder.String()
}

func renderWorkbenchHeader(builder *strings.Builder, header WorkbenchHeader) {
	builder.WriteString(header.Title + "\n")
	builder.WriteString(strings.Repeat("=", len(header.Title)) + "\n")
	builder.WriteString(header.Subtitle + "\n")
	builder.WriteString(header.Banner)
}

func renderRuntimeSnapshotSection(builder *strings.Builder, snapshot WorkbenchSnapshot) {
	builder.WriteString("\nRuntime Snapshot\n")
	builder.WriteString("----------------\n")
	writeField(builder, "Version", snapshot.Version)
	writeField(builder, "Ready", yesNo(snapshot.Ready))
	writeField(builder, "Profile", snapshot.Profile)
	writeField(builder, "Git", enabledDisabled(snapshot.GitEnabled))
	writeField(builder, "Shell", enabledDisabled(snapshot.ShellEnabled))
	writeField(builder, "Drafts", snapshot.DraftSummary())
	writeField(builder, "Checkpoints", fmt.Sprintf("%d", snapshot.Checkpoints))
	writeField(builder, "WorkDir", snapshot.WorkDir)
	writeField(builder, "Vault", snapshot.VaultRoot)
	writeField(builder, "Health", snapshot.HealthStatus)
	writeField(builder, "Message", snapshot.HealthMessage)
	writeField(builder, "Agent", snapshot.AgentID)
	writeField(builder, "Day", snapshot.Day.Format("2006-01-02"))
	writeField(builder, "Daily Report", snapshot.DailyReportPath)
}

func renderManagedCoreSection(builder *strings.Builder, coreDocs []model.ManagedCoreStatus) {
	builder.WriteString("\nManaged Core\n")
	builder.WriteString("------------\n")
	fmt.Fprintf(builder, "%-10s %-8s %s\n", "DOC", "STATE", "PATH")
	for _, doc := range coreDocs {
		state := "missing"
		if doc.Exists {
			state = "ready"
		}
		fmt.Fprintf(builder, "%-10s %-8s %s\n", doc.Name, state, oneLine(doc.Path, 72))
	}
}

func renderPendingDraftsSection(builder *strings.Builder, drafts []model.Draft) {
	builder.WriteString("\nPending Drafts\n")
	builder.WriteString("--------------\n")
	if len(drafts) == 0 {
		builder.WriteString("No pending drafts.\n")
		return
	}

	fmt.Fprintf(builder, "%-22s %-18s %-24s %s\n", "ID", "KIND", "TARGET", "TITLE")
	for _, draft := range drafts {
		fmt.Fprintf(
			builder,
			"%-22s %-18s %-24s %s\n",
			shortID(draft.ID, 22),
			string(draft.Kind),
			shortID(draft.Target.Path, 24),
			oneLine(draft.Title, 56),
		)
	}
}

func renderFocusedDraftSection(builder *strings.Builder, focusedReview *app.DraftReview) {
	builder.WriteString("\nFocused Draft\n")
	builder.WriteString("-------------\n")
	if focusedReview == nil {
		builder.WriteString("No focused draft.\n")
		builder.WriteString("Hint: ask Lore to review the pending draft.\n")
		return
	}

	writeField(builder, "ID", focusedReview.Draft.ID)
	writeField(builder, "State", string(focusedReview.Draft.State))
	writeField(builder, "Target", focusedReview.Draft.Target.Path)
	writeField(builder, "Base Match", yesNo(focusedReview.BaseVersionMatches))
	builder.WriteString("\n")
	builder.WriteString("Summary\n")
	builder.WriteString("~~~~~~~\n")
	builder.WriteString(strings.TrimSpace(excerpt(focusedReview.Draft.Summary, 240)))
	builder.WriteString("\n\n")
	builder.WriteString("Patch Preview\n")
	builder.WriteString("~~~~~~~~~~~~~\n")
	builder.WriteString(strings.TrimSpace(excerpt(focusedReview.Draft.ProposedContent, 320)))
	builder.WriteString("\n")
}

func renderProcessSinkSection(builder *strings.Builder, processSink app.ProcessSinkDayView) {
	builder.WriteString("\nProcess Sink\n")
	builder.WriteString("------------\n")
	if len(processSink.Checkpoints) == 0 {
		builder.WriteString("No checkpoints.\n")
		return
	}

	fmt.Fprintf(builder, "%-13s %-14s %-12s %s\n", "WINDOW", "STATE", "SESSION", "TITLE")
	for _, checkpoint := range processSink.Checkpoints {
		fmt.Fprintf(
			builder,
			"%-13s %-14s %-12s %s\n",
			checkpoint.Window.WindowStart.Format("15:04")+"-"+checkpoint.Window.WindowEnd.Format("15:04"),
			string(checkpoint.State),
			oneLine(checkpoint.Window.SessionID, 12),
			oneLine(checkpoint.Title, 60),
		)
	}
}

func renderToolTraceSection(builder *strings.Builder, toolTrace []operatoragent.ToolCallTrace) {
	builder.WriteString("\nTool Trace\n")
	builder.WriteString("----------\n")
	if len(toolTrace) == 0 {
		builder.WriteString("No tool calls in the last turn.\n")
		return
	}

	fmt.Fprintf(builder, "%-4s %-18s %-8s %-34s %s\n", "STEP", "TOOL", "STATUS", "ARGUMENTS", "ERROR")
	for idx, item := range toolTrace {
		fmt.Fprintf(
			builder,
			"%-4d %-18s %-8s %-34s %s\n",
			idx+1,
			oneLine(item.Name, 18),
			oneLine(item.Status, 8),
			oneLine(formatToolArguments(item.Arguments), 34),
			oneLine(item.Error, 64),
		)
	}
}

func renderConversationSection(builder *strings.Builder, conversation WorkbenchConversation) {
	builder.WriteString("\nConversation Lane\n")
	builder.WriteString("-----------------\n")

	lines := conversationLines(conversation)
	if len(lines) == 0 {
		builder.WriteString("No conversation yet.\n")
		return
	}
	for _, line := range lines {
		builder.WriteString(line + "\n")
	}
}

func renderLatestOutputSection(builder *strings.Builder, lastOutput string) {
	builder.WriteString("\nLatest Output\n")
	builder.WriteString("-------------\n")
	if strings.TrimSpace(lastOutput) == "" {
		builder.WriteString("No active output.\n")
		return
	}

	builder.WriteString(indentBlock(strings.TrimSpace(excerpt(lastOutput, 1200)), "  "))
	builder.WriteString("\n")
}

func renderQuickActionsSection(builder *strings.Builder, actions []string) {
	builder.WriteString("\nQuick Actions\n")
	builder.WriteString("-------------\n")
	for _, action := range actions {
		builder.WriteString("  " + action + "\n")
	}
}

func renderControlsSection(builder *strings.Builder, controls []string) {
	builder.WriteString("\nControls\n")
	builder.WriteString("--------\n")
	for _, control := range controls {
		builder.WriteString(control + "\n")
	}
}

func filterDraftsByState(drafts []model.Draft, state model.DraftState) []model.Draft {
	out := make([]model.Draft, 0, len(drafts))
	for _, draft := range drafts {
		if draft.State == state {
			out = append(out, draft)
		}
	}
	return out
}

func filterDraftsByStates(drafts []model.Draft, states ...model.DraftState) []model.Draft {
	allowed := make(map[model.DraftState]bool, len(states))
	for _, s := range states {
		allowed[s] = true
	}
	out := make([]model.Draft, 0, len(drafts))
	for _, draft := range drafts {
		if allowed[draft.State] {
			out = append(out, draft)
		}
	}
	return out
}

func indentBlock(value string, prefix string) string {
	if strings.TrimSpace(value) == "" {
		return ""
	}
	lines := strings.Split(strings.ReplaceAll(value, "\r\n", "\n"), "\n")
	for i, line := range lines {
		lines[i] = prefix + line
	}
	return strings.Join(lines, "\n")
}

func conversationLines(conversation WorkbenchConversation) []string {
	const maxTurns = 6

	turns := append([]operatoragent.ConversationTurn(nil), conversation.Turns...)
	if len(turns) > maxTurns {
		turns = turns[len(turns)-maxTurns:]
	}

	lines := make([]string, 0, len(turns)+4)
	for _, turn := range turns {
		role := strings.ToUpper(strings.TrimSpace(turn.Role))
		if role == "" {
			role = "UNKNOWN"
		}
		content := strings.TrimSpace(turn.Content)
		if content == "" {
			continue
		}
		lines = append(lines, fmt.Sprintf("%-10s %s", role, oneLine(content, 120)))
	}

	lastOutput := strings.TrimSpace(conversation.LastOutput)
	if lastOutput != "" {
		lastVisible := ""
		if len(lines) > 0 {
			lastVisible = lines[len(lines)-1]
		}
		renderedOutput := fmt.Sprintf("%-10s %s", "LATEST", oneLine(lastOutput, 120))
		if lastVisible != renderedOutput {
			lines = append(lines, renderedOutput)
		}
	}

	return lines
}

func profileLabel(localExec bool) string {
	if localExec {
		return "local-exec"
	}
	return "lore"
}

func enabledDisabled(value bool) string {
	if value {
		return "enabled"
	}
	return "disabled"
}

func formatToolArguments(arguments map[string]any) string {
	if len(arguments) == 0 {
		return "{}"
	}
	keys := make([]string, 0, len(arguments))
	for key := range arguments {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	parts := make([]string, 0, len(arguments))
	for _, key := range keys {
		value := arguments[key]
		parts = append(parts, fmt.Sprintf("%s=%v", key, value))
	}
	return strings.Join(parts, ", ")
}

func modeBanner(localExec bool, shellEnabled bool, agentID string, day time.Time) string {
	parts := []string{
		"chat:lore",
		"profile:" + profileLabel(localExec),
		"git:" + enabledDisabled(localExec),
		"shell:" + enabledDisabled(shellEnabled),
	}
	if strings.TrimSpace(agentID) != "" {
		parts = append(parts, "agent:"+strings.TrimSpace(agentID))
	}
	if !day.IsZero() {
		parts = append(parts, "day:"+day.Format("2006-01-02"))
	}
	return "\n[" + strings.Join(parts, "] [") + "]\n"
}

func quickActions(localExec bool, shellEnabled bool) []string {
	actions := []string{
		"show current status",
		"list pending drafts",
		"review the pending draft",
		"approve it",
		"apply it",
		"show codex daily report today",
	}
	if localExec {
		actions = append(actions,
			"show git status for the local repo",
			"summarize the staged git diff",
		)
	}
	if shellEnabled {
		actions = append(actions, "run go test ./... in the local repo")
	}
	return actions
}
