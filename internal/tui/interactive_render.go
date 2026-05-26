package tui

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/charmbracelet/lipgloss"

	"obsidian-harness/internal/app"
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
	// Input pane budget: header(1) + textarea(3) + border(2) = 6 lines max.
	// Reserve enough so total layout ≤ model.height.
	inputBudget := maxInt(5, model.height-contentHeight)
	if inputBudget > 8 {
		inputBudget = 8
	}

	leftPane := clipPaneLines(
		paneStyle(model.focus == focusConversation).Width(leftWidth).Height(contentHeight).Render(
			renderPaneTitle("Conversation Lane", model.focus == focusConversation, model.running, model.pendingLine, model.spin.View())+"\n"+model.chatViewport.View(),
		), contentHeight)
	statusPane := clipPaneLines(
		paneStyle(model.focus == focusStatus).Width(rightWidth).Height(rightTopHeight).Render(
			renderPaneTitle("Context & Status", model.focus == focusStatus, false, "", "")+"\n"+model.statusViewport.View(),
		), rightTopHeight)

	var approvalTitle string
	if narrow {
		approvalTitle = "Drafts"
	} else {
		approvalTitle = "Reviewable Drafts"
	}

	// Right-bottom pane switches content based on focus
	var rightBottomContent string
	var rightBottomTitle string
	switch model.focus {
	case focusFindings:
		rightBottomTitle = "Findings"
		rightBottomContent = renderFindingsPane(model.viewModel.Findings, model.findingsCursor, model.findingsOffset, model.findingsDetail, rightWidth-4, visiblePanelHeight(model.findingsHeight, rightBottomHeight-4))
	case focusProcessSink:
		if narrow {
			rightBottomTitle = "Sink Timeline"
		} else {
			rightBottomTitle = "Process-sink Timeline"
		}
		rightBottomContent = renderSinkTimelinePane(model.viewModel.ProcessSink, model.sinkCursor, model.sinkOffset, model.sinkDetail, rightWidth-4, visiblePanelHeight(model.sinkHeight, rightBottomHeight-4))
	default:
		// focusApproval (and any other) shows drafts
		rightBottomTitle = approvalTitle
		rightBottomContent = renderApprovalPane(model.viewModel.PendingDrafts, model.approvalCursor, model.approvalOffset, model.approvalDetail, model.viewModel.FocusedReview, rightWidth-4, visiblePanelHeight(model.approvalHeight, rightBottomHeight-4))
	}

	rightBottomPane := clipPaneLines(
		paneStyle(model.focus == focusApproval || model.focus == focusFindings || model.focus == focusProcessSink).Width(rightWidth).Height(rightBottomHeight).Render(
			renderPaneTitle(rightBottomTitle, model.focus == focusApproval || model.focus == focusFindings || model.focus == focusProcessSink, false, "", "")+"\n"+rightBottomContent,
		), rightBottomHeight)
	rightColumn := lipgloss.JoinVertical(lipgloss.Left, statusPane, rightBottomPane)

	inputPane := clipPaneLines(
		inputPaneStyle(model.focus == focusInput).Width(w).Render(
			renderInputHeader(model.focus == focusInput, model.running)+"\n"+model.input.View(),
		), inputBudget)

	return clipPaneLines(
		lipgloss.JoinVertical(lipgloss.Left,
			lipgloss.JoinHorizontal(lipgloss.Top, leftPane, rightColumn),
			inputPane,
		), model.height)
}

// clipPaneLines trims a multi-line string to at most maxLines lines.
// Preserves ANSI escape sequences; used as a safety net so no pane can
// visually overflow its allocated height budget and bleed into adjacent
// panes on the next frame.
func clipPaneLines(content string, maxLines int) string {
	if maxLines <= 0 {
		return ""
	}
	lines := strings.Split(content, "\n")
	if len(lines) <= maxLines {
		return content
	}
	return strings.Join(lines[:maxLines], "\n")
}

