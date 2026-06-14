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
	if model.modelPanelActive {
		switch model.modelPanelMode {
		case modelPanelPresets:
			rightBottomTitle = "New Profile (pick preset)"
			rightBottomContent = renderPresetPicker(model.modelPanelPresets, model.modelPanelCursor, rightWidth-4, visiblePanelHeight(model.approvalHeight, rightBottomHeight-4))
		case modelPanelProfiles:
			rightBottomTitle = "Model Profiles"
			rightBottomContent = renderProfilePanel(model.modelPanelList, model.modelPanelCursor, model.modelPanelEditing, model.modelPanelEditText, rightWidth-4, visiblePanelHeight(model.approvalHeight, rightBottomHeight-4))
		default:
			rightBottomTitle = "Model"
			rightBottomContent = renderModelPanel(model.modelPanelList, model.modelPanelCursor, model.modelPanelEditing, model.modelPanelEditText, rightWidth-4, visiblePanelHeight(model.approvalHeight, rightBottomHeight-4))
		}
	} else {
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
		case focusCandidates:
			rightBottomTitle = "Persona Candidates"
			rightBottomContent = renderCandidatePanel(model.viewModel.CandidateList, model.candidatePanelCursor, model.candidatePanelDetail, rightWidth-4, visiblePanelHeight(model.approvalHeight, rightBottomHeight-4))
		}
	}

	rightBottomFocused := model.modelPanelActive || model.focus == focusApproval || model.focus == focusFindings || model.focus == focusProcessSink || model.focus == focusCandidates

	rightBottomPane := clipPaneLines(
		paneStyle(rightBottomFocused).Width(rightWidth).Height(rightBottomHeight).Render(
			renderPaneTitle(rightBottomTitle, rightBottomFocused, false, "", "")+"\n"+rightBottomContent,
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

	// Three-line model identity display
	builder.WriteString("\n" + styleSectionHead.Render("Models") + "\n")
	renderModelIdentityLine(&builder, "Chat", viewModel.ChatModelIdentity, viewModel.Snapshot.CurrentModel)
	renderModelIdentityLine(&builder, "Persona", viewModel.PersonaModelIdentity, "")
	renderModelIdentityLine(&builder, "Sink", viewModel.SinkModelIdentity, "")

	// Warning if chat and persona use different models
	if viewModel.ChatModelIdentity != nil && viewModel.PersonaModelIdentity != nil &&
		viewModel.ChatModelIdentity.Enabled && viewModel.PersonaModelIdentity.Enabled &&
		viewModel.ChatModelIdentity.Model != "" && viewModel.PersonaModelIdentity.Model != "" &&
		viewModel.ChatModelIdentity.Model != viewModel.PersonaModelIdentity.Model {
		builder.WriteString("  " + styleWarn.Render("chat/persona models differ") + "\n")
	}

	if viewModel.Snapshot.SessionID != "" {
		builder.WriteString("\n" + styleSectionHead.Render("Session") + "\n")
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

// --- Model panel ---

func renderModelPanel(models []ModelInfo, cursor int, editing bool, editText string, width int, height int) string {
	if len(models) == 0 {
		return styleMutedText.Render("No models discovered.") + "\n" +
			styleMutedText.Render("Check LORE_LLM_BASE_URL and LORE_LLM_API_KEY.")
	}

	var builder strings.Builder

	listHeight := height - 2
	if listHeight < 1 {
		listHeight = 1
	}

	start := cursor - listHeight/2
	if start < 0 {
		start = 0
	}
	end := start + listHeight
	if end > len(models) {
		end = len(models)
		start = maxInt(0, end-listHeight)
	}

	for i := start; i < end; i++ {
		m := models[i]
		prefix := "  "
		nameStyle := styleMutedText
		if i == cursor {
			prefix = styleWarn.Render(glyphFocus + " ")
			nameStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("#E5E7EB"))
		}

		currentMark := "  "
		if m.Current {
			currentMark = styleOK.Render("* ")
		}

		testMark := styleMutedText.Render("? ")
		if m.TestStatus == "OK" {
			testMark = styleOK.Render("ok ")
		} else if strings.HasPrefix(m.TestStatus, "failed") {
			testMark = styleErr.Render("!! ")
		}

		line := prefix + currentMark + nameStyle.Render(m.Name)
		if width > 55 {
			line += "  " + testMark + styleMutedText.Render(m.Provider)
		}
		builder.WriteString(oneLine(line, maxInt(8, width-2)))
		builder.WriteString("\n")
	}

	if editing {
		builder.WriteString(styleWarn.Render("model> ") + editText)
		builder.WriteString("\n")
	}

	footer := fmt.Sprintf("[%d/%d] r=refresh t=test enter=use e=edit s=profiles n=new esc=back", cursor+1, len(models))
	if len(models) > 0 && models[0].BaseURL != "" {
		footer += " @ " + models[0].BaseURL
	}
	builder.WriteString(styleMutedText.Render(footer))
	builder.WriteString("\n")

	return builder.String()
}

func renderCandidatePanel(candidates []PersonaCandidateInfo, cursor int, detail bool, width int, height int) string {
	if len(candidates) == 0 {
		return styleMutedText.Render("No persona candidates yet.") + "\n" +
			styleMutedText.Render("Candidates appear after chat turns.") + "\n" +
			styleMutedText.Render("Use /candidates to refresh.")
	}

	if detail && cursor < len(candidates) {
		return renderCandidateDetail(candidates[cursor], width, height)
	}

	return renderCandidateList(candidates, cursor, width, height)
}

func renderCandidateList(candidates []PersonaCandidateInfo, cursor int, width int, height int) string {
	var builder strings.Builder

	listHeight := height - 2
	if listHeight < 1 {
		listHeight = 1
	}

	start := cursor - listHeight/2
	if start < 0 {
		start = 0
	}
	end := start + listHeight
	if end > len(candidates) {
		end = len(candidates)
		start = maxInt(0, end-listHeight)
	}

	for i := start; i < end; i++ {
		c := candidates[i]
		prefix := "  "
		nameStyle := styleMutedText
		if i == cursor {
			prefix = styleWarn.Render(glyphFocus + " ")
			nameStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("#E5E7EB"))
		}

		// State badge
		stateBadge := renderCandidateStateBadge(c.State)

		// Field + proposed value (truncated)
		field := oneLine(c.Field, 12)
		proposed := oneLine(c.ProposedValue, 20)
		conflictMark := ""
		if c.Conflict {
			conflictMark = styleErr.Render("!")
		}

		line := prefix + stateBadge + " " + nameStyle.Render(field) + " " + styleMutedText.Render(proposed) + conflictMark
		builder.WriteString(oneLine(line, maxInt(8, width-2)))
		builder.WriteString("\n")

		// Orphan partial detection
		if i == cursor && c.State == "drafted" && c.DraftID == "" {
			builder.WriteString("  " + styleErr.Render("orphan partial: needs recover or force-dismiss") + "\n")
		}
	}

	footer := fmt.Sprintf("[%d/%d]", cursor+1, len(candidates))
	if cursor < len(candidates) {
		c := candidates[cursor]
		// Derive footer actions from the B-line Actions bundle (single source of truth)
		if c.Actions.CanDraft {
			footer += " d=draft"
		}
		if c.Actions.CanDismiss {
			footer += " x=dismiss"
		}
		if c.Actions.CanRetry {
			footer += " r=retry"
		} else if c.Actions.CanRecover && c.DraftID == "" {
			footer += " f=force-dismiss"
		} else if c.Actions.CanRecover {
			footer += " r=recover"
		}
		if c.State == "drafted" && c.DraftID != "" {
			footer += " draft=" + shortID(c.DraftID, 8)
		}
	}
	footer += " enter=detail esc=tab"
	builder.WriteString(styleMutedText.Render(footer))
	builder.WriteString("\n")

	return builder.String()
}

func renderCandidateDetail(c PersonaCandidateInfo, width int, height int) string {
	var builder strings.Builder

	builder.WriteString(styleSectionHead.Render("Candidate Detail") + "\n")
	builder.WriteString("  ID      " + styleMutedText.Render(shortID(c.ID, 16)) + "\n")
	builder.WriteString("  State   " + renderCandidateStateBadge(c.State) + "\n")
	builder.WriteString("  Field   " + styleWarn.Render(c.Field) + "\n")

	if c.CurrentValue != "" {
		builder.WriteString("  Current " + styleMutedText.Render(oneLine(c.CurrentValue, maxInt(8, width-12))) + "\n")
	} else {
		builder.WriteString("  Current " + styleMutedText.Render("(empty)") + "\n")
	}

	builder.WriteString("  Proposed " + styleOK.Render(oneLine(c.ProposedValue, maxInt(8, width-12))) + "\n")

	if c.Conflict {
		builder.WriteString("  Conflict " + styleErr.Render("yes") + "\n")
	}

	builder.WriteString("\n" + styleSectionHead.Render("Evidence") + "\n")
	if c.EvidenceQuote != "" {
		builder.WriteString("  " + wrapText(oneLine(c.EvidenceQuote, width*3), maxInt(10, width-2)) + "\n")
	} else {
		builder.WriteString("  " + styleMutedText.Render("(none)") + "\n")
	}

	if c.Reason != "" {
		builder.WriteString("\n" + styleSectionHead.Render("Reason") + "\n")
		builder.WriteString("  " + wrapText(oneLine(c.Reason, width*2), maxInt(10, width-2)) + "\n")
	}

	builder.WriteString("\n" + styleSectionHead.Render("Meta") + "\n")
	if c.Confidence != "" {
		builder.WriteString("  Confidence " + c.Confidence + "\n")
	}
	if c.SourceKind != "" {
		builder.WriteString("  Source     " + c.SourceKind + "\n")
	}
	if c.SourceSession != "" {
		builder.WriteString("  Session    " + oneLine(c.SourceSession, 20) + "\n")
	}
	if c.ObservedAt != "" {
		builder.WriteString("  Observed   " + c.ObservedAt + "\n")
	}
	if c.DraftID != "" {
		builder.WriteString("  Draft      " + styleOK.Render(c.DraftID) + "\n")
	}
	if c.DedupKey != "" {
		builder.WriteString("  DedupKey   " + styleMutedText.Render(oneLine(c.DedupKey, maxInt(8, width-14))) + "\n")
	}

	builder.WriteString("\n")
	// Derive detail actions from the B-line Actions bundle (single source of truth)
	actions := c.Actions
	if actions.CanDraft {
		builder.WriteString(styleOK.Render("d") + "=draft ")
	}
	if actions.CanDismiss {
		builder.WriteString(styleErr.Render("x") + "=dismiss ")
	}
	if actions.CanRetry {
		builder.WriteString(styleOK.Render("r") + "=retry ")
	} else if actions.CanRecover && c.DraftID == "" {
		builder.WriteString(styleErr.Render("f") + "=force-dismiss ")
	} else if actions.CanRecover {
		builder.WriteString(styleWarn.Render("r") + "=recover ")
	}
	builder.WriteString(styleMutedText.Render("esc=back"))
	builder.WriteString("\n")

	return builder.String()
}

func renderCandidateStateBadge(state string) string {
	switch state {
	case "open":
		return styleWarn.Render("opn")
	case "drafted":
		return styleOK.Render("dft")
	case "dismissed":
		return styleMutedText.Render("dis")
	default:
		return styleMutedText.Render(state[:minInt(3, len(state))])
	}
}

func renderModelIdentityLine(b *strings.Builder, label string, identity *app.LLMIdentity, fallbackModel string) {
	if identity == nil || !identity.Enabled {
		b.WriteString("  " + styleMutedText.Render(fmt.Sprintf("%-8s (not configured)", label)) + "\n")
		return
	}
	model := identity.Model
	if model == "" && fallbackModel != "" {
		model = fallbackModel
	}
	if model == "" {
		model = "?"
	}
	source := string(identity.Source)
	profile := identity.Profile
	if profile != "" {
		source += ":" + profile
	}
	line := fmt.Sprintf("%-8s %s", label, styleOK.Render(model))
	if source != "" {
		line += " " + styleMutedText.Render("("+source+")")
	}
	// Key status indicator
	if identity.APIKeyEnv != "" {
		// We don't know key status from identity alone (it doesn't include the key),
		// but we show the env var name for user reference
	}
	b.WriteString("  " + oneLine(line, maxInt(20, 36)) + "\n")
}

func renderProfilePanel(profiles []ModelInfo, cursor int, editing bool, editText string, width int, height int) string {
	if len(profiles) == 0 {
		return styleMutedText.Render("No profiles configured.") + "\n" +
			styleMutedText.Render("Press n to create one from a preset.") + "\n" +
			styleMutedText.Render("Press esc to return to model list.")
	}

	var builder strings.Builder

	listHeight := height - 2
	if listHeight < 1 {
		listHeight = 1
	}

	start := cursor - listHeight/2
	if start < 0 {
		start = 0
	}
	end := start + listHeight
	if end > len(profiles) {
		end = len(profiles)
		start = maxInt(0, end-listHeight)
	}

	for i := start; i < end; i++ {
		p := profiles[i]
		prefix := "  "
		nameStyle := styleMutedText
		if i == cursor {
			prefix = styleWarn.Render(glyphFocus + " ")
			nameStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("#E5E7EB"))
		}

		activeMark := "  "
		if p.Active {
			activeMark = styleOK.Render("* ")
		}

		// Key status indicator
		keyMark := styleMutedText.Render("   ")
		if p.KeyStatus == "missing" {
			keyMark = styleErr.Render("!! ")
		} else if p.KeyStatus == "present" {
			keyMark = styleOK.Render("ok ")
		}

		line := prefix + activeMark + nameStyle.Render(p.ProfileName)
		if width > 50 {
			line += " " + styleMutedText.Render(p.Provider+"/"+p.Name)
		}
		if width > 64 {
			line += " " + keyMark
		}
		builder.WriteString(oneLine(line, maxInt(8, width-2)))
		builder.WriteString("\n")

		// Show missing key warning inline for the selected profile
		if i == cursor && p.KeyStatus == "missing" && p.APIKeyEnv != "" {
			builder.WriteString("  " + styleErr.Render(p.APIKeyEnv+" missing") + "\n")
		}
	}

	if editing {
		builder.WriteString(styleWarn.Render("profile> ") + editText)
		builder.WriteString("\n")
	}

	footer := fmt.Sprintf("[%d/%d] enter=use p=persist t=test n=new r=refresh esc=models", cursor+1, len(profiles))
	builder.WriteString(styleMutedText.Render(footer))
	builder.WriteString("\n")

	return builder.String()
}

