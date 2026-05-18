package tools

import (
	"time"

	"obsidian-harness/internal/model"
)

// builtin_proposal.go defines the 2 proposal-intake tools surfaced on
// both the MCP and console surfaces. They translate a generic arguments
// map into a typed model.PersonaUpdateProposal or model.MarkdownNoteProposal
// and dispatch into the harness's ProposeXxx methods. Both tools create a
// pending draft and never apply it directly; the governance line is
// enforced by the harness, not by these tools.
//
// Each tool optionally holds a now func override for deterministic tests.
// In production NewServer-style call sites pass time.Now via the default
// nil clock so the tools call time.Now() at dispatch.

const proposalSurfaces = SurfaceMCP | SurfaceConsole | SurfaceProposal

// RegisterProposal registers the 2 proposal-intake tools into reg. MCP
// tools/list output surfaces them in the order persona_update_propose,
// markdown_note_propose, immediately after the 9 read-only tools.
func RegisterProposal(reg *Registry, h Harness) error {
	tools := []Tool{
		personaUpdateProposeTool{h: h, now: time.Now},
		markdownNoteProposeTool{h: h, now: time.Now},
	}
	for _, t := range tools {
		if err := reg.Register(t); err != nil {
			return err
		}
	}
	return nil
}

type personaUpdateProposeTool struct {
	h   Harness
	now func() time.Time
}

func (t personaUpdateProposeTool) Name() string { return "persona_update_propose" }
func (t personaUpdateProposeTool) Description() string {
	return "Submit a persona update proposal for Lore review. Creates a pending draft; does not write or apply the persona document."
}
func (t personaUpdateProposeTool) Surfaces() Surface { return proposalSurfaces }
func (t personaUpdateProposeTool) Arguments() []Argument {
	return []Argument{
		{Name: "field", Type: ArgString},
		{Name: "current_value", Type: ArgString},
		{Name: "proposed_value", Type: ArgString},
		{Name: "evidence", Type: ArgString},
		{Name: "confidence", Type: ArgString, Enum: []string{"low", "medium", "high"}},
		{Name: "reason", Type: ArgString},
		{Name: "source", Type: ArgString},
		{Name: "observed_at", Type: ArgString},
	}
}
func (t personaUpdateProposeTool) Required() []string {
	return []string{"field", "proposed_value", "evidence", "confidence", "reason", "source", "observed_at"}
}
func (t personaUpdateProposeTool) Call(args map[string]any) (any, error) {
	observedAt, err := parseObservedAt(getString(args, "observed_at"))
	if err != nil {
		return nil, err
	}
	return t.h.ProposePersonaUpdate(model.PersonaUpdateProposal{
		Field:         getString(args, "field"),
		CurrentValue:  getString(args, "current_value"),
		ProposedValue: getString(args, "proposed_value"),
		Evidence:      getString(args, "evidence"),
		Reason:        getString(args, "reason"),
		Confidence:    getString(args, "confidence"),
		Source:        getString(args, "source"),
		ObservedAt:    observedAt,
	}, t.now())
}

type markdownNoteProposeTool struct {
	h   Harness
	now func() time.Time
}

func (t markdownNoteProposeTool) Name() string { return "markdown_note_propose" }
func (t markdownNoteProposeTool) Description() string {
	return "Submit an ordinary markdown note proposal for Lore review. Creates a pending draft; does not write the note and does not apply any draft."
}
func (t markdownNoteProposeTool) Surfaces() Surface { return proposalSurfaces }
func (t markdownNoteProposeTool) Arguments() []Argument {
	return []Argument{
		{Name: "target_path", Type: ArgString},
		{Name: "title", Type: ArgString},
		{Name: "content", Type: ArgString},
		{Name: "source_kind", Type: ArgString, Enum: []string{"class", "meeting", "development", "conversation", "research", "other"}},
		{Name: "evidence", Type: ArgString},
		{Name: "reason", Type: ArgString},
		{Name: "source", Type: ArgString},
		{Name: "observed_at", Type: ArgString},
		{Name: "task_context", Type: ArgString},
		{Name: "course", Type: ArgString},
		{Name: "topic", Type: ArgString},
		{Name: "dedupe_key", Type: ArgString},
	}
}
func (t markdownNoteProposeTool) Required() []string {
	return []string{"target_path", "title", "content", "source_kind", "evidence", "reason", "source", "observed_at"}
}
func (t markdownNoteProposeTool) Call(args map[string]any) (any, error) {
	observedAt, err := parseObservedAt(getString(args, "observed_at"))
	if err != nil {
		return nil, err
	}
	return t.h.ProposeMarkdownNote(model.MarkdownNoteProposal{
		TargetPath:  getString(args, "target_path"),
		Title:       getString(args, "title"),
		Content:     getString(args, "content"),
		SourceKind:  getString(args, "source_kind"),
		Evidence:    getString(args, "evidence"),
		Reason:      getString(args, "reason"),
		Source:      getString(args, "source"),
		ObservedAt:  observedAt,
		TaskContext: getString(args, "task_context"),
		Course:      getString(args, "course"),
		Topic:       getString(args, "topic"),
		DedupeKey:   getString(args, "dedupe_key"),
	}, t.now())
}
