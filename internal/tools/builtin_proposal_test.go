package tools

import (
	"errors"
	"reflect"
	"testing"
	"time"

	"obsidian-harness/internal/model"
)

func TestRegisterProposalRegistersBothToolsInLegacyOrder(t *testing.T) {
	r := NewRegistry()
	if err := RegisterProposal(r, &fakeHarness{}); err != nil {
		t.Fatalf("RegisterProposal() error = %v", err)
	}

	got := names(r.ListBySurface(SurfaceMCP))
	want := []string{"persona_update_propose", "markdown_note_propose"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("ListBySurface(MCP) = %v, want %v", got, want)
	}

	// Both tools also appear on the SurfaceConsole and SurfaceProposal
	// surfaces. SurfaceProposal is informational only, but if a Surface
	// bit is set callers can filter on it for documentation purposes.
	gotProposal := names(r.ListBySurface(SurfaceProposal))
	if !reflect.DeepEqual(gotProposal, want) {
		t.Errorf("ListBySurface(Proposal) = %v, want %v", gotProposal, want)
	}
}

func TestRegisterProposalMCPDefinitionsMatchLegacyShape(t *testing.T) {
	r := NewRegistry()
	if err := RegisterProposal(r, &fakeHarness{}); err != nil {
		t.Fatalf("RegisterProposal() error = %v", err)
	}

	defs := MCPDefinitions(r.ListBySurface(SurfaceMCP))
	want := proposalLegacyDefinitions()
	if len(defs) != len(want) {
		t.Fatalf("definitions count = %d, want %d", len(defs), len(want))
	}
	for i := range defs {
		if !reflect.DeepEqual(defs[i], want[i]) {
			t.Errorf("definitions[%d] mismatch\n got = %#v\nwant = %#v", i, defs[i], want[i])
		}
	}
}

func TestPersonaUpdateProposeCallTranslatesArgs(t *testing.T) {
	frozenNow := time.Date(2026, 4, 30, 9, 0, 0, 0, time.UTC)
	h := &fakeHarness{
		proposePersonaRes: model.PersonaUpdateProposalResult{DraftID: "draft-x", Status: "pending_review"},
	}
	tool := personaUpdateProposeTool{
		h:   h,
		now: func() time.Time { return frozenNow },
	}

	got, err := tool.Call(map[string]any{
		"field":          "morning_routine",
		"current_value":  "wake at 8",
		"proposed_value": "wake at 7",
		"evidence":       "noted at lab",
		"confidence":     "high",
		"reason":         "consistency",
		"source":         "self_observed",
		"observed_at":    "2026-04-22",
	})
	if err != nil {
		t.Fatalf("Call() error = %v", err)
	}
	res, ok := got.(model.PersonaUpdateProposalResult)
	if !ok {
		t.Fatalf("Call() returned %T, want model.PersonaUpdateProposalResult", got)
	}
	if res.DraftID != "draft-x" {
		t.Errorf("res.DraftID = %q, want draft-x", res.DraftID)
	}

	if h.proposePersonaArg.Field != "morning_routine" {
		t.Errorf("proposePersonaArg.Field = %q, want morning_routine", h.proposePersonaArg.Field)
	}
	if h.proposePersonaArg.Confidence != "high" {
		t.Errorf("proposePersonaArg.Confidence = %q, want high", h.proposePersonaArg.Confidence)
	}
	wantObservedAt := time.Date(2026, 4, 22, 0, 0, 0, 0, time.UTC)
	if !h.proposePersonaArg.ObservedAt.Equal(wantObservedAt) {
		t.Errorf("ObservedAt = %v, want %v", h.proposePersonaArg.ObservedAt, wantObservedAt)
	}
	if !h.proposePersonaAt.Equal(frozenNow) {
		t.Errorf("at = %v, want frozen now %v (proves now func override is used)", h.proposePersonaAt, frozenNow)
	}
}

func TestMarkdownNoteProposeCallTranslatesAllArgs(t *testing.T) {
	frozenNow := time.Date(2026, 4, 30, 12, 0, 0, 0, time.UTC)
	h := &fakeHarness{
		proposeMarkdownRes: model.MarkdownNoteProposalResult{DraftID: "draft-y", Status: "pending_review"},
	}
	tool := markdownNoteProposeTool{
		h:   h,
		now: func() time.Time { return frozenNow },
	}

	if _, err := tool.Call(map[string]any{
		"target_path":  "03-notes/inbox/x.md",
		"title":        "X",
		"content":      "body",
		"source_kind":  "research",
		"evidence":     "ev",
		"reason":       "rs",
		"source":       "src",
		"observed_at":  "2026-04-22T09:00:00Z",
		"task_context": "tc",
		"course":       "c",
		"topic":        "t",
		"dedupe_key":   "k",
	}); err != nil {
		t.Fatalf("Call() error = %v", err)
	}

	got := h.proposeMarkdownArg
	if got.TargetPath != "03-notes/inbox/x.md" {
		t.Errorf("TargetPath = %q", got.TargetPath)
	}
	if got.SourceKind != "research" {
		t.Errorf("SourceKind = %q, want research", got.SourceKind)
	}
	if got.TaskContext != "tc" || got.Course != "c" || got.Topic != "t" || got.DedupeKey != "k" {
		t.Errorf("optional fields lost: %+v", got)
	}
	wantObservedAt := time.Date(2026, 4, 22, 9, 0, 0, 0, time.UTC)
	if !got.ObservedAt.Equal(wantObservedAt) {
		t.Errorf("ObservedAt = %v, want %v", got.ObservedAt, wantObservedAt)
	}
}

