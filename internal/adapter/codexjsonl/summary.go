package codexjsonl

import (
	"fmt"
	"path/filepath"
	"strings"

	"obsidian-harness/internal/model"
)

func buildTitle(window model.SessionWindow, events []Event) string {
	headline := ""
	for _, event := range events {
		text := normalizeText(event.Text)
		if text == "" {
			continue
		}
		headline = text
		break
	}
	if headline != "" {
		if len(headline) > 72 {
			headline = headline[:72] + "..."
		}
		return fmt.Sprintf(
			"%s checkpoint %s-%s: %s",
			window.AgentID,
			window.WindowStart.Format("15:04"),
			window.WindowEnd.Format("15:04"),
			headline,
		)
	}
	return ""
}

func summarizeWindow(sourcePath string, events []Event) string {
	if len(events) == 0 {
		return ""
	}
	lines := []string{
		fmt.Sprintf("- source: `%s`", filepath.Base(sourcePath)),
		fmt.Sprintf("- transcript events: %d", len(events)),
		fmt.Sprintf("- roles: %s", strings.Join(uniqueRoles(events), ", ")),
	}

	userSnippets := collectSnippets(events, "user", 3)
	assistantSnippets := collectSnippets(events, "assistant", 4)
	if len(userSnippets) > 0 {
		lines = append(lines, "", "## User Inputs")
		lines = append(lines, bulletLines(userSnippets)...)
	}
	if len(assistantSnippets) > 0 {
		lines = append(lines, "", "## Agent Outputs")
		lines = append(lines, bulletLines(assistantSnippets)...)
	}
	if len(userSnippets) == 0 && len(assistantSnippets) == 0 {
		lines = append(lines, "", "## Notes", "- no textual transcript content was extracted")
	}
	return strings.Join(lines, "\n")
}

func joinRawLines(events []Event) string {
	lines := make([]string, 0, len(events))
	for _, event := range events {
		lines = append(lines, event.RawLine)
	}
	return strings.Join(lines, "\n")
}

func uniqueRoles(events []Event) []string {
	seen := make(map[string]struct{}, len(events))
	out := make([]string, 0, len(events))
	for _, event := range events {
		if event.Role == "" {
			continue
		}
		if _, ok := seen[event.Role]; ok {
			continue
		}
		seen[event.Role] = struct{}{}
		out = append(out, event.Role)
	}
	if len(out) == 0 {
		return []string{"unknown"}
	}
	return out
}

func collectSnippets(events []Event, role string, limit int) []string {
	if limit <= 0 {
		return nil
	}
	out := make([]string, 0, limit)
	for _, event := range events {
		if event.Role != role {
			continue
		}
		text := normalizeText(event.Text)
		if text == "" {
			continue
		}
		if role == "assistant" && strings.TrimSpace(event.Phase) != "" {
			text = "[" + strings.TrimSpace(event.Phase) + "] " + text
		}
		if len(text) > 180 {
			text = text[:180] + "..."
		}
		out = append(out, text)
		if len(out) >= limit {
			break
		}
	}
	return out
}

func bulletLines(items []string) []string {
	out := make([]string, 0, len(items))
	for _, item := range items {
		out = append(out, "- "+item)
	}
	return out
}
