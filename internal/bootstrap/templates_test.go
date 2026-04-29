package bootstrap

import (
	"strings"
	"testing"
	"time"

	"obsidian-harness/internal/model"
)

func TestDefaultManagedTemplatesIncludeRuntimeAgentDocs(t *testing.T) {
	templates := DefaultManagedTemplates(time.Date(2026, 4, 23, 9, 0, 0, 0, time.UTC))

	agent := findTemplateByClass(templates, model.DocClassAgentDoc)
	if agent == nil {
		t.Fatal("expected agent template to exist")
	}
	if agent.Ref.Path != "agent.md" {
		t.Fatalf("agent.Ref.Path = %q, want agent.md", agent.Ref.Path)
	}
	for _, want := range []string{
		"# Workspace Agent Operating Manual",
		"for external agents working through Lore MCP",
		"This is not Lore self identity",
		"system_doc_get(\"agent\")",
		"## Mission",
		"## Tool Strategy",
		"## Writing Boundaries",
		"## Persona Update Candidates",
		"## Markdown Note Candidates",
		"draft -> review -> apply",
		"Current MCP exposes read tools and narrow proposal intake",
		"External agents must submit durable markdown write requests",
		"direct vault markdown write",
		"persona_update_propose",
		"Proposal creation is not an apply",
		"Persona Update Candidate:",
		"Markdown Note Candidate:",
		"action: request_lore_review",
	} {
		if !strings.Contains(agent.Content, want) {
			t.Fatalf("agent template missing %q:\n%s", want, agent.Content)
		}
	}
	for _, forbidden := range []string{
		"Low-risk writing may be added later",
		"Low-governance note writing may be allowed later",
	} {
		if strings.Contains(agent.Content, forbidden) {
			t.Fatalf("agent template contains forbidden external direct-write wording %q:\n%s", forbidden, agent.Content)
		}
	}

	identity := findTemplateByClass(templates, model.DocClassIdentityDoc)
	if identity == nil {
		t.Fatal("expected identity template to exist")
	}
	if identity.Ref.Path != "identity.md" {
		t.Fatalf("identity.Ref.Path = %q, want identity.md", identity.Ref.Path)
	}
	for _, want := range []string{
		"# Lore Identity",
		"## Audience",
		"local Lore agent's self identity",
		"External agents should not default-read this file",
		"External agents should read `agent.md`",
		"## Core Identity",
		"## Collaboration Defaults",
		"## Focus Areas",
		"## Workspace Customization",
	} {
		if !strings.Contains(identity.Content, want) {
			t.Fatalf("identity template missing %q:\n%s", want, identity.Content)
		}
	}
}

func findTemplateByClass(templates []Template, class model.DocClass) *Template {
	for i := range templates {
		if templates[i].Ref.Class == class {
			return &templates[i]
		}
	}
	return nil
}
