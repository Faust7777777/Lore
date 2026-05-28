package cli

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"

	"obsidian-harness/internal/model"
)

func TestRenderUsageReportShowsPurposeBreakdown(t *testing.T) {
	// B-P11a: when PurposeBreakdown is populated, the report adds a
	// "By purpose:" block aggregated across the queried window. Rows
	// are sorted alphabetically by Purpose so successive runs of
	// `lore usage` print identical output (map iteration is otherwise
	// unspecified and would flap CI on a per-purpose diff).
	day := time.Date(2026, 5, 21, 0, 0, 0, 0, time.UTC)
	daily := []model.UsageSummary{
		{
			Day:              day,
			Calls:            4,
			PromptTokens:     830,
			CompletionTokens: 350,
			TotalTokens:      1180,
			PurposeBreakdown: map[string]model.UsagePurposeStats{
				model.UsagePurposeChat:           {Calls: 2, PromptTokens: 300, CompletionTokens: 130},
				model.UsagePurposePersonaExtract: {Calls: 1, PromptTokens: 30, CompletionTokens: 20},
				model.UsagePurposeProcessSink:    {Calls: 1, PromptTokens: 500, CompletionTokens: 200},
			},
		},
	}

	var buf bytes.Buffer
	renderUsageReport(&buf, 1, daily)
	out := buf.String()

	if !strings.Contains(out, "By purpose:") {
		t.Fatalf("output missing By purpose block:\n%s", out)
	}
	for _, want := range []string{
		"chat",
		"persona_extract",
		"process_sink",
		"2 calls / 300 prompt + 130 completion = 430 tokens",
		"1 calls / 30 prompt + 20 completion = 50 tokens",
		"1 calls / 500 prompt + 200 completion = 700 tokens",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("output missing %q:\n%s", want, out)
		}
	}

	// Alphabetical order: chat < persona_extract < process_sink.
	chatIdx := strings.Index(out, "chat ")
	personaIdx := strings.Index(out, "persona_extract")
	psIdx := strings.Index(out, "process_sink")
	if chatIdx == -1 || personaIdx == -1 || psIdx == -1 {
		t.Fatalf("missing one of the purpose labels: chat=%d persona_extract=%d process_sink=%d\n%s", chatIdx, personaIdx, psIdx, out)
	}
	if !(chatIdx < personaIdx && personaIdx < psIdx) {
		t.Fatalf("purposes are not alphabetical: chat=%d persona_extract=%d process_sink=%d\n%s", chatIdx, personaIdx, psIdx, out)
	}
}

func TestRenderUsageReportHidesPurposeBlockWhenBreakdownNil(t *testing.T) {
	// Legacy records (Purpose unset across the window) leave the
	// PurposeBreakdown map nil. The "By purpose:" block must be
	// suppressed entirely rather than printing a blank header --
	// noise in the report is worse than no report.
	day := time.Date(2026, 5, 21, 0, 0, 0, 0, time.UTC)
	daily := []model.UsageSummary{
		{
			Day:              day,
			Calls:            3,
			PromptTokens:     100,
			CompletionTokens: 50,
			TotalTokens:      150,
		},
	}

	var buf bytes.Buffer
	renderUsageReport(&buf, 1, daily)
	out := buf.String()
	if strings.Contains(out, "By purpose:") {
		t.Fatalf("output unexpectedly contains By purpose block when breakdown is nil:\n%s", out)
	}
	if !strings.Contains(out, "Total: 3 calls") {
		t.Fatalf("output missing top-line totals:\n%s", out)
	}
}

func TestRenderUsageReportRendersEmptyPurposeAsUnspecified(t *testing.T) {
	// Records with Purpose == "" (pre-B-P11 legacy data) MUST still
	// show up in the breakdown -- silently dropping them would hide
	// real cost from the operator. We render them under the
	// "unspecified" label rather than an empty cell so the row is
	// self-describing.
	day := time.Date(2026, 5, 21, 0, 0, 0, 0, time.UTC)
	daily := []model.UsageSummary{
		{
			Day:              day,
			Calls:            1,
			PromptTokens:     50,
			CompletionTokens: 25,
			TotalTokens:      75,
			PurposeBreakdown: map[string]model.UsagePurposeStats{
				"": {Calls: 1, PromptTokens: 50, CompletionTokens: 25},
			},
		},
	}

	var buf bytes.Buffer
	renderUsageReport(&buf, 1, daily)
	out := buf.String()
	if !strings.Contains(out, "By purpose:") {
		t.Fatalf("output missing By purpose block:\n%s", out)
	}
	if !strings.Contains(out, "unspecified") {
		t.Fatalf("empty Purpose should render as 'unspecified':\n%s", out)
	}
}