func visiblePanelHeight(configured int, fallback int) int {
	if configured > 0 {
		return configured
	}
	return maxInt(3, fallback)
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

	if len(viewModel.TurnSteps) > 0 {
		builder.WriteString(thinRule(40) + "\n")
		builder.WriteString(styleSectionHead.Render("Task Steps") + "\n")
		for _, step := range viewModel.TurnSteps {
			statusStyle := styleMutedText
			statusLabel := step.Status
			switch strings.ToLower(step.Status) {
			case "ok", "success":
				statusStyle = styleOK
				statusLabel = "ok"
			case "error", "fail", "failed":
				statusStyle = styleErr
				statusLabel = "error"
			case "pending", "waiting":
				statusStyle = styleWarn
				statusLabel = "pending"
			}

			builder.WriteString(fmt.Sprintf("  %d. %s %s\n", step.Index, styleToolName.Render(oneLine(step.Tool, 24)), statusStyle.Render(statusLabel)))

			// Argument summary
			if argLine := stepArgSummary(step.Tool, step.Arguments); argLine != "" {
				builder.WriteString("     " + styleMutedText.Render(oneLine(argLine, contentWidth-6)) + "\n")
			}

			// Observation excerpt (when B-line populates it)
			if step.ObservationExcerpt != "" {
				excerpt := renderObservationExcerpt(step.ObservationExcerpt, 200)
				builder.WriteString("     " + styleMutedText.Render("obs: "+excerpt) + "\n")
			}

			// Error detail
			if step.Error != "" {
				builder.WriteString("     " + styleErr.Render(oneLine(step.Error, contentWidth-6)) + "\n")
			}
		}
		builder.WriteString("\n")
	}

	// Show error/status line only when lastOutput is an error
	if strings.HasPrefix(strings.TrimSpace(lastOutput), "Error:") {
		builder.WriteString(thinRule(40) + "\n")
		builder.WriteString(styleSectionHead.Render("Status") + "\n")
		builder.WriteString(styleErr.Render("  " + wrapText(strings.TrimSpace(unescapeLiteralNewlines(lastOutput)), contentWidth)))
		builder.WriteString("\n")
	}

	return builder.String()
}

