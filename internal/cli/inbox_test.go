package cli

import (
	"bytes"
	"strings"
	"testing"
	"time"

	"obsidian-harness/internal/app"
	"obsidian-harness/internal/model"
	"obsidian-harness/internal/persona"
)

func TestRenderOperatorQueue(t *testing.T) {
	cand := persona.PersonaCandidate{Field: "major", ProposedValue: "economics"}
	queue := app.OperatorQueue{
		Day:                   time.Date(2026, 6, 4, 0, 0, 0, 0, time.UTC),
		PendingDrafts:         []model.Draft{{ID: "draft-1", Kind: model.DraftKindPersonaUpdate, Title: "update major"}},
		OpenFindings:          []model.Finding{{ID: "finding-1", Severity: model.FindingSeverityInfo, Title: "out-of-band write"}},
		OpenPersonaCandidates: []persona.PersonaCandidateRecord{{ID: "pc-1", Candidate: cand}},
		TodayUsage:            model.UsageSummary{Calls: 4, TotalTokens: 1180},
	}

	var buf bytes.Buffer
	renderOperatorQueue(&buf, queue)
	out := buf.String()

	for _, want := range []string{
		"Operator Queue",
		"Action items: 3",
		"Drafts pending review (1):", "draft-1", "update major",
		"Open findings (1):", "finding-1", "out-of-band write",
		"Open persona candidates (1):", "pc-1", "major = economics",
		"Today's usage: 4 calls / 1180 tokens",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("render missing %q:\n%s", want, out)
		}
	}
}

func TestRenderOperatorQueueEmptyShowsNone(t *testing.T) {
	var buf bytes.Buffer
	renderOperatorQueue(&buf, app.OperatorQueue{Day: time.Date(2026, 6, 4, 0, 0, 0, 0, time.UTC)})
	out := buf.String()

	if !strings.Contains(out, "Action items: 0") {
		t.Fatalf("empty queue should show 0 action items:\n%s", out)
	}
	if strings.Count(out, "(none)") != 3 {
		t.Fatalf("empty queue should show (none) for all 3 sections:\n%s", out)
	}
}
