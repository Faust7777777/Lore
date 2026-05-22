package store_test

import (
	"path/filepath"
	"testing"
	"time"

	"obsidian-harness/internal/model"
	"obsidian-harness/internal/store"
	"obsidian-harness/internal/store/jsonstore"
	"obsidian-harness/internal/store/memory"
	"obsidian-harness/internal/store/sqlitestore"
)

// usageBackendFactory mirrors backendFactory in persona_candidate_test.go
// for the UsageStore contract so B-P11a's PurposeBreakdown behavior is
// exercised on memory, jsonstore, and sqlitestore in one table.
type usageBackendFactory struct {
	name string
	open func(t *testing.T) store.UsageStore
}

func usageBackends(t *testing.T) []usageBackendFactory {
	t.Helper()
	return []usageBackendFactory{
		{
			name: "memory",
			open: func(_ *testing.T) store.UsageStore {
				return memory.New().Usage()
			},
		},
		{
			name: "jsonstore",
			open: func(t *testing.T) store.UsageStore {
				t.Helper()
				path := filepath.Join(t.TempDir(), "state.json")
				s, err := jsonstore.New(path, ".tmp")
				if err != nil {
					t.Fatalf("jsonstore.New: %v", err)
				}
				return s.Usage()
			},
		},
		{
			name: "sqlitestore",
			open: func(t *testing.T) store.UsageStore {
				t.Helper()
				path := filepath.Join(t.TempDir(), "state", "store.db")
				s, err := sqlitestore.New(path)
				if err != nil {
					t.Fatalf("sqlitestore.New: %v", err)
				}
				t.Cleanup(func() { _ = s.Close() })
				return s.Usage()
			},
		},
	}
}

func TestSummarizeUsageFillsPurposeBreakdown(t *testing.T) {
	day := time.Date(2026, 5, 20, 0, 0, 0, 0, time.UTC)
	for _, b := range usageBackends(t) {
		b := b
		t.Run(b.name, func(t *testing.T) {
			us := b.open(t)
			records := []model.UsageRecord{
				{RecordedAt: day.Add(1 * time.Hour), PromptTokens: 100, CompletionTokens: 50, Purpose: model.UsagePurposeChat},
				{RecordedAt: day.Add(2 * time.Hour), PromptTokens: 200, CompletionTokens: 80, Purpose: model.UsagePurposeChat},
				{RecordedAt: day.Add(3 * time.Hour), PromptTokens: 30, CompletionTokens: 20, Purpose: model.UsagePurposePersonaExtract},
				{RecordedAt: day.Add(4 * time.Hour), PromptTokens: 500, CompletionTokens: 200, Purpose: model.UsagePurposeProcessSink},
			}
			for _, r := range records {
				if err := us.AppendUsage(r); err != nil {
					t.Fatalf("AppendUsage: %v", err)
				}
			}

			summary, err := us.SummarizeUsage(day)
			if err != nil {
				t.Fatalf("SummarizeUsage: %v", err)
			}

			// Top-line totals stay byte-identical to pre-B-P11a so
			// existing `lore usage` callers and tests do not regress.
			if summary.Calls != 4 {
				t.Fatalf("Calls = %d, want 4", summary.Calls)
			}
			if summary.PromptTokens != 830 {
				t.Fatalf("PromptTokens = %d, want 830", summary.PromptTokens)
			}
			if summary.CompletionTokens != 350 {
				t.Fatalf("CompletionTokens = %d, want 350", summary.CompletionTokens)
			}
			if summary.TotalTokens != 1180 {
				t.Fatalf("TotalTokens = %d, want 1180", summary.TotalTokens)
			}

			// Three Purpose buckets exist and each carries the right
			// per-record sums.
			if len(summary.PurposeBreakdown) != 3 {
				t.Fatalf("PurposeBreakdown buckets = %d, want 3; got %+v", len(summary.PurposeBreakdown), summary.PurposeBreakdown)
			}
			chat := summary.PurposeBreakdown[model.UsagePurposeChat]
			if chat.Calls != 2 || chat.PromptTokens != 300 || chat.CompletionTokens != 130 {
				t.Fatalf("chat bucket = %+v, want {Calls:2 PromptTokens:300 CompletionTokens:130}", chat)
			}
			persona := summary.PurposeBreakdown[model.UsagePurposePersonaExtract]
			if persona.Calls != 1 || persona.PromptTokens != 30 || persona.CompletionTokens != 20 {
				t.Fatalf("persona_extract bucket = %+v, want {Calls:1 PromptTokens:30 CompletionTokens:20}", persona)
			}
			ps := summary.PurposeBreakdown[model.UsagePurposeProcessSink]
			if ps.Calls != 1 || ps.PromptTokens != 500 || ps.CompletionTokens != 200 {
				t.Fatalf("process_sink bucket = %+v, want {Calls:1 PromptTokens:500 CompletionTokens:200}", ps)
			}

			// Sum of all buckets equals the top-line totals so the
			// reconciliation contract documented on UsagePurposeStats
			// holds for every backend.
			var sumCalls, sumPrompt, sumCompletion int
			for _, stats := range summary.PurposeBreakdown {
				sumCalls += stats.Calls
				sumPrompt += stats.PromptTokens
				sumCompletion += stats.CompletionTokens
			}
			if sumCalls != summary.Calls {
				t.Fatalf("sum of bucket Calls (%d) != summary.Calls (%d)", sumCalls, summary.Calls)
			}
			if sumPrompt != summary.PromptTokens {
				t.Fatalf("sum of bucket PromptTokens (%d) != summary.PromptTokens (%d)", sumPrompt, summary.PromptTokens)
			}
			if sumCompletion != summary.CompletionTokens {
				t.Fatalf("sum of bucket CompletionTokens (%d) != summary.CompletionTokens (%d)", sumCompletion, summary.CompletionTokens)
			}
		})
	}
}