func TestProposalToolReturnsParseObservedAtError(t *testing.T) {
	cases := map[string]string{
		"empty":            "",
		"unsupported form": "April 22, 2026",
	}

	for name, raw := range cases {
		t.Run(name, func(t *testing.T) {
			h := &fakeHarness{}
			tool := personaUpdateProposeTool{h: h, now: time.Now}
			_, err := tool.Call(map[string]any{
				"field":          "x",
				"proposed_value": "y",
				"evidence":       "e",
				"confidence":     "high",
				"reason":         "r",
				"source":         "s",
				"observed_at":    raw,
			})
			if err == nil {
				t.Fatalf("Call(observed_at=%q) error = nil, want non-nil", raw)
			}
			if h.proposePersonaArg.Field != "" {
				t.Error("ProposePersonaUpdate must not be called when observed_at fails to parse")
			}
		})
	}
}

func TestProposalToolPropagatesHarnessError(t *testing.T) {
	wantErr := errors.New("harness rejected")
	h := &fakeHarness{proposePersonaErr: wantErr}
	tool := personaUpdateProposeTool{h: h, now: time.Now}

	_, err := tool.Call(map[string]any{
		"field":          "x",
		"proposed_value": "y",
		"evidence":       "e",
		"confidence":     "high",
		"reason":         "r",
		"source":         "s",
		"observed_at":    "2026-04-22",
	})
	if !errors.Is(err, wantErr) {
		t.Errorf("Call() error = %v, want %v", err, wantErr)
	}
}

func TestParseObservedAtAcceptsRFC3339AndDate(t *testing.T) {
	cases := map[string]struct {
		raw  string
		want time.Time
	}{
		"date":    {raw: "2026-04-22", want: time.Date(2026, 4, 22, 0, 0, 0, 0, time.UTC)},
		"rfc3339": {raw: "2026-04-22T09:30:00Z", want: time.Date(2026, 4, 22, 9, 30, 0, 0, time.UTC)},
		"trim":    {raw: "  2026-04-22  ", want: time.Date(2026, 4, 22, 0, 0, 0, 0, time.UTC)},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			got, err := parseObservedAt(tc.raw)
			if err != nil {
				t.Fatalf("parseObservedAt(%q) error = %v", tc.raw, err)
			}
			if !got.Equal(tc.want) {
				t.Errorf("got = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestParseObservedAtRejectsEmptyAndInvalid(t *testing.T) {
	cases := map[string]string{
		"empty":   "",
		"invalid": "yesterday",
		"partial": "2026/04/22",
	}

	for name, raw := range cases {
		t.Run(name, func(t *testing.T) {
			_, err := parseObservedAt(raw)
			if err == nil {
				t.Fatalf("parseObservedAt(%q) err = nil, want non-nil", raw)
			}
		})
	}
}

// proposalLegacyDefinitions returns the byte-equivalent expected MCP
// definitions for the 2 proposal tools, derived from the entries that
// previously lived in internal/mcp/tool_contract.go's toolContracts()
// table. Used to confirm RegisterProposal produces an identical
// tools/list payload to the legacy code path. If this fixture and the
// SDK contract snapshot at docs/contracts/mcp-sdk-tools-v0.json drift,
// commit 3a has changed observable MCP behavior.
func proposalLegacyDefinitions() []map[string]any {
	return []map[string]any{
		{
			"name":        "persona_update_propose",
			"description": "Submit a persona update proposal for Lore review. Creates a pending draft; does not write or apply the persona document.",
			"inputSchema": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"field":          map[string]any{"type": "string"},
					"current_value":  map[string]any{"type": "string"},
					"proposed_value": map[string]any{"type": "string"},
					"evidence":       map[string]any{"type": "string"},
					"confidence":     map[string]any{"type": "string", "enum": []string{"low", "medium", "high"}},
					"reason":         map[string]any{"type": "string"},
					"source":         map[string]any{"type": "string"},
					"observed_at":    map[string]any{"type": "string"},
				},
				"required": []string{"field", "proposed_value", "evidence", "confidence", "reason", "source", "observed_at"},
			},
		},
		{
			"name":        "markdown_note_propose",
			"description": "Submit an ordinary markdown note proposal for Lore review. Creates a pending draft; does not write the note and does not apply any draft.",
			"inputSchema": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"target_path":  map[string]any{"type": "string"},
					"title":        map[string]any{"type": "string"},
					"content":      map[string]any{"type": "string"},
					"source_kind":  map[string]any{"type": "string", "enum": []string{"class", "meeting", "development", "conversation", "research", "other"}},
					"evidence":     map[string]any{"type": "string"},
					"reason":       map[string]any{"type": "string"},
					"source":       map[string]any{"type": "string"},
					"observed_at":  map[string]any{"type": "string"},
					"task_context": map[string]any{"type": "string"},
					"course":       map[string]any{"type": "string"},
					"topic":        map[string]any{"type": "string"},
					"dedupe_key":   map[string]any{"type": "string"},
				},
				"required": []string{"target_path", "title", "content", "source_kind", "evidence", "reason", "source", "observed_at"},
			},
		},
	}
}
