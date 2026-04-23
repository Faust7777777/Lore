package tui

import (
	"fmt"
	"sort"
	"strings"

	"obsidian-harness/internal/app"
	"obsidian-harness/internal/model"
	"obsidian-harness/internal/operatoragent"
)

func RenderWorkbench(version string, managed model.ManagedStatusView, drafts []model.Draft, processSink app.ProcessSinkDayView, focusedReview *app.DraftReview, toolTrace []operatoragent.ToolCallTrace, localExec bool, shellEnabled bool, lastOutput string) string {
	var builder strings.Builder
	pendingDrafts := filterDraftsByState(drafts, model.DraftPendingReview)
	builder.WriteString("Lore Workbench\n")
	builder.WriteString("==============\n")
	builder.WriteString("Live view for managed knowledge ops, draft review, and process-sink monitoring.\n")

	builder.WriteString("\nRuntime Snapshot\n")
	builder.WriteString("----------------\n")
	writeField(&builder, "Version", version)
	writeField(&builder, "Ready", yesNo(managed.Ready))
	writeField(&builder, "Profile", profileLabel(localExec))
	writeField(&builder, "Shell", enabledDisabled(shellEnabled))
	writeField(&builder, "Drafts", fmt.Sprintf("%d pending / %d total", len(pendingDrafts), len(drafts)))
	writeField(&builder, "Checkpoints", fmt.Sprintf("%d", len(processSink.Checkpoints)))
	writeField(&builder, "WorkDir", managed.WorkDir)
	writeField(&builder, "Vault", managed.VaultRoot)
	writeField(&builder, "Health", string(managed.Health.Outcome.Status))
	writeField(&builder, "Message", managed.Health.Message)
	writeField(&builder, "Agent", processSink.AgentID)
	writeField(&builder, "Day", processSink.Day.Format("2006-01-02"))
	if processSink.Report != nil {
		writeField(&builder, "Daily Report", processSink.Report.Path)
	} else {
		writeField(&builder, "Daily Report", "missing")
	}

	builder.WriteString("\nManaged Core\n")
	builder.WriteString("------------\n")
	fmt.Fprintf(&builder, "%-10s %-8s %s\n", "DOC", "STATE", "PATH")
	for _, doc := range managed.CoreDocs {
		state := "missing"
		if doc.Exists {
			state = "ready"
		}
		fmt.Fprintf(&builder, "%-10s %-8s %s\n", doc.Name, state, oneLine(doc.Path, 72))
	}

	builder.WriteString("\nPending Drafts\n")
	builder.WriteString("--------------\n")
	if len(pendingDrafts) == 0 {
		builder.WriteString("No pending drafts.\n")
	} else {
		fmt.Fprintf(&builder, "%-22s %-18s %-24s %s\n", "ID", "KIND", "TARGET", "TITLE")
		for _, draft := range pendingDrafts {
			fmt.Fprintf(
				&builder,
				"%-22s %-18s %-24s %s\n",
				shortID(draft.ID, 22),
				string(draft.Kind),
				shortID(draft.Target.Path, 24),
				oneLine(draft.Title, 56),
			)
		}
	}

	builder.WriteString("\nFocused Draft\n")
	builder.WriteString("-------------\n")
	if focusedReview == nil {
		builder.WriteString("No focused draft.\n")
		builder.WriteString("Hint: ask Lore to review the pending draft.\n")
	} else {
		writeField(&builder, "ID", focusedReview.Draft.ID)
		writeField(&builder, "State", string(focusedReview.Draft.State))
		writeField(&builder, "Target", focusedReview.Draft.Target.Path)
		writeField(&builder, "Base Match", yesNo(focusedReview.BaseVersionMatches))
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

	builder.WriteString("\nProcess Sink\n")
	builder.WriteString("------------\n")
	if len(processSink.Checkpoints) == 0 {
		builder.WriteString("No checkpoints.\n")
	} else {
		fmt.Fprintf(&builder, "%-13s %-14s %-12s %s\n", "WINDOW", "STATE", "SESSION", "TITLE")
		for _, checkpoint := range processSink.Checkpoints {
			fmt.Fprintf(
				&builder,
				"%-13s %-14s %-12s %s\n",
				checkpoint.Window.WindowStart.Format("15:04")+"-"+checkpoint.Window.WindowEnd.Format("15:04"),
				string(checkpoint.State),
				oneLine(checkpoint.Window.SessionID, 12),
				oneLine(checkpoint.Title, 60),
			)
		}
	}

	builder.WriteString("\nTool Trace\n")
	builder.WriteString("----------\n")
	if len(toolTrace) == 0 {
		builder.WriteString("No tool calls in the last turn.\n")
	} else {
		fmt.Fprintf(&builder, "%-18s %-8s %-34s %s\n", "TOOL", "STATUS", "ARGUMENTS", "ERROR")
		for _, item := range toolTrace {
			fmt.Fprintf(
				&builder,
				"%-18s %-8s %-34s %s\n",
				oneLine(item.Name, 18),
				oneLine(item.Status, 8),
				oneLine(formatToolArguments(item.Arguments), 34),
				oneLine(item.Error, 64),
			)
		}
	}

	builder.WriteString("\nLast Action\n")
	builder.WriteString("-----------\n")
	if strings.TrimSpace(lastOutput) == "" {
		builder.WriteString("No action yet.\n")
	} else {
		builder.WriteString("Most recent agent-visible output:\n")
		builder.WriteString(indentBlock(strings.TrimSpace(lastOutput), "  "))
		builder.WriteString("\n")
	}

	builder.WriteString("\nControls\n")
	builder.WriteString("--------\n")
	builder.WriteString("Type a natural language request.\n")
	builder.WriteString("Special commands: /refresh, /quit\n")
	builder.WriteString("Examples:\n")
	builder.WriteString("  show status\n")
	builder.WriteString("  list pending drafts\n")
	builder.WriteString("  review the pending draft\n")
	builder.WriteString("  approve it\n")
	builder.WriteString("  apply it\n")
	builder.WriteString("  show codex daily report today\n")

	return builder.String()
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