func renderPresetPicker(presets []ModelProfilePreset, cursor int, width int, height int) string {
	if len(presets) == 0 {
		return styleMutedText.Render("No presets available.")
	}

	var builder strings.Builder

	listHeight := height - 2
	if listHeight < 1 {
		listHeight = 1
	}

	start := cursor - listHeight/2
	if start < 0 {
		start = 0
	}
	end := start + listHeight
	if end > len(presets) {
		end = len(presets)
		start = maxInt(0, end-listHeight)
	}

	for i := start; i < end; i++ {
		p := presets[i]
		prefix := "  "
		nameStyle := styleMutedText
		descStyle := styleMutedText
		if i == cursor {
			prefix = styleWarn.Render(glyphFocus + " ")
			nameStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("#E5E7EB"))
			descStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("#9CA3AF"))
		}

		line := prefix + nameStyle.Render(p.Name) + " " + descStyle.Render(oneLine(p.Description, maxInt(8, width-16)))
		builder.WriteString(oneLine(line, maxInt(8, width-2)))
		builder.WriteString("\n")

		// Show details for selected preset
		if i == cursor {
			detailLine := "  " + styleMutedText.Render(p.Provider+" "+p.Model+" @ "+p.BaseURL)
			if p.APIKeyEnv != "" {
				detailLine += " " + styleMutedText.Render("key:"+p.APIKeyEnv)
			}
			builder.WriteString(oneLine(detailLine, maxInt(8, width-2)))
			builder.WriteString("\n")
		}
	}

	footer := fmt.Sprintf("[%d/%d] enter=create esc=back", cursor+1, len(presets))
	builder.WriteString(styleMutedText.Render(footer))
	builder.WriteString("\n")

	return builder.String()
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

	footer := fmt.Sprintf("[%d/%d] enter=detail", cursor+1, len(findings))
	if cursor < len(findings) && findings[cursor].State == model.FindingOpen {
		footer += " x=resolve i=ignore"
	}
	builder.WriteString(styleMutedText.Render(footer))
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

	// "What does this change?" summary header
	builder.WriteString(styleSectionHead.Render("Draft Review") + "\n")
	builder.WriteString("  Target " + oneLine(draft.Target.Path, maxInt(10, width-10)) + "\n")
	builder.WriteString("  Kind   " + renderDraftKindBadge(draft.Kind) + " " + styleMutedText.Render(string(draft.State)) + "\n")

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

	// Dynamic action footer based on state
	builder.WriteString("\n")
	switch draft.State {
	case model.DraftPendingReview:
		builder.WriteString(styleOK.Render("a") + "=approve " + styleErr.Render("r") + "=reject " + styleMutedText.Render("esc=back"))
	case model.DraftApproved:
		builder.WriteString(styleOK.Render("p") + "=apply (writes to file) " + styleMutedText.Render("esc=back"))
	case model.DraftRejected:
		builder.WriteString(styleMutedText.Render("rejected — candidate can retry/dismiss") + " esc=back")
	case model.DraftApplied:
		builder.WriteString(styleOK.Render("applied") + " → written to " + oneLine(draft.Target.Path, maxInt(8, width-20)) + " esc=back")
	default:
		builder.WriteString(styleMutedText.Render("esc=back"))
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

	b.WriteString("\n" + styleSectionHead.Render("Persona Change") + "\n")
	if prop.Field != "" {
		b.WriteString("  Field " + styleWarn.Render(prop.Field) + "\n")
	}
	// Old → New diff display
	b.WriteString("  Old   " + styleMutedText.Render(oneLine(prop.CurrentValue, maxInt(8, width-10))) + "\n")
	b.WriteString("  New   " + styleOK.Render(oneLine(prop.ProposedValue, maxInt(8, width-10))) + "\n")

	if prop.Confidence != "" || prop.Source != "" {
		b.WriteString("\n")
		if prop.Confidence != "" {
			b.WriteString("  Confidence " + prop.Confidence + "\n")
		}
		if prop.Source != "" {
			b.WriteString("  Source     " + styleMutedText.Render(oneLine(prop.Source, maxInt(8, width-14))) + "\n")
		}
	}

	if prop.Evidence != "" {
		b.WriteString("\n" + styleSectionHead.Render("Evidence") + "\n")
		b.WriteString("  " + wrapText(oneLine(prop.Evidence, width*2), maxInt(10, width-2)) + "\n")
	}
	if prop.Reason != "" {
		b.WriteString("\n" + styleSectionHead.Render("Reason") + "\n")
		b.WriteString("  " + wrapText(oneLine(prop.Reason, width*2), maxInt(10, width-2)) + "\n")
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
