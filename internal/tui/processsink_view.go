package tui

import (
	"fmt"
	"strings"

	"obsidian-harness/internal/app"
)

func RenderProcessSinkDay(view app.ProcessSinkDayView) string {
	var builder strings.Builder

	builder.WriteString("Process Sink Day\n")
	builder.WriteString("================\n")
	writeField(&builder, "Agent", view.AgentID)
	writeField(&builder, "Day", view.Day.Format("2006-01-02"))
	writeField(&builder, "Checkpoints", fmt.Sprintf("%d", len(view.Checkpoints)))
	if view.Report != nil {
		writeField(&builder, "Daily Report", view.Report.Path)
		writeField(&builder, "Report Title", oneLine(view.Report.Title, 72))
	} else {
		writeField(&builder, "Daily Report", "missing")
	}
	if len(view.Checkpoints) > 0 {
		latest := view.Checkpoints[len(view.Checkpoints)-1]
		writeField(&builder, "Latest Window", latest.Window.WindowStart.Format("15:04")+"-"+latest.Window.WindowEnd.Format("15:04"))
		writeField(&builder, "Latest Title", oneLine(latest.Title, 72))
	}

	if view.Report != nil && strings.TrimSpace(view.Report.Content) != "" {
		builder.WriteString("\nDaily Report Preview\n")
		builder.WriteString("--------------------\n")
		builder.WriteString(strings.TrimSpace(excerpt(view.Report.Content, 280)))
		builder.WriteString("\n")
	}

	builder.WriteString("\nWindows\n")
	builder.WriteString("-------\n")
	if len(view.Checkpoints) == 0 {
		builder.WriteString("No checkpoints.\n")
		return builder.String()
	}
	fmt.Fprintf(&builder, "%-13s %-14s %-12s %s\n", "WINDOW", "STATE", "SESSION", "TITLE")
	for _, checkpoint := range view.Checkpoints {
		fmt.Fprintf(
			&builder,
			"%-13s %-14s %-12s %s\n",
			checkpoint.Window.WindowStart.Format("15:04")+"-"+checkpoint.Window.WindowEnd.Format("15:04"),
			string(checkpoint.State),
			oneLine(checkpoint.Window.SessionID, 12),
			oneLine(checkpoint.Title, 60),
		)
	}

	return builder.String()
}