func TestRenderUsageReportShowsModelDetailUnderPurpose(t *testing.T) {
	// B-next: when a purpose bucket carries ByModel data, the human
	// report prints indented per-model lines beneath the purpose line
	// so an operator can see chat went to deepseek vs kimi.
	day := time.Date(2026, 5, 21, 0, 0, 0, 0, time.UTC)
	daily := []model.UsageSummary{
		{
			Day:              day,
			Calls:            3,
			PromptTokens:     180,
			CompletionTokens: 70,
			TotalTokens:      250,
			PurposeBreakdown: map[string]model.UsagePurposeStats{
				model.UsagePurposeChat: {
					Calls: 3, PromptTokens: 180, CompletionTokens: 70,
					ByModel: map[string]model.UsagePurposeStats{
						"deepseek/deepseek-v4-pro": {Calls: 2, PromptTokens: 150, CompletionTokens: 60},
						"kimi/kimi-k2":             {Calls: 1, PromptTokens: 30, CompletionTokens: 10},
					},
				},
			},
		},
	}

	var buf bytes.Buffer
	renderUsageReport(&buf, 1, daily)
	out := buf.String()
	for _, want := range []string{
		"deepseek/deepseek-v4-pro",
		"2 calls / 150 prompt + 60 completion = 210 tokens",
		"kimi/kimi-k2",
		"1 calls / 30 prompt + 10 completion = 40 tokens",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("output missing model-detail %q:\n%s", want, out)
		}
	}
	// Model lines are indented deeper than the purpose line so the
	// hierarchy reads visually. The purpose line uses 2-space indent;
	// the model lines use 4.
	if !strings.Contains(out, "    deepseek/deepseek-v4-pro") {
		t.Fatalf("model detail should be indented 4 spaces under the purpose:\n%s", out)
	}
	// Higher-token model sorts first (deepseek 210 > kimi 40).
	if strings.Index(out, "deepseek/deepseek-v4-pro") > strings.Index(out, "kimi/kimi-k2") {
		t.Fatalf("models should sort by total tokens desc (deepseek before kimi):\n%s", out)
	}
}

func TestRenderUsageReportCapsModelDetailAtTopFive(t *testing.T) {
	// A purpose touching more than five models (plausible after a day
	// of hot-switching) prints only the top five by tokens plus a
	// "... N more model(s)" line so the report stays bounded.
	byModel := map[string]model.UsagePurposeStats{}
	for i := 0; i < 8; i++ {
		// Descending token weight so ordering is deterministic and the
		// dropped ones are the smallest.
		byModel[fmt.Sprintf("prov/model-%d", i)] = model.UsagePurposeStats{
			Calls: 1, PromptTokens: 100 - i*5, CompletionTokens: 0,
		}
	}
	daily := []model.UsageSummary{
		{
			Day:              time.Date(2026, 5, 21, 0, 0, 0, 0, time.UTC),
			Calls:            8,
			PromptTokens:     540,
			CompletionTokens: 0,
			TotalTokens:      540,
			PurposeBreakdown: map[string]model.UsagePurposeStats{
				model.UsagePurposeChat: {Calls: 8, PromptTokens: 540, ByModel: byModel},
			},
		},
	}

	var buf bytes.Buffer
	renderUsageReport(&buf, 1, daily)
	out := buf.String()
	if !strings.Contains(out, "... 3 more model(s)") {
		t.Fatalf("expected a '... 3 more model(s)' summary line for 8 models capped at 5:\n%s", out)
	}
	// model-0 (largest) shown; model-7 (smallest) dropped.
	if !strings.Contains(out, "prov/model-0") {
		t.Fatalf("top model should be shown:\n%s", out)
	}
	if strings.Contains(out, "prov/model-7") {
		t.Fatalf("smallest model should be dropped past the top-5 cap:\n%s", out)
	}
}

