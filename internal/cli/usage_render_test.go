package cli

import (
	"bytes"
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
