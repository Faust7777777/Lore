package tui

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
)

func renderInteractiveWorkbenchLayout(model interactiveWorkbenchModel) string {
	leftWidth := maxInt(40, (model.width*2)/3)
	rightWidth := maxInt(28, model.width-leftWidth-1)
	contentHeight := maxInt(12, model.height-8)
	rightTopHeight := maxInt(8, (contentHeight*2)/3)
	rightBottomHeight := maxInt(5, contentHeight-rightTopHeight-1)

	leftPane := paneStyle(model.focus == focusConversation).Width(leftWidth).Height(contentHeight).Render(
		renderPaneTitle("Conversation Lane", model.focus == focusConversation, model.running, model.pendingLine, model.spin.View()) + "\n" + model.chatViewport.View(),
	)
	statusPane := paneStyle(model.focus == focusStatus).Width(rightWidth).Height(rightTopHeight).Render(
		renderPaneTitle("Context & Status", model.focus == focusStatus, false, "", "") + "\n" + model.statusViewport.View(),
	)
	approvalPane := paneStyle(false).Width(rightWidth).Height(rightBottomHeight).Render(
		renderPaneTitle("Pending Approvals", false, false, "", "") + "\n" + renderApprovalPlaceholder(),
	)
	rightColumn := lipgloss.JoinVertical(lipgloss.Left, statusPane, approvalPane)

	inputPane := inputPaneStyle(model.focus == focusInput).Width(model.width).Render(
		renderInputHeader(model.focus == focusInput, model.running) + "\n" + model.input.View(),
	)

	return lipgloss.JoinVertical(lipgloss.Left,
		lipgloss.JoinHorizontal(lipgloss.Top, leftPane, rightColumn),
		inputPane,
	)
}

func renderInteractiveConversation(viewModel WorkbenchViewModel, lastOutput string, running bool, pendingLine string) string {
	var builder strings.Builder

	turns := viewModel.Conversation.Turns
	if len(turns) > 8 {
		turns = turns[len(turns)-8:]
	}

	if len(turns) == 0 {
		builder.WriteString("No conversation yet.\n")
	} else {
		for _, turn := range turns {
			role := strings.ToUpper(strings.TrimSpace(turn.Role))
			if role == "" {
				role = "UNKNOWN"
			}
			builder.WriteString(role + "\n")
			builder.WriteString(indentBlock(strings.TrimSpace(turn.Content), "  "))
			builder.WriteString("\n\n")
		}
	}

	if running && strings.TrimSpace(pendingLine) != "" {
		builder.WriteString("RUNNING\n")
		builder.WriteString(indentBlock(strings.TrimSpace(pendingLine), "  "))
		builder.WriteString("\n\n")
	}

	builder.WriteString("Latest Output\n")
	builder.WriteString("-------------\n")
	if strings.TrimSpace(lastOutput) == "" {
		builder.WriteString("No active output.\n")
	} else {
		builder.WriteString(indentBlock(strings.TrimSpace(excerpt(lastOutput, 2200)), "  "))
		builder.WriteString("\n")
	}

	return builder.String()
}

func renderInteractiveStatus(viewModel WorkbenchViewModel) string {
	var builder strings.Builder

	writeField(&builder, "Profile", viewModel.Snapshot.Profile)
	writeField(&builder, "Ready", yesNo(viewModel.Snapshot.Ready))
	writeField(&builder, "Health", viewModel.Snapshot.HealthStatus)
	writeField(&builder, "Message", oneLine(viewModel.Snapshot.HealthMessage, 72))
	writeField(&builder, "Drafts", viewModel.Snapshot.DraftSummary())
	writeField(&builder, "Agent", viewModel.Snapshot.AgentID)
	writeField(&builder, "Day", viewModel.Snapshot.Day.Format("2006-01-02"))

	builder.WriteString("\nCore Docs\n")
	builder.WriteString("---------\n")
	if len(viewModel.ManagedCore) == 0 {
		builder.WriteString("No managed docs.\n")
	} else {
		for _, doc := range viewModel.ManagedCore {
			state := "missing"
			if doc.Exists {
				state = "ready"
			}
			builder.WriteString("- " + doc.Name + " [" + state + "]\n")
		}
	}

	builder.WriteString("\nPending Drafts\n")
	builder.WriteString("--------------\n")
	if len(viewModel.PendingDrafts) == 0 {
		builder.WriteString("No pending drafts.\n")
	} else {
		limit := minInt(4, len(viewModel.PendingDrafts))
		for _, draft := range viewModel.PendingDrafts[:limit] {
			builder.WriteString("- " + oneLine(draft.Title, 48) + "\n")
		}
	}

	builder.WriteString("\nProcess Sink\n")
	builder.WriteString("------------\n")
	if len(viewModel.ProcessSink.Checkpoints) == 0 {
		builder.WriteString("No checkpoints.\n")
	} else {
		latest := viewModel.ProcessSink.Checkpoints[len(viewModel.ProcessSink.Checkpoints)-1]
		builder.WriteString("Latest: " + latest.Window.WindowStart.Format("15:04") + "-" + latest.Window.WindowEnd.Format("15:04") + "\n")
		builder.WriteString(oneLine(latest.Title, 60) + "\n")
	}

	builder.WriteString("\nTool Trace\n")
	builder.WriteString("----------\n")
	if len(viewModel.ToolTrace) == 0 {
		builder.WriteString("No tool calls.\n")
	} else {
		limit := minInt(5, len(viewModel.ToolTrace))
		for idx, item := range viewModel.ToolTrace[:limit] {
			builder.WriteString(oneLine(strings.ToUpper(item.Status), 8) + " ")
			builder.WriteString(oneLine(item.Name, 28))
			if idx < limit-1 {
				builder.WriteString("\n")
			}
		}
		builder.WriteString("\n")
	}

	return builder.String()
}

func renderApprovalPlaceholder() string {
	return "No pending actions.\n\nRuntime-backed approval queue is not wired into the interactive shell yet.\nManaged core writes still go through draft -> review -> apply."
}

func renderPaneTitle(title string, focused bool, running bool, pendingLine string, spin string) string {
	status := ""
	if running && strings.TrimSpace(pendingLine) != "" {
		status = "  " + strings.TrimSpace(spin) + " running"
	}
	if focused {
		return focusedTitleStyle.Render(title) + titleStatusStyle.Render(status)
	}
	return titleStyle.Render(title) + titleStatusStyle.Render(status)
}

func renderInputHeader(focused bool, running bool) string {
	label := "Input"
	if focused {
		label = "Input (focused)"
	}
	if running {
		return focusedTitleStyle.Render(label) + titleStatusStyle.Render("  agent running...")
	}
	return focusedTitleStyle.Render(label) + titleStatusStyle.Render("  Enter submit | Tab switch pane | Ctrl+C quit")
}