// renderObservationExcerpt flattens a multi-line observation into a single
// display line, capped at maxRunes runes (not bytes). If the original text
// contains a "[truncated ...]" marker (placed by B-line), the marker is
// preserved even when the preceding content must be cut.
func renderObservationExcerpt(text string, maxRunes int) string {
	flat := strings.Join(strings.Fields(text), " ")
	runes := []rune(flat)
	if len(runes) <= maxRunes {
		return flat
	}
	// Check for truncated marker from B-line
	if idx := strings.LastIndex(flat, "[truncated"); idx > 0 {
		markerRunes := []rune(flat[idx:])
		headroom := maxRunes - len(markerRunes) - 4 // "..." + space
		if headroom > 20 {
			return string(runes[:headroom]) + "... " + string(markerRunes)
		}
	}
	return string(runes[:maxRunes-3]) + "..."
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

func renderApprovalPane(drafts []model.Draft, cursor int, offset int, detail bool, focusedReview *app.DraftReview, width int, height int) string {
	if len(drafts) == 0 {
		return styleMutedText.Render("No reviewable drafts.") + "\n" +
			styleMutedText.Render("Proposals appear here for review.")
	}

	if detail && cursor < len(drafts) {
		return renderApprovalDetailWithTarget(drafts[cursor], focusedReview, width, height)
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

		stateBadge := renderDraftStateBadge(draft.State)
		kindBadge := renderDraftKindBadge(draft.Kind)
		idShort := shortDraftID(draft.ID)
		title := oneLine(draft.Title, maxInt(8, width-22))
		builder.WriteString(prefix + stateBadge + " " + kindBadge + " " + styleMutedText.Render(idShort) + " " + titleStyle.Render(title))
		builder.WriteString("\n")
	}

	// Contextual footer: show available actions for the currently selected draft
	footer := fmt.Sprintf("[%d/%d]", cursor+1, len(drafts))
	if cursor < len(drafts) {
		switch drafts[cursor].State {
		case model.DraftPendingReview:
			footer += " enter=detail a=approve r=reject"
		case model.DraftApproved:
			footer += " enter=detail p=apply"
		default:
			footer += " enter=detail"
		}
	} else {
		footer += " enter=detail"
	}
	builder.WriteString(styleMutedText.Render(footer))
	builder.WriteString("\n")

	return builder.String()
}

func renderDraftStateBadge(state model.DraftState) string {
	switch state {
	case model.DraftApproved:
		return styleOK.Render("ok")
	case model.DraftPendingReview:
		return styleWarn.Render("rev")
	case model.DraftApplied:
		return styleOK.Render("app")
	case model.DraftRejected:
		return styleMutedText.Render("rej")
	case model.DraftConflicted:
		return styleErr.Render("cnf")
	default:
		return styleMutedText.Render(string(state[:minInt(3, len(state))]))
	}
}

func renderDraftKindBadge(kind model.DraftKind) string {
	switch kind {
	case model.DraftKindPersonaUpdate:
		return styleAssistantLabel.Render("persona")
	case model.DraftKindMarkdownNoteWrite:
		return styleToolName.Render("note")
	case model.DraftKindProgressSync:
		return styleOK.Render("sync")
	case model.DraftKindWeaknessUpdate:
		return styleErr.Render("weak")
	case model.DraftKindPlanAdjustment:
		return styleWarn.Render("plan")
	default:
		return styleMutedText.Render(string(kind))
	}
}

func shortDraftID(id string) string {
	if len(id) <= 7 {
		return id
	}
	return id[:7]
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

// --- Findings pane ---

func renderFindingsPane(findings []model.Finding, cursor int, offset int, detail bool, width int, height int) string {
	if len(findings) == 0 {
		return styleMutedText.Render("No findings.") + "\n" +
			styleMutedText.Render("Governance findings appear here.")
	}

	if detail && cursor < len(findings) {
		return renderFindingsDetail(findings[cursor], width, height)
	}

	return renderFindingsList(findings, cursor, offset, width, height)
}

func renderFindingsList(findings []model.Finding, cursor int, offset int, width int, height int) string {
	var builder strings.Builder

	listHeight := height - 1
	if listHeight < 1 {
		listHeight = 1
	}

	end := offset + listHeight
	if end > len(findings) {
		end = len(findings)
	}

	for i := offset; i < end; i++ {
		f := findings[i]
		prefix := "  "
		titleStyle := styleMutedText
		if i == cursor {
			prefix = styleWarn.Render(glyphFocus + " ")
			titleStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("#E5E7EB"))
		}

		var sevBadge string
		switch f.Severity {
		case model.FindingSeverityCritical:
			sevBadge = styleErr.Render("crit")
		case model.FindingSeverityWarning:
			sevBadge = styleWarn.Render("warn")
		default:
			sevBadge = styleOK.Render("info")
		}

		var stateBadge string
		switch f.State {
		case model.FindingResolved:
			stateBadge = styleOK.Render("ok")
		case model.FindingIgnored:
			stateBadge = styleMutedText.Render("ign")
		default:
			stateBadge = styleWarn.Render("open")
		}

		title := oneLine(f.Title, maxInt(12, width-14))
		builder.WriteString(prefix + sevBadge + " " + stateBadge + " " + titleStyle.Render(title))
		builder.WriteString("\n")
	}

	builder.WriteString(styleMutedText.Render(fmt.Sprintf("[%d/%d] enter=detail", cursor+1, len(findings))))
	builder.WriteString("\n")

	return builder.String()
}

func renderFindingsDetail(f model.Finding, width int, height int) string {
	var builder strings.Builder

	builder.WriteString(styleSectionHead.Render("Finding Detail") + "\n")
	builder.WriteString("  Kind     " + string(f.Kind) + "\n")
	builder.WriteString("  Severity " + string(f.Severity) + "\n")
	builder.WriteString("  State    " + string(f.State) + "\n")
	builder.WriteString("  Target   " + oneLine(f.Target.Path, maxInt(10, width-10)) + "\n")
	builder.WriteString("  Source   " + oneLine(f.Source, maxInt(10, width-10)) + "\n")
	builder.WriteString("  Detected " + f.DetectedAt.Format("2006-01-02 15:04") + "\n")

	if f.Summary != "" {
		builder.WriteString("\n" + styleSectionHead.Render("Summary") + "\n")
		summaryLines := maxInt(1, (height-10)/2)
		summary := oneLine(f.Summary, width*summaryLines)
		builder.WriteString("  " + wrapText(summary, maxInt(10, width-2)) + "\n")
	}

	if f.Detail != "" {
		builder.WriteString("\n" + styleSectionHead.Render("Detail") + "\n")
		detailLines := maxInt(1, (height-12)/2)
		detail := oneLine(f.Detail, width*detailLines)
		builder.WriteString("  " + wrapText(detail, maxInt(10, width-2)) + "\n")
	}

	builder.WriteString("\n")
	switch f.State {
	case model.FindingOpen:
		builder.WriteString(styleOK.Render("x") + "=resolve " + styleMutedText.Render("i") + "=ignore " + styleMutedText.Render("esc=back"))
	default:
		builder.WriteString(styleMutedText.Render("esc=back"))
	}

	return builder.String()
}

// --- Process-sink timeline pane ---

func renderSinkTimelinePane(sink app.ProcessSinkDayView, cursor int, offset int, detail bool, width int, height int) string {
	if len(sink.Checkpoints) == 0 {
		return styleMutedText.Render("No checkpoints today.") + "\n" +
			styleMutedText.Render("Session checkpoints appear here.")
	}

	if detail && cursor < len(sink.Checkpoints) {
		return renderSinkDetail(sink, cursor, width, height)
	}

	return renderSinkTimelineList(sink, cursor, offset, width, height)
}

func renderSinkTimelineList(sink app.ProcessSinkDayView, cursor int, offset int, width int, height int) string {
	var builder strings.Builder

	listHeight := height - 2
	if listHeight < 1 {
		listHeight = 1
	}

	end := offset + listHeight
	if end > len(sink.Checkpoints) {
		end = len(sink.Checkpoints)
	}

	for i := offset; i < end; i++ {
		cp := sink.Checkpoints[i]
		prefix := "  "
		timeStyle := styleMutedText
		titleStyle := styleMutedText
		if i == cursor {
			prefix = styleWarn.Render(glyphFocus + " ")
			timeStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("#E5E7EB"))
			titleStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("#E5E7EB"))
		}

		timeRange := cp.Window.WindowStart.Format("15:04") + glyphDash + cp.Window.WindowEnd.Format("15:04")
		title := oneLine(cp.Title, maxInt(10, width-12))

		var stateBadge string
		switch cp.State {
		case model.CheckpointMaterialized:
			stateBadge = styleOK.Render("*")
		default:
			stateBadge = styleMutedText.Render("o")
		}

		builder.WriteString(prefix + timeStyle.Render(timeRange) + " " + stateBadge + " " + titleStyle.Render(title))
		builder.WriteString("\n")
	}

	// Report status line
	if sink.Report != nil {
		builder.WriteString(styleMutedText.Render("report: " + oneLine(sink.Report.Title, maxInt(10, width-10))))
		builder.WriteString("\n")
	}

	builder.WriteString(styleMutedText.Render(fmt.Sprintf("[%d/%d] enter=detail j/k=navigate", cursor+1, len(sink.Checkpoints))))
	builder.WriteString("\n")

	return builder.String()
}

