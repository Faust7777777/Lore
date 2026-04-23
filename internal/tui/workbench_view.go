package tui

import (
	"fmt"
	"strings"

	"obsidian-harness/internal/app"
	"obsidian-harness/internal/model"
)

func RenderWorkbench(version string, managed model.ManagedStatusView, drafts []model.Draft, processSink app.ProcessSinkDayView, lastOutput string) string {
	var builder strings.Builder

	builder.WriteString("Obsidian Harness Workbench\n")
	builder.WriteString("==========================\n")
	writeField(&builder, "Version", version)
	writeField(&builder, "WorkDir", managed.WorkDir)
	writeField(&builder, "Vault", managed.VaultRoot)
	writeField(&builder, "Health", string(managed.Health.Outcome.Status))
	writeField(&builder, "Message", managed.Health.Message)
	writeField(&builder, "Agent", processSink.AgentID)
	writeField(&builder, "Day", processSink.Day.Format("2006-01-02"))

	builder.WriteString("\nManaged Core\n")
	builder.WriteString("------------\n")
	for _, doc := range managed.CoreDocs {
		state := "missing"
		if doc.Exists {
			state = "ready"
		}
		fmt.Fprintf(&builder, "%-10s %-8s %s\n", doc.Name, state, oneLine(doc.Path, 72))
	}

	builder.WriteString("\nPending Drafts\n")
	builder.WriteString("--------------\n")
	pendingDrafts := filterDraftsByState(drafts, model.DraftPendingReview)
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

	builder.WriteString("\nProcess Sink\n")
	builder.WriteString("------------\n")
	writeField(&builder, "Checkpoints", fmt.Sprintf("%d", len(processSink.Checkpoints)))
	if processSink.Report != nil {
		writeField(&builder, "Daily Report", processSink.Report.Path)
	} else {
		writeField(&builder, "Daily Report", "missing")
	}
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

	builder.WriteString("\nLast Action\n")
	builder.WriteString("-----------\n")
	if strings.TrimSpace(lastOutput) == "" {
		builder.WriteString("No action yet.\n")
	} else {
		builder.WriteString(indentBlock(strings.TrimSpace(lastOutput), "  "))
		builder.WriteString("\n")
	}

	builder.WriteString("\nControls\n")
	builder.WriteString("--------\n")
	builder.WriteString("Type a natural language request.\n")
	builder.WriteString("Special commands: /refresh, /quit\n")

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