func TestSummarizeUsageBreakdownNilOnEmptyDay(t *testing.T) {
	// A day with zero records leaves PurposeBreakdown nil rather than
	// an empty map so JSON encodings omit the field via omitempty and
	// older callers that range over a nil map stay happy.
	day := time.Date(2026, 5, 20, 0, 0, 0, 0, time.UTC)
	for _, b := range usageBackends(t) {
		b := b
		t.Run(b.name, func(t *testing.T) {
			us := b.open(t)
			summary, err := us.SummarizeUsage(day)
			if err != nil {
				t.Fatalf("SummarizeUsage: %v", err)
			}
			if summary.Calls != 0 {
				t.Fatalf("Calls = %d, want 0 on empty day", summary.Calls)
			}
			if summary.PurposeBreakdown != nil {
				t.Fatalf("PurposeBreakdown = %+v, want nil on empty day", summary.PurposeBreakdown)
			}
		})
	}
}

func TestSummarizeUsageBreakdownEmptyPurposeCollapses(t *testing.T) {
	// Records without Purpose (pre-B-P11 / legacy) collapse into the
	// "" bucket so they remain visible in the breakdown even though
	// they predate the Purpose field. Backends MUST NOT drop them
	// silently or roll them into "chat" -- the empty bucket is the
	// honest answer.
	day := time.Date(2026, 5, 20, 0, 0, 0, 0, time.UTC)
	for _, b := range usageBackends(t) {
		b := b
		t.Run(b.name, func(t *testing.T) {
			us := b.open(t)
			if err := us.AppendUsage(model.UsageRecord{
				RecordedAt:       day.Add(1 * time.Hour),
				PromptTokens:     50,
				CompletionTokens: 25,
			}); err != nil {
				t.Fatalf("AppendUsage: %v", err)
			}
			summary, err := us.SummarizeUsage(day)
			if err != nil {
				t.Fatalf("SummarizeUsage: %v", err)
			}
			empty, ok := summary.PurposeBreakdown[""]
			if !ok {
				t.Fatalf("PurposeBreakdown missing empty-purpose bucket: %+v", summary.PurposeBreakdown)
			}
			if empty.Calls != 1 || empty.PromptTokens != 50 || empty.CompletionTokens != 25 {
				t.Fatalf("empty-purpose bucket = %+v, want {Calls:1 PromptTokens:50 CompletionTokens:25}", empty)
			}
		})
	}
}
