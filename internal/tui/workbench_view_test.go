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
		[]operatoragent.ConversationTurn{
			{Role: "user", Content: "show me current status"},
			{Role: "assistant", Content: "Managed Status\n============\nReady: yes"},
		},
		true,
		false,
		"Managed Status\n============\nReady: yes",
	)

	for _, expected := range []string{
		"Lore",
		"chat:lore",
		"Managed Core",
		"Pending Drafts",
		"Focused Draft",
		"Process Sink",
		"Tool Trace",
		"STEP",
		"managed_status",
		"Conversation Lane",
		"USER",
		"ASSISTANT",
		"Quick Actions",
		"show git status for the local repo",
		"Runtime Snapshot",
		"Profile",
		"local-exec",
		"Git",
		"enabled",
		"Shell",
		"disabled",
		"morning checkpoint",
		"pending draft",
		"sync the weekly progress table",
		"approve it",
		"show me current status",
		"Ready: yes",
	} {
		if !strings.Contains(view, expected) {
			t.Fatalf("expected workbench to contain %q, got %q", expected, view)
		}
	}
}

func TestNewWorkbenchViewModelFiltersPendingDraftsAndPreservesConversation(t *testing.T) {
	day := time.Date(2026, 4, 23, 12, 0, 0, 0, time.Local)
	viewModel := NewWorkbenchViewModel(
		"test",
		model.ManagedStatusView{
			Ready:   true,
			WorkDir: "work",
			Health:  model.HealthSnapshot{Outcome: model.NewOutcome(model.StatusOK), Message: "healthy"},
		},
		[]model.Draft{
			{ID: "draft-1", State: model.DraftPendingReview},
			{ID: "draft-2", State: model.DraftApproved},
		},
		app.ProcessSinkDayView{AgentID: "codex", Day: day},
		nil,
		nil,
		nil,
		[]operatoragent.ConversationTurn{
			{Role: "user", Content: "hello"},
			{Role: "assistant", Content: "world"},
		},
		false,
		false,
		"world",
		"session-1",
		"state/sessions/session-1.jsonl",
	)

	if got, want := len(viewModel.PendingDrafts), 2; got != want {
		t.Fatalf("len(viewModel.PendingDrafts) = %d, want %d (pending_review + approved)", got, want)
	}
	if got, want := len(viewModel.Conversation.Turns), 2; got != want {
		t.Fatalf("len(viewModel.Conversation.Turns) = %d, want %d", got, want)
	}
	if got, want := viewModel.Snapshot.PendingDrafts, 2; got != want {
		t.Fatalf("viewModel.Snapshot.PendingDrafts = %d, want %d (pending_review + approved)", got, want)
	}
	if got, want := viewModel.Conversation.LastOutput, "world"; got != want {
		t.Fatalf("viewModel.Conversation.LastOutput = %q, want %q", got, want)
	}
}