func renderSinkDetail(sink app.ProcessSinkDayView, idx int, width int, height int) string {
	var builder strings.Builder
	cp := sink.Checkpoints[idx]

	builder.WriteString(styleSectionHead.Render("Checkpoint Detail") + "\n")
	builder.WriteString("  Window  " + cp.Window.WindowStart.Format("15:04") + glyphDash + cp.Window.WindowEnd.Format("15:04") + "\n")
	builder.WriteString("  State   " + string(cp.State) + "\n")
	builder.WriteString("  Session " + oneLine(cp.Window.SessionID, maxInt(10, width-10)) + "\n")
	builder.WriteString("  Path    " + oneLine(cp.Path, maxInt(10, width-10)) + "\n")

	if cp.Title != "" {
		builder.WriteString("\n" + styleSectionHead.Render("Title") + "\n")
		builder.WriteString("  " + wrapText(cp.Title, maxInt(10, width-2)) + "\n")
	}

	if cp.Content != "" {
		builder.WriteString("\n" + styleSectionHead.Render("Content") + "\n")
		contentLines := maxInt(1, (height-12)/2)
		content := oneLine(cp.Content, width*contentLines)
		builder.WriteString("  " + wrapText(content, maxInt(10, width-2)) + "\n")
	}

	builder.WriteString("\n")
	builder.WriteString(styleMutedText.Render("esc=back"))

	return builder.String()
}

// --- Draft detail diff (enhanced approval detail) ---

func renderApprovalDetailWithTarget(draft model.Draft, review *app.DraftReview, width int, height int) string {
	var builder strings.Builder

	builder.WriteString(styleSectionHead.Render("Draft Detail") + "\n")
	builder.WriteString("  ID     " + styleMutedText.Render(draft.ID) + "\n")
	builder.WriteString("  Kind   " + string(draft.Kind) + "\n")
	builder.WriteString("  Target " + oneLine(draft.Target.Path, maxInt(10, width-10)) + "\n")
	builder.WriteString("  State  " + string(draft.State) + "\n")

	// Parse ProposedContent for structured display
	switch draft.Kind {
	case model.DraftKindPersonaUpdate:
		renderPersonaUpdateDetail(&builder, draft, width, height)
	case model.DraftKindMarkdownNoteWrite:
		renderMarkdownNoteDetail(&builder, draft, review, width, height)
	default:
		renderGenericProposalDetail(&builder, draft, width, height)
	}

	// Current target content for diff comparison
	if review != nil && review.TargetDocument != nil && strings.TrimSpace(review.TargetDocument.Content) != "" {
		builder.WriteString("\n" + styleSectionHead.Render("Current Target") + "\n")
		diffLines := maxInt(1, (height-18)/4)
		current := oneLine(review.TargetDocument.Content, width*diffLines)
		builder.WriteString("  " + wrapText(current, maxInt(10, width-2)) + "\n")
	}

	builder.WriteString("\n")
	switch draft.State {
	case model.DraftPendingReview:
		builder.WriteString(styleOK.Render("a") + "=同意 " + styleErr.Render("r") + "=拒绝 " + styleMutedText.Render("esc=返回"))
	case model.DraftApproved:
		builder.WriteString(styleOK.Render("p") + "=应用 " + styleMutedText.Render("esc=返回"))
	default:
		builder.WriteString(styleMutedText.Render("esc=返回"))
	}

	return builder.String()
}

