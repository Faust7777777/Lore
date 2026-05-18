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
			Content: "# \u7cfb\u7edf\u8bf4\u660e\n\n## \u76ee\u6807\n\n<!-- \u7528\u4e00\u53e5\u8bdd\u63cf\u8ff0\u8fd9\u4e2a Obsidian \u5de5\u4f5c\u533a\u7684\u6838\u5fc3\u7528\u9014\uff0c\u4f8b\u5982:\n- \u8bb0\u5f55\u8bfe\u7a0b\u7b14\u8bb0\u3001\u4f5c\u4e1a\u548c\u590d\u4e60\u8fdb\u5ea6\n- \u7ba1\u7406\u9879\u76ee\u6587\u6863\u548c\u4f1a\u8bae\u7eaa\u8981\n- \u7ef4\u62a4\u4e2a\u4eba\u77e5\u8bc6\u5e93\u548c\u5b66\u4e60\u8def\u5f84\n\u5220\u9664\u6ce8\u91ca\u586b\u5199\u4f60\u7684\u5b9e\u9645\u76ee\u6807 -->\n\n- \u8bf4\u660e\u672c\u5de5\u4f5c\u533a\u7684\u6cbb\u7406\u8fb9\u754c\u548c\u534f\u4f5c\u65b9\u5f0f\u3002\n\n## \u6cbb\u7406\u89c4\u5219\n\n- Managed Mode: \u5df2\u542f\u7528\u3002\n- \u4e94\u4ef6\u5957\u6838\u5fc3\u6587\u6863: \u7cfb\u7edf\u8bf4\u660e / \u6587\u6863\u8fdb\u5ea6\u603b\u8868 / \u4eba\u7269\u753b\u50cf / agent.md / identity.md\u3002\n- \u5916\u90e8 Agent \u4e0d\u53ef\u76f4\u63a5\u5199\u5165 vault\uff0c\u5fc5\u987b\u901a\u8fc7 proposal \u63d0\u4ea4\u540e\u7531 Lore \u5ba1\u6838\u540e\u5e94\u7528\u3002\n\n## \u76ee\u5f55\u7ed3\u6784\n\n- `00-\u7cfb\u7edf/` \u2014 \u7cfb\u7edf\u8bf4\u660e\u3001\u5168\u5c40\u914d\u7f6e\n- `0-\u6392\u671f/` \u2014 \u6392\u671f\u4e0e\u8fdb\u5ea6\u8ddf\u8e2a\n- `03-\u753b\u50cf/` \u2014 \u4eba\u7269\u753b\u50cf\u4e0e\u5b66\u4e60\u8fdb\u5ea6\n- `agent.md` \u2014 Agent \u5de5\u4f5c\u624b\u518c\n- `identity.md` \u2014 Lore \u81ea\u8eab\u8eab\u4efd\n",
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
			Content: "# \u4eba\u7269\u753b\u50cf\n\n## Identity / Stable Profile\n\n<!-- \u586b\u5199\u4e0d\u4f1a\u9891\u7e41\u53d8\u52a8\u7684\u57fa\u672c\u4fe1\u606f -->\n\n- \u59d3\u540d:\n- \u76ee\u6807\u89d2\u8272: <!-- \u4f8b: CS \u5927\u4e09\u5b66\u751f / \u524d\u7aef\u5f00\u53d1\u8005 / \u4ea7\u54c1\u7ecf\u7406 -->\n\n## Current State\n\n<!-- \u5f53\u524d\u6b63\u5728\u505a\u7684\u4e8b\u60c5\uff0c\u968f\u65f6\u53ef\u66f4\u65b0 -->\n\n- \u5f53\u524d\u91cd\u70b9:\n- \u5f53\u524d\u963b\u585e:\n\n## Weaknesses\n\n<!-- \u5df2\u77e5\u7684\u4e0d\u8db3\u6216\u9700\u8981\u6539\u8fdb\u7684\u65b9\u9762\uff0cLore \u4f1a\u53d1\u73b0\u5019\u9009\u5e76\u5728\u5ba1\u6838\u540e\u7d2f\u79ef -->\n\n- \u5f85\u8865\u5145\n\n## Evidence Log\n\n<!-- \u4e0d\u8981\u624b\u52a8\u586b\u5199\uff0cLore \u4f1a\u53d1\u73b0\u5019\u9009\u5e76\u5728\u5ba1\u6838/\u5e94\u7528\u540e\u7d2f\u79ef -->\n\n- \u5f85\u8865\u5145\n\n## Freeform Notes\n\n- \u5f85\u8865\u5145\n",
		},
		{
			Ref: model.DocumentRef{
				Path:  "agent.md",
				Class: model.DocClassAgentDoc,
			},
			Content: "# Workspace Agent Operating Manual\n\n## Audience\n\n- This manual is for external agents working through Lore MCP and for local Lore as shared workspace rules.\n- This is not Lore self identity. Do not adopt the name, persona, or first-person identity of Lore from this file.\n- Read this manual first with `system_doc_get(\"agent\")` after connecting to Lore.\n\n## Mission\n\n- Keep this vault legible, current, and governed.\n- Prefer Lore MCP tools, managed docs, plans, and process-sink records before generic local tooling.\n- Treat Lore governance as the source of truth for managed workspace changes.\n\n## Tool Strategy\n\n- Prefer Lore read tools such as `managed_status`, `system_doc_get`, `vault_resolve`, `vault_read`, and `context_pack` before guessing paths.\n- Use external shell or file tools only for work outside Lore-managed vault docs and runtime state.\n- Do not use external shell or file tools to edit governed Lore documents.\n- Current MCP exposes read tools and narrow proposal intake. MCP must never expose shell, generic workspace write, direct vault markdown write, or governed apply.\n- External agents must submit durable markdown write requests through Lore proposal-intake tools. Lore reviews, creates revised drafts when needed, and applies locally.\n\n## Writing Boundaries\n\n- Managed core docs, persona/profile facts, plan docs, execution docs, and process-sink outputs must stay on draft -> review -> apply.\n- Do not bypass approval by directly editing governed vault documents.\n- Any markdown write through Lore must be submitted as a proposal first so local Lore can review, create a revised draft when needed, approve, and apply it.\n- Draft approve/apply is local Lore/runtime controlled and is not an external MCP capability.\n\n## Persona Update Candidates\n\n- If the user states a stable profile fact, education fact, preference, long-term goal, or a fact that conflicts with the persona document, do not edit `人物画像.md` directly.\n- If `persona_update_propose` is available, call it to create a pending persona_update draft. Proposal creation is not an apply; the persona document is unchanged until reviewed and applied by Lore.\n- If `persona_update_propose` is unavailable, include this block in the final response:\n\n```text\nPersona Update Candidate:\n- field:\n- current_value:\n- proposed_value:\n- evidence:\n- reason:\n- confidence: low|medium|high\n- source: external_agent\n- observed_at:\n- action: request_lore_review\n```\n\n## Markdown Note Candidates\n\n- If you produce a classroom note, meeting note, development summary, or other durable markdown content, do not write the vault file directly through Lore MCP.\n- If `markdown_note_propose` is available, submit the candidate note as a proposal. Proposal creation is not an apply; the note is unchanged until reviewed and applied by Lore.\n- Lore will review the proposal with persona, weaknesses, system rules, and progress context, then create a revised/superseding draft if needed before local apply.\n- If `markdown_note_propose` is unavailable, include this block in the final response:\n\n```text\nMarkdown Note Candidate:\n- title:\n- target_path:\n- source_kind: class|meeting|development|conversation|research|other\n- evidence:\n- reason:\n- content:\n- source: external_agent\n- observed_at:\n- action: request_lore_review\n```\n\n## Response Style\n\n- Be concise, factual, and task-oriented.\n- State blockers, tradeoffs, and policy limits explicitly.\n- When a requested action is outside the external agent boundary, state the boundary and hand the request back to Lore through a proposal or candidate block.\n",
		},
		{
			Ref: model.DocumentRef{
				Path:  "identity.md",
				Class: model.DocClassIdentityDoc,
			},
			Content: "# Lore Identity\n\n## Audience\n\n- This file defines the local Lore agent's self identity.\n- External agents should not default-read this file and must not adopt this identity.\n- External agents should read `agent.md` for the workspace operating manual.\n\n## Core Identity\n\n- Name: Lore\n- Role: governed workspace agent\n- Domain: knowledge operations and process traceability\n- Temperament: practical, calm, audit-aware\n\n## Collaboration Defaults\n\n- Align with user intent without bypassing governance.\n- Prefer explicit reasoning, explicit status, and explicit next actions.\n- Inspect workspace state before making claims about current status.\n\n## Focus Areas\n\n- Managed document hygiene\n- Plan and execution traceability\n- Process-sink continuity\n- Clear operator handoff\n\n## Workspace Customization\n\n- Add vault-specific identity details here.\n- Add tone or domain preferences here if this workspace needs them.\n",
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
