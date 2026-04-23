package bootstrap

import (
	"fmt"
	"path/filepath"
	"time"

	"obsidian-harness/internal/model"
)

type Template struct {
	Ref     model.DocumentRef `json:"ref"`
	Content string            `json:"content"`
}

func DefaultManagedTemplates(now time.Time) []Template {
	weekStart := now.AddDate(0, 0, -int(now.Weekday()-1))
	if now.Weekday() == time.Sunday {
		weekStart = now.AddDate(0, 0, -6)
	}
	weekEnd := weekStart.AddDate(0, 0, 6)
	weekName := fmt.Sprintf(
		"Week1\u9010\u65e5\u6267\u884c\u8868-%s\u81f3%s.md",
		weekStart.Format("2006-01-02"),
		weekEnd.Format("2006-01-02"),
	)

	return []Template{
		{
			Ref: model.DocumentRef{
				Path:  filepath.Join("00-\u7cfb\u7edf", "\u7cfb\u7edf\u8bf4\u660e.md"),
				Class: model.DocClassSystemDoc,
			},
			Content: "# \u7cfb\u7edf\u8bf4\u660e\n\n- \u76ee\u6807: \u8bf4\u660e\u672c Obsidian \u5de5\u4f5c\u533a\u7684\u6cbb\u7406\u8fb9\u754c\u548c\u534f\u4f5c\u65b9\u5f0f\u3002\n- Managed Mode: \u5df2\u542f\u7528\u3002\n- \u4e09\u4ef6\u5957: \u7cfb\u7edf\u8bf4\u660e / \u6587\u6863\u8fdb\u5ea6\u603b\u8868 / \u4eba\u7269\u753b\u50cf / agent.md / identity.md\u3002\n",
		},
		{
			Ref: model.DocumentRef{
				Path:  filepath.Join("0-\u6392\u671f", "00-\u7cfb\u7edf", "\u6587\u6863\u8fdb\u5ea6\u603b\u8868.md"),
				Class: model.DocClassProgressIndex,
			},
			Content: "# \u6587\u6863\u8fdb\u5ea6\u603b\u8868\n\n| \u6587\u6863 | \u7c7b\u578b | \u72b6\u6001 | \u6700\u8fd1\u66f4\u65b0 |\n| --- | --- | --- | --- |\n| \u7cfb\u7edf\u8bf4\u660e | \u7cfb\u7edf\u6587\u6863 | \u5728\u7ba1 | |\n| \u4eba\u7269\u753b\u50cf | \u753b\u50cf | \u5728\u7ba1 | |\n| agent.md | Agent \u6307\u4ee4 | \u5728\u7ba1 | |\n| identity.md | Agent \u8eab\u4efd | \u5728\u7ba1 | |\n",
		},
		{
			Ref: model.DocumentRef{
				Path:  filepath.Join("03-\u753b\u50cf", "\u4eba\u7269\u753b\u50cf.md"),
				Class: model.DocClassPersona,
			},
			Content: "# \u4eba\u7269\u753b\u50cf\n\n## Identity / Stable Profile\n\n- \u59d3\u540d:\n- \u76ee\u6807\u89d2\u8272:\n\n## Current State\n\n- \u5f53\u524d\u91cd\u70b9:\n- \u5f53\u524d\u963b\u585e:\n\n## Weaknesses\n\n- \u5f85\u8865\u5145\n\n## Evidence Log\n\n- \u5f85\u8865\u5145\n\n## Freeform Notes\n\n- \u5f85\u8865\u5145\n",
		},
		{
			Ref: model.DocumentRef{
				Path:  "agent.md",
				Class: model.DocClassAgentDoc,
			},
			Content: "# Lore Agent Instructions\n\n## Role\n\n- You are Lore, the governed workspace agent for this vault.\n- Prefer Lore vault context and governed tools over generic local execution.\n\n## Default Behavior\n\n- Read before writing.\n- Keep managed core docs and plan/execution docs on draft -> review -> apply.\n- Use direct note writes only for low-governance notes when the user explicitly asks for one.\n\n## Interaction Style\n\n- Be concise, factual, and task-oriented.\n- State constraints plainly instead of hiding them.\n",
		},
		{
			Ref: model.DocumentRef{
				Path:  "identity.md",
				Class: model.DocClassIdentityDoc,
			},
			Content: "# Lore Identity\n\n## Core Identity\n\n- Name: Lore\n- Domain: governed knowledge operations\n- Default stance: practical, audit-aware, user-aligned\n\n## Working Preferences\n\n- Preserve user intent without bypassing governance.\n- Prefer explicit reasoning over vague reassurance.\n\n## Workspace Notes\n\n- Add vault-specific identity details here.\n",
		},
		{
			Ref: model.DocumentRef{
				Path:  filepath.Join("0-\u6392\u671f", "04-\u6267\u884c", weekName),
				Class: model.DocClassPlanWeek,
			},
			Content: fmt.Sprintf("# Week1\u9010\u65e5\u6267\u884c\u8868\n\n- \u5468\u671f: %s \u81f3 %s\n- [ ] \u4eca\u65e5\u91cd\u70b9 1\n- [ ] \u4eca\u65e5\u91cd\u70b9 2\n", weekStart.Format("2006-01-02"), weekEnd.Format("2006-01-02")),
		},
	}
}
