package tui

import (
	"strings"
	"testing"
	"time"

	"obsidian-harness/internal/app"
	"obsidian-harness/internal/model"
)

func TestRenderProcessSinkDayIncludesWindowsAndReport(t *testing.T) {
	day := time.Date(2026, 4, 22, 0, 0, 0, 0, time.UTC)
	view := RenderProcessSinkDay(app.ProcessSinkDayView{
		AgentID: "codex",
		Day:     day,
		Checkpoints: []model.CheckpointDoc{{
			Window: model.SessionWindow{
				AgentID:     "codex",
				SessionID:   "session-1",
				WindowStart: day.Add(9 * time.Hour),
				WindowEnd:   day.Add(9*time.Hour + 30*time.Minute),
			},
			State: model.CheckpointMaterialized,
			Title: "morning checkpoint",
		}},
		Report: &model.DailyReport{
			Path:    "09-process-sink/codex/daily/2026-04-22.md",
			Title:   "daily rollup",
			Content: "- 09:00 -> morning checkpoint",
		},
	})

	for _, expected := range []string{
		"Process Sink Day",
		"codex",
		"09:00-09:30",
		"morning checkpoint",
		"09-process-sink",
		"daily rollup",
		"Latest Title",
	} {
		if !strings.Contains(view, expected) {
			t.Fatalf("expected view to contain %q, got %q", expected, view)
		}
	}
}
