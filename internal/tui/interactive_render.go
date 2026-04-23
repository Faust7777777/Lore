package tui

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
)

func renderInteractiveWorkbenchLayout(model interactiveWorkbenchModel) string {
	w := model.width
	narrow := w < 80

	var leftWidth, rightWidth int
	if narrow {
		leftWidth = maxInt(30, (w*3)/4)
		rightWidth = maxInt(20, w-leftWidth-1)
	} else {
		leftWidth = maxInt(40, (w*2)/3)
		rightWidth = maxInt(28, w-leftWidth-1)
	}
	contentHeight := maxInt(12, model.height-8)
	rightTopHeight := maxInt(8, (contentHeight*2)/3)
	rightBottomHeight := maxInt(5, contentHeight-rightTopHeight-1)

	leftPane := paneStyle(model.focus == focusConversation).Width(leftWidth).Height(contentHeight).Render(
		renderPaneTitle("Conversation Lane", model.focus == focusConversation, model.running, model.pendingLine, model.spin.View()) + "\n" + model.chatViewport.View(),
	)
	statusPane := paneStyle(model.focus == focusStatus).Width(rightWidth).Height(rightTopHeight).Render(
		renderPaneTitle("Context & Status", model.focus == focusStatus, false, "", "") + "\n" + model.statusViewport.View(),
	)

	var approvalTitle string
	if narrow {
		approvalTitle = "Approvals"
	} else {
		approvalTitle = "Pending Approvals"
	}
	approvalPane := paneStyle(false).Width(rightWidth).Height(rightBottomHeight).Render(
		renderPaneTitle(approvalTitle, false, false, "", "") + "\n" + renderApprovalPlaceholder(),
	)
	rightColumn := lipgloss.JoinVertical(lipgloss.Left, statusPane, approvalPane)

	inputPane := inputPaneStyle(model.focus == focusInput).Width(w).Render(
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
		builder.WriteString(styleMutedText.Render("Ready. Ask Lore about your vault, drafts, or process sink."))
		builder.WriteString("\n")
	} else {
		for _, turn := range turns {
			role := strings.ToLower(strings.TrimSpace(turn.Role))
			content := strings.TrimSpace(turn.Content)
			switch role {
			case "user":
				builder.WriteString(styleUserLabel.Render("You") + "\n")
			case "assistant":
				builder.WriteString(styleAssistantLabel.Render("Lore") + "\n")
			default:
				builder.WriteString(styleMutedText.Render(strings.ToUpper(role)) + "\n")
			}
			builder.WriteString(indentBlock(content, "  "))
			builder.WriteString("\n\n")
		}
	}

	if running && strings.TrimSpace(pendingLine) != "" {
		builder.WriteString(styleRunning.Render(glyphFocus+" You") + "\n")
		builder.WriteString(indentBlock(strings.TrimSpace(pendingLine), "  "))
		builder.WriteString("\n")
		builder.WriteString(styleRunning.Render("  "+glyphThinking+" thinking") + "\n\n")
	}

	if len(viewModel.ToolTrace) > 0 {
		builder.WriteString(thinRule(40) + "\n")
		builder.WriteString(styleSectionHead.Render("Tool Calls") + "\n")
		limit := minInt(5, len(viewModel.ToolTrace))
		for _, item := range viewModel.ToolTrace[:limit] {
			statusStyle := styleMutedText
			switch strings.ToLower(item.Status) {
			case "ok", "success":
				statusStyle = styleOK
			case "error", "fail", "failed":
				statusStyle = styleErr
			case "pending", "waiting":
				statusStyle = styleWarn
			case "cancelled", "canceled":
				statusStyle = styleMutedText
			case "running":
				statusStyle = styleRunning
			}
			builder.WriteString("  " + styleToolName.Render(oneLine(item.Name, 24)) + " " + statusStyle.Render(item.Status))
			if item.Error != "" {
				builder.WriteString(" " + styleErr.Render(oneLine(item.Error, 40)))
			}
			builder.WriteString("\n")
		}
		builder.WriteString("\n")
	}

	builder.WriteString(thinRule(40) + "\n")
	builder.WriteString(styleSectionHead.Render("Latest Output") + "\n")
	if strings.TrimSpace(lastOutput) == "" {
		builder.WriteString(styleMutedText.Render("  No active output.") + "\n")
	} else {
		builder.WriteString(indentBlock(strings.TrimSpace(excerpt(lastOutput, 2200)), "  "))
		builder.WriteString("\n")
	}

	return builder.String()
}

