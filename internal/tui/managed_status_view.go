package tui

import (
	"fmt"
	"strings"

	"obsidian-harness/internal/model"
)

func RenderManagedStatus(version string, managed model.ManagedStatusView) string {
	var builder strings.Builder

	builder.WriteString("Managed Status\n")
	builder.WriteString("==============\n")
	writeField(&builder, "Version", version)
	writeField(&builder, "Ready", yesNo(managed.Ready))
	writeField(&builder, "WorkDir", managed.WorkDir)
	writeField(&builder, "Vault", managed.VaultRoot)
	writeField(&builder, "Health", string(managed.Health.Outcome.Status))
	writeField(&builder, "Message", managed.Health.Message)

	builder.WriteString("\nCore Docs\n")
	builder.WriteString("---------\n")
	for _, doc := range managed.CoreDocs {
		state := "missing"
		if doc.Exists {
			state = "ready"
		}
		fmt.Fprintf(&builder, "%-10s %-8s %s\n", doc.Name, state, doc.Path)
	}

	if len(managed.Health.Outcome.ReasonCodes) > 0 {
		builder.WriteString("\nReason Codes\n")
		builder.WriteString("------------\n")
		for _, code := range managed.Health.Outcome.ReasonCodes {
			builder.WriteString("- " + string(code) + "\n")
		}
	}

	return builder.String()
}
