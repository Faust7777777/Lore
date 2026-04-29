package orchestrator

import (
	"fmt"
	"sort"
	"strings"

	"obsidian-harness/internal/model"
)

const (
	coreContextDefaultLimit = 6
	coreContextFieldLimit   = 1200
	coreContextSectionLimit = 700
	coreContextDraftLimit   = 220
)

func (h *Harness) BuildCoreContext(limit int) (model.CoreContext, error) {
	if limit <= 0 {
		limit = coreContextDefaultLimit
	}
	ctx := model.CoreContext{}

	persona, err := h.SystemDocGet("persona")
	if err != nil {
		ctx.Notes = append(ctx.Notes, "persona document could not be loaded: "+err.Error())
	} else {
		ctx.PersonaSummary = buildPersonaSummary(persona.Content, &ctx)
		ctx.WeaknessSummary = excerptRunes(markdownSection(persona.Content, "Weaknesses"), coreContextSectionLimit)
		if strings.TrimSpace(ctx.WeaknessSummary) == "" {
			ctx.Notes = append(ctx.Notes, "persona section ## Weaknesses is missing or empty")
		}
	}

	systemDoc, err := h.SystemDocGet("system")
	if err != nil {
		ctx.Notes = append(ctx.Notes, "system document could not be loaded: "+err.Error())
	} else {
		ctx.SystemRulesSummary = excerptRunes(systemDoc.Content, coreContextFieldLimit)
	}

	progress, err := h.SystemDocGet("progress")
	if err != nil {
		ctx.Notes = append(ctx.Notes, "progress document could not be loaded: "+err.Error())
	} else {
		ctx.ProgressSummary = excerptRunes(progress.Content, coreContextFieldLimit)
	}

	drafts, err := h.ListDrafts()
	if err != nil {
		return model.CoreContext{}, err
	}
	ctx.PendingDrafts = summarizePendingDrafts(drafts, limit)
	return ctx, nil
}

func buildPersonaSummary(content string, ctx *model.CoreContext) string {
	sections := []struct {
		heading string
		label   string
	}{
		{heading: "Identity / Stable Profile", label: "Identity / Stable Profile"},
		{heading: "Current State", label: "Current State"},
	}
	parts := make([]string, 0, len(sections))
	for _, section := range sections {
		body := excerptRunes(markdownSection(content, section.heading), coreContextSectionLimit)
		if strings.TrimSpace(body) == "" {
			if ctx != nil {
				ctx.Notes = append(ctx.Notes, "persona section ## "+section.heading+" is missing or empty")
			}
			continue
		}
		parts = append(parts, section.label+":\n"+body)
	}
	if len(parts) == 0 {
		return excerptRunes(content, coreContextFieldLimit)
	}
	return excerptRunes(strings.Join(parts, "\n\n"), coreContextFieldLimit)
}

func summarizePendingDrafts(drafts []model.Draft, limit int) []string {
	if len(drafts) == 0 || limit <= 0 {
		return nil
	}
	filtered := make([]model.Draft, 0, len(drafts))
	for _, draft := range drafts {
		if draft.State == model.DraftPendingReview || draft.State == model.DraftApproved {
			filtered = append(filtered, draft)
		}
	}
	sort.SliceStable(filtered, func(i, j int) bool {
		return filtered[i].UpdatedAt.After(filtered[j].UpdatedAt)
	})
	if len(filtered) > limit {
		filtered = filtered[:limit]
	}
	out := make([]string, 0, len(filtered))
	for _, draft := range filtered {
		summary := fmt.Sprintf("%s [%s/%s] target=%s title=%s", draft.ID, draft.State, draft.Kind, draft.Target.Path, draft.Title)
		out = append(out, excerptRunes(summary, coreContextDraftLimit))
	}
	return out
}

func markdownSection(content string, heading string) string {
	target := strings.ToLower(strings.TrimSpace(heading))
	if target == "" {
		return ""
	}
	text := strings.ReplaceAll(content, "\r\n", "\n")
	lines := strings.Split(text, "\n")
	start := -1
	startLevel := 0
	for i, line := range lines {
		level, title, ok := parseMarkdownHeading(line)
		if !ok {
			continue
		}
		if start < 0 {
			if strings.ToLower(title) == target {
				start = i + 1
				startLevel = level
			}
			continue
		}
		if level <= startLevel {
			return strings.TrimSpace(strings.Join(lines[start:i], "\n"))
		}
	}
	if start < 0 {
		return ""
	}
	return strings.TrimSpace(strings.Join(lines[start:], "\n"))
}

func parseMarkdownHeading(line string) (int, string, bool) {
	trimmed := strings.TrimSpace(line)
	if !strings.HasPrefix(trimmed, "#") {
		return 0, "", false
	}
	level := 0
	for level < len(trimmed) && trimmed[level] == '#' {
		level++
	}
	if level == 0 || level > 6 || level >= len(trimmed) || trimmed[level] != ' ' {
		return 0, "", false
	}
	return level, strings.TrimSpace(trimmed[level:]), true
}

func excerptRunes(value string, limit int) string {
	value = strings.TrimSpace(value)
	if value == "" || limit <= 0 {
		return value
	}
	runes := []rune(value)
	if len(runes) <= limit {
		return value
	}
	return string(runes[:limit]) + "\n..."
}
