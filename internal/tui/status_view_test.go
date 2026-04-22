package tui

import (
	"strings"
	"testing"

	"obsidian-harness/internal/app"
)

func TestRenderStatusIncludesCoreFields(t *testing.T) {
	view := RenderStatus(app.Status{
		Name:          "obsidian-harness",
		Version:       "0.1.0",
		State:         "ready",
		ViewMode:      "text",
		VaultPath:     `C:\vault`,
		ActiveProfile: "default",
		Transport:     "local",
	})

	for _, expected := range []string{
		"Obsidian Harness",
		"0.1.0",
		"ready",
		"text",
		`C:\vault`,
		"default",
		"local",
	} {
		if !strings.Contains(view, expected) {
			t.Fatalf("expected rendered view to contain %q, got %q", expected, view)
		}
	}
}
