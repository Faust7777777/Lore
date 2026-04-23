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
		"# Lore Agent Instructions",
		"## Mission",
		"## Tool Strategy",
		"## Writing Boundaries",
		"draft -> review -> apply",
		"shell commands always require confirmation before execution",
		"runtime still enforces path, doc-class, and governance boundaries",
	} {
		if !strings.Contains(agent.Content, want) {
			t.Fatalf("agent template missing %q:\n%s", want, agent.Content)
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
