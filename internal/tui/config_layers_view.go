package tui

import (
	"strings"

	"obsidian-harness/internal/config"
)

// RenderConfigLayers formats the layered config diagnostics as a small section
// suitable for appending to the CLI status output. Returns an empty string
// when no diagnostics were collected.
func RenderConfigLayers(diagnostics []config.LoadDiagnostic) string {
	if len(diagnostics) == 0 {
		return ""
	}
	var builder strings.Builder
	builder.WriteString("\nConfig Layers\n")
	builder.WriteString("-------------\n")
	for _, diag := range diagnostics {
		line := string(diag.Source) + " (" + string(diag.Status) + ")"
		if diag.Path != "" {
			line += " " + diag.Path
		}
		if diag.Err != nil {
			line += " err=" + diag.Err.Error()
		}
		builder.WriteString(line + "\n")
	}
	return builder.String()
}
