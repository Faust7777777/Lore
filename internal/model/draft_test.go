package model

import (
	"encoding/json"
	"testing"
	"time"
)

func TestMarkdownNoteProposalJSONContract(t *testing.T) {
	observedAt := time.Date(2026, 4, 28, 10, 30, 0, 0, time.UTC)
	proposal := MarkdownNoteProposal{
		TargetPath:  "03-notes/inbox/ecommerce-platforms.md",
		Title:       "E-commerce Platforms",
		Content:     "# E-commerce Platforms\n",
		SourceKind:  "class",
		Evidence:    "class transcript",
		Reason:      "durable class note",
		Source:      "external_agent",
		ObservedAt:  observedAt,
		TaskContext: "course recap",
		Course:      "E-commerce",
		Topic:       "platforms",
		Tags:        []string{"ecommerce"},
		RelatedPaths: []string{
			"03-notes/markets.md",
		},
		DedupeKey: "class-2026-04-28-platforms",
	}

	data, err := json.Marshal(proposal)
	if err != nil {
		t.Fatalf("json.Marshal() error = %v", err)
	}
	var decoded map[string]any
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("json.Unmarshal() error = %v", err)
	}
	for _, key := range []string{
		"target_path",
		"title",
		"content",
		"source_kind",
		"evidence",
		"reason",
		"source",
		"observed_at",
		"task_context",
		"course",
		"topic",
		"tags",
		"related_paths",
		"dedupe_key",
	} {
		if _, ok := decoded[key]; !ok {
			t.Fatalf("encoded proposal missing %s in %#v", key, decoded)
		}
	}
}

func TestMarkdownNoteDraftConstants(t *testing.T) {
	if DraftKindMarkdownNoteWrite != "markdown_note_write" {
		t.Fatalf("DraftKindMarkdownNoteWrite = %q", DraftKindMarkdownNoteWrite)
	}
	if DraftBaseVersionNewFile != "new" {
		t.Fatalf("DraftBaseVersionNewFile = %q", DraftBaseVersionNewFile)
	}
}
