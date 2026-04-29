package tui

import (
	"errors"
	"strings"
	"testing"

	"obsidian-harness/internal/config"
)

func TestRenderConfigLayersEmpty(t *testing.T) {
	if got := RenderConfigLayers(nil); got != "" {
		t.Errorf("expected empty output for nil diagnostics, got %q", got)
	}
	if got := RenderConfigLayers([]config.LoadDiagnostic{}); got != "" {
		t.Errorf("expected empty output for empty diagnostics, got %q", got)
	}
}

func TestRenderConfigLayersIncludesAllLayers(t *testing.T) {
	diagnostics := []config.LoadDiagnostic{
		{Source: config.LayerDefault, Status: config.LayerStatusLoaded},
		{Source: config.LayerUserGlobal, Status: config.LayerStatusMissing, Path: "/home/user/.lore/config.json"},
		{Source: config.LayerWorkspace, Status: config.LayerStatusLoaded, Path: "/tmp/workspace/.lore/config.json"},
	}
	got := RenderConfigLayers(diagnostics)

	if !strings.Contains(got, "Config Layers") {
		t.Errorf("missing section header: %q", got)
	}
	if !strings.Contains(got, "default (loaded)") {
		t.Errorf("missing default loaded line: %q", got)
	}
	if !strings.Contains(got, "user_global (missing) /home/user/.lore/config.json") {
		t.Errorf("missing user-global missing line: %q", got)
	}
	if !strings.Contains(got, "workspace (loaded) /tmp/workspace/.lore/config.json") {
		t.Errorf("missing workspace loaded line: %q", got)
	}
}

func TestRenderConfigLayersIncludesError(t *testing.T) {
	diagnostics := []config.LoadDiagnostic{
		{Source: config.LayerDefault, Status: config.LayerStatusLoaded},
		{
			Source: config.LayerUserGlobal,
			Status: config.LayerStatusError,
			Path:   "/tmp/broken.json",
			Err:    errors.New("unexpected end of JSON input"),
		},
	}
	got := RenderConfigLayers(diagnostics)
	if !strings.Contains(got, "user_global (error) /tmp/broken.json") {
		t.Errorf("missing error line prefix: %q", got)
	}
	if !strings.Contains(got, "err=unexpected end of JSON input") {
		t.Errorf("missing error message: %q", got)
	}
}
