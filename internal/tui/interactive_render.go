package tui

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/charmbracelet/lipgloss"

	"obsidian-harness/internal/model"
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
		approvalTitle = "Drafts"
	} else {
		approvalTitle = "Reviewable Drafts"
	}
	approvalPane := paneStyle(model.focus == focusApproval).Width(rightWidth).Height(rightBottomHeight).Render(
		renderPaneTitle(approvalTitle, model.focus == focusApproval, false, "", "") + "\n" + renderApprovalPane(model.viewModel.PendingDrafts, model.approvalCursor, model.approvalOffset, model.approvalDetail, rightWidth-4, rightBottomHeight-4),
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

func renderInteractiveConversation(viewModel WorkbenchViewModel, lastOutput string, running bool, pendingLine string, wrapWidth int) string {
	var builder strings.Builder

	turns := viewModel.Conversation.Turns

	contentWidth := wrapWidth - 6
	if contentWidth < 1 {
		contentWidth = 1
	}

	if len(turns) == 0 {
		builder.WriteString(styleMutedText.Render("Ready. Ask Lore about your vault, drafts, or process sink."))
		builder.WriteString("\n")
	} else {
		prevRole := ""
		for _, turn := range turns {
			role := strings.ToLower(strings.TrimSpace(turn.Role))
			content := wrapText(unescapeLiteralNewlines(strings.TrimSpace(turn.Content)), contentWidth)
			if role != prevRole {
				switch role {
				case "user":
					builder.WriteString(styleUserLabel.Render("You") + "\n")
				case "assistant":
					builder.WriteString(styleAssistantLabel.Render("Lore") + "\n")
				default:
					builder.WriteString(styleMutedText.Render(strings.ToUpper(role)) + "\n")
				}
			}
			prevRole = role
			builder.WriteString(indentBlock(content, "  "))
			builder.WriteString("\n\n")
		}
	}

	if running && strings.TrimSpace(pendingLine) != "" {
		builder.WriteString(styleRunning.Render(glyphFocus+" You") + "\n")
		builder.WriteString(indentBlock(wrapText(strings.TrimSpace(pendingLine), contentWidth), "  "))
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
		builder.WriteString(indentBlock(wrapText(strings.TrimSpace(unescapeLiteralNewlines(lastOutput)), contentWidth), "  "))
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
	if viewModel.Snapshot.SessionID != "" {
		builder.WriteString("  Session  " + oneLine(viewModel.Snapshot.SessionID, 20) + "\n")
	}
	if viewModel.Snapshot.TranscriptPath != "" {
		builder.WriteString("  Log      " + oneLine(filepath.Base(viewModel.Snapshot.TranscriptPath), 32) + "\n")
	}
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

	builder.WriteString("\n" + styleSectionHead.Render("Reviewable Drafts") + "\n")
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

func renderApprovalPane(drafts []model.Draft, cursor int, offset int, detail bool, width int, height int) string {
	if len(drafts) == 0 {
		return styleMutedText.Render("No reviewable drafts.") + "\n" +
			styleMutedText.Render("Proposals appear here for review.")
	}

	if detail && cursor < len(drafts) {
		return renderApprovalDetail(drafts[cursor], width, height)
	}

	return renderApprovalList(drafts, cursor, offset, width, height)
}

func renderApprovalList(drafts []model.Draft, cursor int, offset int, width int, height int) string {
	var builder strings.Builder

	// Reserve 1 line for position hint
	listHeight := height - 1
	if listHeight < 1 {
		listHeight = 1
	}

	// Clamp offset
	end := offset + listHeight
	if end > len(drafts) {
		end = len(drafts)
	}

	for i := offset; i < end; i++ {
		draft := drafts[i]
		prefix := "  "
		titleStyle := styleMutedText
		if i == cursor {
			prefix = styleWarn.Render(glyphFocus + " ")
			titleStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("#E5E7EB"))
		}

		// State badge
		var stateBadge string
		switch draft.State {
		case model.DraftApproved:
			stateBadge = styleOK.Render("approved")
		case model.DraftPendingReview:
			stateBadge = styleWarn.Render("review")
		default:
			stateBadge = styleMutedText.Render(string(draft.State))
		}

		title := oneLine(draft.Title, maxInt(16, width-8))

		builder.WriteString(prefix + stateBadge + " " + titleStyle.Render(title))
		builder.WriteString("\n")
	}

	builder.WriteString(styleMutedText.Render(fmt.Sprintf("[%d/%d] enter=detail", cursor+1, len(drafts))))
	builder.WriteString("\n")

	return builder.String()
}

func renderApprovalDetail(draft model.Draft, width int, height int) string {
	var builder strings.Builder

	builder.WriteString(styleSectionHead.Render("Draft Detail") + "\n")
	builder.WriteString("  Kind   " + string(draft.Kind) + "\n")
	builder.WriteString("  Target " + oneLine(draft.Target.Path, maxInt(10, width-10)) + "\n")
	builder.WriteString("  State  " + string(draft.State) + "\n")

	if draft.Summary != "" {
		builder.WriteString("\n" + styleSectionHead.Render("Summary") + "\n")
		summaryLines := maxInt(1, (height-8)/2)
		summary := oneLine(draft.Summary, width*summaryLines)
		builder.WriteString("  " + wrapText(summary, maxInt(10, width-2)) + "\n")
	}

	if draft.ProposedContent != "" {
		builder.WriteString("\n" + styleSectionHead.Render("Proposed") + "\n")
		contentLines := maxInt(1, (height-10)/2)
		content := oneLine(draft.ProposedContent, width*contentLines)
		builder.WriteString("  " + wrapText(content, maxInt(10, width-2)) + "\n")
	}

	builder.WriteString("\n")
	switch draft.State {
	case model.DraftPendingReview:
		builder.WriteString(styleWarn.Render("a") + "=approve " + styleErr.Render("r") + "=reject " + styleMutedText.Render("esc=back"))
	case model.DraftApproved:
		builder.WriteString(styleOK.Render("p") + "=apply " + styleMutedText.Render("esc=back"))
	default:
		builder.WriteString(styleMutedText.Render("esc=back"))
	}

	return builder.String()
}

func renderSessionDetail(snap WorkbenchSnapshot) string {
	var b strings.Builder
	b.WriteString("Session Info\n")
	b.WriteString("  Profile    " + snap.Profile + "\n")
	b.WriteString("  Agent      " + snap.AgentID + "\n")
	if snap.SessionID != "" {
		b.WriteString("  Session    " + snap.SessionID + "\n")
	}
	if snap.TranscriptPath != "" {
		b.WriteString("  Log        " + snap.TranscriptPath + "\n")
	}
	if snap.DailyReportPath != "" {
		b.WriteString("  Report     " + snap.DailyReportPath + "\n")
	}
	b.WriteString("  Day        " + snap.Day.Format("2006-01-02") + "\n")
	b.WriteString("  Health     " + snap.HealthStatus + "\n")
	if snap.HealthMessage != "" {
		b.WriteString("  Message    " + snap.HealthMessage + "\n")
	}
	return b.String()
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
	return focusedTitleStyle.Render(label) + "  " + styleMutedText.Render("Enter send "+glyphSep+" Ctrl+J newline "+glyphSep+" Ctrl+C copy "+glyphSep+" Ctrl+Q quit")
}
