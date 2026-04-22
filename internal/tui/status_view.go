package tui

import (
	"fmt"
	"strings"

	"obsidian-harness/internal/app"
)

func RenderStatus(status app.Status) string {
	var builder strings.Builder

	builder.WriteString("Obsidian Harness\n")
	builder.WriteString("================\n")
	builder.WriteString("Minimal CLI/TUI bootstrap is wired.\n\n")
	writeField(&builder, "Name", status.Name)
	writeField(&builder, "Version", status.Version)
	writeField(&builder, "State", status.State)
	writeField(&builder, "View", status.ViewMode)
	writeField(&builder, "Vault", status.VaultPath)
	writeField(&builder, "Profile", status.ActiveProfile)
	writeField(&builder, "Transport", status.Transport)

	return builder.String()
}

func writeField(builder *strings.Builder, label string, value string) {
	fmt.Fprintf(builder, "%-10s %s\n", label+":", value)
}
