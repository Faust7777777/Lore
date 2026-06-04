package cli

import (
	"bytes"
	"encoding/json"
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

func TestEmitOperatorQueueJSON(t *testing.T) {
	cand := persona.PersonaCandidate{Field: "major", ProposedValue: "economics"}
	queue := app.OperatorQueue{
		Day:                   time.Date(2026, 6, 4, 0, 0, 0, 0, time.UTC),
		PendingDrafts:         []model.Draft{{ID: "draft-1", Kind: model.DraftKindPersonaUpdate, Title: "update major"}},
		OpenFindings:          []model.Finding{{ID: "finding-1", Severity: model.FindingSeverityInfo, Title: "oob write"}},
		OpenPersonaCandidates: []persona.PersonaCandidateRecord{{ID: "pc-1", Candidate: cand}},
		TodayUsage:            model.UsageSummary{Calls: 4, TotalTokens: 1180},
	}

	var buf bytes.Buffer
	if err := emitOperatorQueueJSON(&buf, queue); err != nil {
		t.Fatalf("emitOperatorQueueJSON: %v", err)
	}
	out := strings.TrimSpace(buf.String())
	if strings.Count(out, "\n") != 0 {
		t.Fatalf("JSON should be a single line:\n%s", out)
	}

	var got struct {
		Day           string `json:"day"`
		ActionItems   int    `json:"action_items"`
		PendingDrafts []struct {
			ID    string `json:"id"`
			Kind  string `json:"kind"`
			Title string `json:"title"`
		} `json:"pending_drafts"`
		OpenFindings []struct {
			ID string `json:"id"`
		} `json:"open_findings"`
		OpenPersonaCandidates []struct {
			ID            string `json:"id"`
			Field         string `json:"field"`
			ProposedValue string `json:"proposed_value"`
		} `json:"open_persona_candidates"`
		TodayUsage struct {
			Calls       int `json:"calls"`
			TotalTokens int `json:"total_tokens"`
		} `json:"today_usage"`
	}
	if err := json.Unmarshal([]byte(out), &got); err != nil {
		t.Fatalf("invalid JSON: %v\n%s", err, out)
	}
	if got.Day != "2026-06-04" || got.ActionItems != 3 {
		t.Fatalf("day/action_items = %q/%d, want 2026-06-04/3", got.Day, got.ActionItems)
	}
	if len(got.PendingDrafts) != 1 || got.PendingDrafts[0].ID != "draft-1" || got.PendingDrafts[0].Kind != string(model.DraftKindPersonaUpdate) {
		t.Fatalf("pending_drafts = %+v", got.PendingDrafts)
	}
	if len(got.OpenFindings) != 1 || got.OpenFindings[0].ID != "finding-1" {
		t.Fatalf("open_findings = %+v", got.OpenFindings)
	}
	if len(got.OpenPersonaCandidates) != 1 || got.OpenPersonaCandidates[0].Field != "major" || got.OpenPersonaCandidates[0].ProposedValue != "economics" {
		t.Fatalf("open_persona_candidates = %+v", got.OpenPersonaCandidates)
	}
	if got.TodayUsage.Calls != 4 || got.TodayUsage.TotalTokens != 1180 {
		t.Fatalf("today_usage = %+v, want 4/1180", got.TodayUsage)
	}
}

func TestOperatorQueueNudge(t *testing.T) {
	if operatorQueueNudge(0) != "" {
		t.Fatal("0 items must produce no nudge")
	}
	if operatorQueueNudge(-1) != "" {
		t.Fatal("negative must produce no nudge")
	}
	got := operatorQueueNudge(3)
	if !strings.Contains(got, "3 item") || !strings.Contains(got, "lore inbox") {
		t.Fatalf("nudge must name the count and `lore inbox`: %q", got)
	}
}
