package tui

import (
	"strings"
	"testing"
	"time"

	"obsidian-harness/internal/app"
	"obsidian-harness/internal/model"
)

func TestRenderDraftListIncludesDraftAndHints(t *testing.T) {
	view := RenderDraftList([]model.Draft{{
		ID:    "draft-1",
		Kind:  model.DraftKindProgressSync,
		State: model.DraftPendingReview,
		Target: model.DocumentRef{
			Path: "0-排期/00-系统/文档进度总表.md",
		},
		Title: "Progress sync for week.md",
	}})

	for _, expected := range []string{
		"Draft Inbox",
		"draft-1",
		"pending_review",
		"obsidian-harness draft review <id>",
	} {
		if !strings.Contains(view, expected) {
			t.Fatalf("expected view to contain %q, got %q", expected, view)
		}
	}
}

func TestRenderDraftReviewIncludesPatchAndActions(t *testing.T) {
	review := app.DraftReview{
		Draft: model.Draft{
			ID:    "draft-2",
			Kind:  model.DraftKindProgressSync,
			State: model.DraftApproved,
			Target: model.DocumentRef{
				Path:        "0-排期/00-系统/文档进度总表.md",
				Class:       model.DocClassProgressIndex,
				BaseVersion: "base-1",
			},
			Summary:         "Sync latest managed progress.",
			ProposedContent: "## Auto Progress Sync",
			EvidenceRefs:    []string{"0-排期/04-执行/week.md"},
			CreatedAt:       time.Now(),
			UpdatedAt:       time.Now(),
		},
		CurrentBaseVersion: "base-1",
		BaseVersionMatches: true,
		TargetDocument: &model.VaultDocument{
			Path:        "0-排期/00-系统/文档进度总表.md",
			BaseVersion: "base-1",
			Content:     "# Progress\n\nExisting content",
		},
	}

	view := RenderDraftReview(review)
	for _, expected := range []string{
		"Draft Review",
		"Base Match",
		"## Auto Progress Sync",
		"Current Target Excerpt",
		"obsidian-harness draft apply draft-2",
	} {
		if !strings.Contains(view, expected) {
			t.Fatalf("expected view to contain %q, got %q", expected, view)
		}
	}
}
