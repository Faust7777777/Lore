package tui

import (
	"strings"
	"testing"
	"time"

	"obsidian-harness/internal/model"
)

func TestRenderManagedStatusIncludesCoreDocsAndHealth(t *testing.T) {
	view := RenderManagedStatus("test", model.ManagedStatusView{
		Ready:     true,
		WorkDir:   `C:\work`,
		VaultRoot: `C:\work\vault`,
		CoreDocs: []model.ManagedCoreStatus{
			{Name: "system", Exists: true, Path: "00-系统/系统说明.md"},
			{Name: "progress", Exists: true, Path: "0-排期/00-系统/文档进度总表.md"},
		},
		Health: model.HealthSnapshot{
			Outcome:   model.NewOutcome(model.StatusOK),
			Message:   "healthy",
			CheckedAt: time.Now(),
		},
	})

	for _, expected := range []string{
		"Managed Status",
		"healthy",
		"system",
		"progress",
		`C:\work\vault`,
	} {
		if !strings.Contains(view, expected) {
			t.Fatalf("expected view to contain %q, got %q", expected, view)
		}
	}
}
