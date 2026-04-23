package tui

import (
	"strings"
	"testing"
	"time"

	"obsidian-harness/internal/app"
	"obsidian-harness/internal/model"
	"obsidian-harness/internal/operatoragent"
)

func TestRenderWorkbenchIncludesCorePanels(t *testing.T) {
	day := time.Date(2026, 4, 23, 12, 0, 0, 0, time.Local)
	view := RenderWorkbench(
		"test",
		model.ManagedStatusView{
			Ready:     true,
			WorkDir:   "work",
			VaultRoot: "vault",
			Health:    model.HealthSnapshot{Outcome: model.NewOutcome(model.StatusOK), Message: "healthy"},
			CoreDocs: []model.ManagedCoreStatus{
				{Name: "system", Exists: true, Path: "00-system/system.md"},
			},
		},
		[]model.Draft{{
			ID:    "draft-1",
			State: model.DraftPendingReview,
			Kind:  model.DraftKindProgressSync,
			Target: model.DocumentRef{
				Path: "progress.md",
			},
			Title: "pending draft",
		}},
		app.ProcessSinkDayView{
			AgentID: "codex",
			Day:     day,
			Checkpoints: []model.CheckpointDoc{{
				Window: model.SessionWindow{
					AgentID:     "codex",
					SessionID:   "session-1",
					WindowStart: day.Truncate(24 * time.Hour).Add(9 * time.Hour),
					WindowEnd:   day.Truncate(24 * time.Hour).Add(9*time.Hour + 30*time.Minute),
				},
				State: model.CheckpointMaterialized,
				Title: "morning checkpoint",
			}},
		},
		&app.DraftReview{
			Draft: model.Draft{
				ID:              "draft-1",
				State:           model.DraftPendingReview,
				Kind:            model.DraftKindProgressSync,
				Title:           "pending draft",
				Summary:         "sync the weekly progress table",
				ProposedContent: "- [x] done",
				Target: model.DocumentRef{
					Path: "progress.md",
				},
			},
			BaseVersionMatches: true,
		},
		[]operatoragent.ToolCallTrace{{
			Name:      "managed_status",
			Status:    "ok",
			Arguments: map[string]any{},
		}},
		true,
		false,
		"Managed Status\n============\nReady: yes",
	)

	for _, expected := range []string{
		"Lore",
		"Managed Core",
		"Pending Drafts",
		"Focused Draft",
		"Process Sink",
		"Tool Trace",
		"managed_status",
		"Last Action",
		"Runtime Snapshot",
		"Profile",
		"local-exec",
		"Shell",
		"disabled",
		"morning checkpoint",
		"pending draft",
		"sync the weekly progress table",
		"approve it",
		"Managed Status",
	} {
		if !strings.Contains(view, expected) {
			t.Fatalf("expected workbench to contain %q, got %q", expected, view)
		}
	}
}