func TestEmitUsageJSONIncludesByModelBreakdown(t *testing.T) {
	// JSON mode carries the uncapped by_model map so scripting / TUI
	// consumers can build a full cost dashboard. Shape:
	// {window_days, totals, purpose_breakdown.<p>.by_model.<prov/model>, days}.
	day := time.Date(2026, 5, 21, 0, 0, 0, 0, time.UTC)
	daily := []model.UsageSummary{
		{
			Day:              day,
			Calls:            3,
			PromptTokens:     180,
			CompletionTokens: 70,
			TotalTokens:      250,
			PurposeBreakdown: map[string]model.UsagePurposeStats{
				model.UsagePurposeChat: {
					Calls: 3, PromptTokens: 180, CompletionTokens: 70,
					ByModel: map[string]model.UsagePurposeStats{
						"deepseek/deepseek-v4-pro": {Calls: 2, PromptTokens: 150, CompletionTokens: 60},
						"kimi/kimi-k2":             {Calls: 1, PromptTokens: 30, CompletionTokens: 10},
					},
				},
			},
		},
	}

	var buf bytes.Buffer
	if err := emitUsageJSON(&buf, 1, daily); err != nil {
		t.Fatalf("emitUsageJSON: %v", err)
	}
	out := strings.TrimSpace(buf.String())
	if strings.Count(out, "\n") != 0 {
		t.Fatalf("JSON should be a single line:\n%s", out)
	}
	var got struct {
		WindowDays int `json:"window_days"`
		Totals     struct {
			Calls       int `json:"calls"`
			TotalTokens int `json:"total_tokens"`
		} `json:"totals"`
		PurposeBreakdown map[string]struct {
			Calls       int `json:"calls"`
			TotalTokens int `json:"total_tokens"`
			ByModel     map[string]struct {
				Calls            int `json:"calls"`
				PromptTokens     int `json:"prompt_tokens"`
				CompletionTokens int `json:"completion_tokens"`
				TotalTokens      int `json:"total_tokens"`
			} `json:"by_model"`
		} `json:"purpose_breakdown"`
		Days []struct {
			Day   string `json:"day"`
			Calls int    `json:"calls"`
		} `json:"days"`
	}
	if err := json.Unmarshal([]byte(out), &got); err != nil {
		t.Fatalf("invalid JSON: %v\n%s", err, out)
	}
	if got.WindowDays != 1 {
		t.Fatalf("window_days = %d, want 1", got.WindowDays)
	}
	if got.Totals.Calls != 3 || got.Totals.TotalTokens != 250 {
		t.Fatalf("totals = %+v, want calls=3 total=250", got.Totals)
	}
	chat, ok := got.PurposeBreakdown["chat"]
	if !ok {
		t.Fatalf("purpose_breakdown missing chat: %+v", got.PurposeBreakdown)
	}
	ds, ok := chat.ByModel["deepseek/deepseek-v4-pro"]
	if !ok {
		t.Fatalf("chat.by_model missing deepseek: %+v", chat.ByModel)
	}
	if ds.Calls != 2 || ds.PromptTokens != 150 || ds.CompletionTokens != 60 || ds.TotalTokens != 210 {
		t.Fatalf("deepseek by_model = %+v, want calls=2 prompt=150 completion=60 total=210", ds)
	}
	if len(got.Days) != 1 || got.Days[0].Day != "2026-05-21" || got.Days[0].Calls != 3 {
		t.Fatalf("days = %+v, want one day 2026-05-21 with 3 calls", got.Days)
	}
}

func TestRenderUsageReportAggregatesAcrossDays(t *testing.T) {
	// Multi-day windows aggregate breakdowns into one block at the
	// bottom so the operator sees the trailing-N-days cost split
	// without having to add columns per purpose into every DAY row.
	day1 := time.Date(2026, 5, 20, 0, 0, 0, 0, time.UTC)
	day2 := time.Date(2026, 5, 21, 0, 0, 0, 0, time.UTC)
	daily := []model.UsageSummary{
		{
			Day:              day1,
			Calls:            2,
			PromptTokens:     200,
			CompletionTokens: 100,
			TotalTokens:      300,
			PurposeBreakdown: map[string]model.UsagePurposeStats{
				model.UsagePurposeChat: {Calls: 2, PromptTokens: 200, CompletionTokens: 100},
			},
		},
		{
			Day:              day2,
			Calls:            3,
			PromptTokens:     150,
			CompletionTokens: 80,
			TotalTokens:      230,
			PurposeBreakdown: map[string]model.UsagePurposeStats{
				model.UsagePurposeChat:           {Calls: 1, PromptTokens: 100, CompletionTokens: 50},
				model.UsagePurposePersonaExtract: {Calls: 2, PromptTokens: 50, CompletionTokens: 30},
			},
		},
	}

	var buf bytes.Buffer
	renderUsageReport(&buf, 2, daily)
	out := buf.String()
	// Aggregated chat = day1's 2 + day2's 1 = 3 calls / 300 prompt + 150 completion.
	if !strings.Contains(out, "3 calls / 300 prompt + 150 completion = 450 tokens") {
		t.Fatalf("aggregated chat line missing:\n%s", out)
	}
	if !strings.Contains(out, "2 calls / 50 prompt + 30 completion = 80 tokens") {
		t.Fatalf("aggregated persona_extract line missing:\n%s", out)
	}
}