func renderInteractiveStatus(viewModel WorkbenchViewModel) string {
	var builder strings.Builder

	healthStyle := styleOK
	switch strings.ToLower(viewModel.Snapshot.HealthStatus) {
	case "error", "fail", "critical":
		healthStyle = styleErr
	case "warn", "warning", "degraded":
		healthStyle = styleWarn
	}

	readyStyle := styleOK
	readyLabel := "yes"
	if !viewModel.Snapshot.Ready {
		readyStyle = styleErr
		readyLabel = "no"
	}

	builder.WriteString(styleSectionHead.Render("System") + "\n")
	builder.WriteString("  Profile  " + viewModel.Snapshot.Profile + "\n")
	builder.WriteString("  Ready    " + readyStyle.Render(readyLabel) + "\n")
	builder.WriteString("  Health   " + healthStyle.Render(viewModel.Snapshot.HealthStatus) + "\n")
	if msg := oneLine(viewModel.Snapshot.HealthMessage, 48); msg != "" {
		builder.WriteString("  " + styleMutedText.Render(msg) + "\n")
	}
	builder.WriteString("  Drafts   " + viewModel.Snapshot.DraftSummary() + "\n")
	builder.WriteString("  Agent    " + oneLine(viewModel.Snapshot.AgentID, 20) + "\n")
	builder.WriteString("  Day      " + viewModel.Snapshot.Day.Format("2006-01-02") + "\n")

	builder.WriteString("\n" + styleSectionHead.Render("Core Docs") + "\n")
	if len(viewModel.ManagedCore) == 0 {
		builder.WriteString("  " + styleMutedText.Render("None.") + "\n")
	} else {
		for _, doc := range viewModel.ManagedCore {
			if doc.Exists {
				builder.WriteString("  " + styleOK.Render(glyphOK) + " " + doc.Name + "\n")
			} else {
				builder.WriteString("  " + styleErr.Render(glyphMissing) + " " + styleMutedText.Render(doc.Name) + "\n")
			}
		}
	}

	builder.WriteString("\n" + styleSectionHead.Render("Pending Drafts") + "\n")
	if len(viewModel.PendingDrafts) == 0 {
		builder.WriteString("  " + styleMutedText.Render("None.") + "\n")
	} else {
		limit := minInt(4, len(viewModel.PendingDrafts))
		for _, draft := range viewModel.PendingDrafts[:limit] {
			builder.WriteString("  " + styleWarn.Render(glyphItem) + " " + oneLine(draft.Title, 36) + "\n")
		}
	}

	builder.WriteString("\n" + styleSectionHead.Render("Process Sink") + "\n")
	if len(viewModel.ProcessSink.Checkpoints) == 0 {
		builder.WriteString("  " + styleMutedText.Render("No data.") + "\n")
	} else {
		latest := viewModel.ProcessSink.Checkpoints[len(viewModel.ProcessSink.Checkpoints)-1]
		builder.WriteString("  " + latest.Window.WindowStart.Format("15:04") + glyphDash + latest.Window.WindowEnd.Format("15:04") + "\n")
		builder.WriteString("  " + oneLine(latest.Title, 40) + "\n")
	}

	return builder.String()
}

func renderApprovalPlaceholder() string {
	return styleMutedText.Render("No pending actions.") + "\n" +
		styleMutedText.Render("Drafts go through review before apply.")
}

func renderPaneTitle(title string, focused bool, running bool, pendingLine string, spin string) string {
	status := ""
	if running && strings.TrimSpace(pendingLine) != "" {
		status = "  " + styleRunning.Render(strings.TrimSpace(spin)+" running")
	}
	if focused {
		return focusedTitleStyle.Render(glyphFocus+" "+title) + status
	}
	return titleStyle.Render("  "+title) + status
}

func renderInputHeader(focused bool, running bool) string {
	if running {
		return focusedTitleStyle.Render(glyphFocus+" Input") + "  " + styleRunning.Render("agent running...")
	}
	label := "  Input"
	if focused {
		label = glyphFocus + " Input"
	}
	return focusedTitleStyle.Render(label) + "  " + styleMutedText.Render("Enter send "+glyphSep+" Tab switch "+glyphSep+" Ctrl+C quit")
}