func renderPersonaUpdateDetail(b *strings.Builder, draft model.Draft, width int, _ int) {
	var prop model.PersonaUpdateProposal
	if err := json.Unmarshal([]byte(draft.ProposedContent), &prop); err != nil {
		b.WriteString("\n" + styleSectionHead.Render("Proposed") + "\n")
		b.WriteString("  " + wrapText(draft.ProposedContent, maxInt(10, width-2)) + "\n")
		return
	}

	if prop.Field != "" {
		b.WriteString("\n" + styleSectionHead.Render("Plan") + "\n")
		b.WriteString("  Field:    " + styleWarn.Render(prop.Field) + "\n")
	}
	if prop.CurrentValue != "" {
		b.WriteString("  Current:  " + styleMutedText.Render(oneLine(prop.CurrentValue, maxInt(8, width-14))) + "\n")
	} else {
		b.WriteString("  Current:  " + styleMutedText.Render("(empty)") + "\n")
	}
	if prop.ProposedValue != "" {
		b.WriteString("  Proposed: " + styleOK.Render(oneLine(prop.ProposedValue, maxInt(8, width-14))) + "\n")
	}
	if prop.Evidence != "" {
		b.WriteString("\n" + styleSectionHead.Render("Evidence") + "\n")
		b.WriteString("  " + wrapText(prop.Evidence, maxInt(10, width-2)) + "\n")
	}
	if prop.Reason != "" {
		b.WriteString("\n" + styleSectionHead.Render("Reason") + "\n")
		b.WriteString("  " + wrapText(prop.Reason, maxInt(10, width-2)) + "\n")
	}
	if prop.Confidence != "" {
		b.WriteString("  Confidence: " + oneLine(prop.Confidence, maxInt(8, width-16)) + "\n")
	}
}

func renderMarkdownNoteDetail(b *strings.Builder, draft model.Draft, review *app.DraftReview, width int, height int) {
	var prop model.MarkdownNoteProposal
	if err := json.Unmarshal([]byte(draft.ProposedContent), &prop); err != nil {
		renderGenericProposalDetail(b, draft, width, height)
		return
	}

	if prop.Title != "" {
		b.WriteString("\n" + styleSectionHead.Render("Title") + "\n")
		b.WriteString("  " + wrapText(prop.Title, maxInt(10, width-2)) + "\n")
	}
	if prop.Reason != "" {
		b.WriteString("\n" + styleSectionHead.Render("Reason") + "\n")
		b.WriteString("  " + wrapText(prop.Reason, maxInt(10, width-2)) + "\n")
	}
	if prop.Content != "" {
		b.WriteString("\n" + styleSectionHead.Render("Proposed Content") + "\n")
		excerptLines := maxInt(1, (height-10)/3)
		excerpt := oneLine(prop.Content, width*excerptLines)
		b.WriteString("  " + wrapText(excerpt, maxInt(10, width-2)) + "\n")
	}
	if prop.Evidence != "" {
		b.WriteString("\n" + styleSectionHead.Render("Evidence") + "\n")
		b.WriteString("  " + wrapText(prop.Evidence, maxInt(10, width-2)) + "\n")
	}
}

func renderGenericProposalDetail(b *strings.Builder, draft model.Draft, width int, height int) {
	if draft.Summary != "" {
		b.WriteString("\n" + styleSectionHead.Render("Summary") + "\n")
		summaryLines := maxInt(1, (height-12)/3)
		summary := oneLine(draft.Summary, width*summaryLines)
		b.WriteString("  " + wrapText(summary, maxInt(10, width-2)) + "\n")
	}
	if draft.ProposedContent != "" {
		b.WriteString("\n" + styleSectionHead.Render("Proposed") + "\n")
		contentLines := maxInt(1, (height-14)/3)
		content := oneLine(draft.ProposedContent, width*contentLines)
		b.WriteString("  " + wrapText(content, maxInt(10, width-2)) + "\n")
	}
}
