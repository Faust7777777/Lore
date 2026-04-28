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
			Content: "# Workspace Agent Operating Manual\n\n## Audience\n\n- This manual is for external agents working through Lore MCP and for local Lore as shared workspace rules.\n- This is not Lore self identity. Do not adopt the name, persona, or first-person identity of Lore from this file.\n- Read this manual first with `system_doc_get(\"agent\")` after connecting to Lore.\n\n## Mission\n\n- Keep this vault legible, current, and governed.\n- Prefer Lore MCP tools, managed docs, plans, and process-sink records before generic local tooling.\n- Treat Lore governance as the source of truth for managed workspace changes.\n\n## Tool Strategy\n\n- Prefer Lore read tools such as `managed_status`, `system_doc_get`, `vault_resolve`, `vault_read`, and `context_pack` before guessing paths.\n- Use external shell or file tools only for work outside Lore-managed vault docs and runtime state.\n- Do not use external shell or file tools to edit governed Lore documents.\n- Current MCP v0 is read-only. MCP v1 may add proposal intake and low-risk writing, but never shell, generic workspace write, or governed apply.\n\n## Writing Boundaries\n\n- Managed core docs, persona/profile facts, plan docs, execution docs, and process-sink outputs must stay on draft -> review -> apply.\n- Do not bypass approval by directly editing governed vault documents.\n- Low-governance note writing may be allowed later only through Lore runtime validation and audit.\n- Draft approve/apply is local Lore/runtime controlled and is not an external MCP capability.\n\n## Persona Update Candidates\n\n- If the user states a stable profile fact, education fact, preference, long-term goal, or a fact that conflicts with the persona document, do not edit `人物画像.md` directly.\n- If `persona_update_propose` is available, call it to create a pending persona_update draft. Proposal creation is not an apply; the persona document is unchanged until reviewed and applied by Lore.\n- If `persona_update_propose` is unavailable, include this block in the final response:\n\n```text\nPersona Update Candidate:\n- field:\n- current_value:\n- proposed_value:\n- evidence:\n- reason:\n- confidence: low|medium|high\n- source: external_agent\n- observed_at:\n- action: request_lore_review\n```\n\n## Response Style\n\n- Be concise, factual, and task-oriented.\n- State blockers, tradeoffs, and policy limits explicitly.\n- When a requested action is outside the external agent boundary, state the boundary and hand the request back to Lore through a proposal or candidate block.\n",
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
