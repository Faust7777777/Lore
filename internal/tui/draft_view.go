package tui

import (
	"fmt"
	"strings"

	"obsidian-harness/internal/app"
	"obsidian-harness/internal/model"
)

func RenderDraftList(drafts []model.Draft) string {
	var builder strings.Builder

	builder.WriteString("Draft Inbox\n")
	builder.WriteString("==========\n")
	if len(drafts) == 0 {
		builder.WriteString("No drafts.\n")
		return builder.String()
	}

	fmt.Fprintf(&builder, "%-22s %-18s %-18s %-24s %s\n", "ID", "STATE", "KIND", "TARGET", "TITLE")
	for _, draft := range drafts {
		fmt.Fprintf(
			&builder,
			"%-22s %-18s %-18s %-24s %s\n",
			shortID(draft.ID, 22),
			string(draft.State),
			string(draft.Kind),
			shortID(draft.Target.Path, 24),
			oneLine(draft.Title, 56),
		)
	}
	builder.WriteString("\n")
	builder.WriteString("Next:\n")
	builder.WriteString("  lore draft review <id>\n")
	builder.WriteString("  lore draft approve <id>\n")
	builder.WriteString("  lore draft reject <id>\n")
	builder.WriteString("  lore draft request-revision <id>\n")
	builder.WriteString("  lore draft apply <id>\n")

	return builder.String()
}

func RenderDraftReview(review app.DraftReview) string {
	var builder strings.Builder

	builder.WriteString("Draft Review\n")
	builder.WriteString("============\n")
	writeField(&builder, "ID", review.Draft.ID)
	writeField(&builder, "State", string(review.Draft.State))
	writeField(&builder, "Kind", string(review.Draft.Kind))
	writeField(&builder, "Target", review.Draft.Target.Path)
	writeField(&builder, "Target Class", string(review.Draft.Target.Class))
	writeField(&builder, "Base Match", yesNo(review.BaseVersionMatches))
	if review.CurrentBaseVersion != "" {
		writeField(&builder, "Current Base", review.CurrentBaseVersion)
	}
	if len(review.Draft.EvidenceRefs) > 0 {
		writeField(&builder, "Evidence", strings.Join(review.Draft.EvidenceRefs, ", "))
	}

	builder.WriteString("\nSummary\n")
	builder.WriteString("-------\n")
	builder.WriteString(strings.TrimSpace(review.Draft.Summary))
	builder.WriteString("\n\n")

	builder.WriteString("Proposed Patch\n")
	builder.WriteString("--------------\n")
	builder.WriteString(strings.TrimSpace(review.Draft.ProposedContent))
	builder.WriteString("\n")

	if review.TargetDocument != nil {
		builder.WriteString("\nCurrent Target Excerpt\n")
		builder.WriteString("----------------------\n")
		builder.WriteString(strings.TrimSpace(excerpt(review.TargetDocument.Content, 800)))
		builder.WriteString("\n")
	}

	builder.WriteString("\nActions\n")
	builder.WriteString("-------\n")
	builder.WriteString("  lore draft approve " + review.Draft.ID + "\n")
	builder.WriteString("  lore draft reject " + review.Draft.ID + "\n")
	builder.WriteString("  lore draft request-revision " + review.Draft.ID + "\n")
	if review.Draft.State == model.DraftApproved {
		builder.WriteString("  lore draft apply " + review.Draft.ID + "\n")
	}

	return builder.String()
}

func RenderDraftActionResult(action string, draft model.Draft) string {
	var builder strings.Builder

	builder.WriteString("Draft Updated\n")
	builder.WriteString("=============\n")
	writeField(&builder, "Action", action)
	writeField(&builder, "ID", draft.ID)
	writeField(&builder, "State", string(draft.State))
	writeField(&builder, "Target", draft.Target.Path)
	writeField(&builder, "Title", draft.Title)

	if draft.State == model.DraftApproved {
		builder.WriteString("\nNext:\n")
		builder.WriteString("  lore draft apply " + draft.ID + "\n")
	}

	return builder.String()
}

func shortID(value string, limit int) string {
	value = oneLine(value, limit)
	if len(value) <= limit {
		return value
	}
	return value[:limit]
}

func oneLine(value string, limit int) string {
	value = strings.Join(strings.Fields(strings.TrimSpace(value)), " ")
	if limit > 0 && len(value) > limit {
		return value[:limit-3] + "..."
	}
	return value
}

func excerpt(value string, limit int) string {
	value = strings.TrimSpace(value)
	if limit > 0 && len(value) > limit {
		return value[:limit] + "..."
	}
	return value
}

func yesNo(value bool) string {
	if value {
		return "yes"
	}
	return "no"
}
