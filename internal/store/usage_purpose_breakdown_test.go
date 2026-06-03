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

func TestSummarizeUsageBreakdownEmptyPurposeFoldsIntoChat(t *testing.T) {
	// Records without Purpose (pre-B-P11 / legacy) fold into the
	// "chat" bucket: before the Purpose field existed the operator
	// agent was the only biller, so an unlabeled call is a chat call.
	// A real chat-labeled record in the same day must merge with them
	// rather than splitting into a separate bucket, so the breakdown
	// has exactly one "chat" bucket and no "" bucket.
	day := time.Date(2026, 5, 20, 0, 0, 0, 0, time.UTC)
	for _, b := range usageBackends(t) {
		b := b
		t.Run(b.name, func(t *testing.T) {
			us := b.open(t)
			// One legacy (no Purpose) + one explicit chat record.
			for _, r := range []model.UsageRecord{
				{RecordedAt: day.Add(1 * time.Hour), PromptTokens: 50, CompletionTokens: 25},
				{RecordedAt: day.Add(2 * time.Hour), PromptTokens: 10, CompletionTokens: 5, Purpose: model.UsagePurposeChat},
			} {
				if err := us.AppendUsage(r); err != nil {
					t.Fatalf("AppendUsage: %v", err)
				}
			}
			summary, err := us.SummarizeUsage(day)
			if err != nil {
				t.Fatalf("SummarizeUsage: %v", err)
			}
			if _, ok := summary.PurposeBreakdown[""]; ok {
				t.Fatalf("PurposeBreakdown must not carry an empty-string bucket: %+v", summary.PurposeBreakdown)
			}
			chat, ok := summary.PurposeBreakdown[model.UsagePurposeChat]
			if !ok {
				t.Fatalf("PurposeBreakdown missing chat bucket: %+v", summary.PurposeBreakdown)
			}
			if chat.Calls != 2 || chat.PromptTokens != 60 || chat.CompletionTokens != 30 {
				t.Fatalf("chat bucket = %+v, want {Calls:2 PromptTokens:60 CompletionTokens:30} (legacy + explicit merged)", chat)
			}
		})
	}
}

func TestSummarizeUsageByModelSplitsSamePurposeAcrossModels(t *testing.T) {
	// Two models billing the same purpose must land in distinct
	// by_model sub-buckets keyed provider/model, and their token sums
	// must reconcile against the parent purpose bucket.
	day := time.Date(2026, 5, 20, 0, 0, 0, 0, time.UTC)
	for _, b := range usageBackends(t) {
		b := b
		t.Run(b.name, func(t *testing.T) {
			us := b.open(t)
			for _, r := range []model.UsageRecord{
				{RecordedAt: day.Add(1 * time.Hour), Provider: "deepseek", Model: "deepseek-v4-pro", PromptTokens: 100, CompletionTokens: 40, Purpose: model.UsagePurposeChat},
				{RecordedAt: day.Add(2 * time.Hour), Provider: "deepseek", Model: "deepseek-v4-pro", PromptTokens: 50, CompletionTokens: 20, Purpose: model.UsagePurposeChat},
				{RecordedAt: day.Add(3 * time.Hour), Provider: "kimi", Model: "kimi-k2", PromptTokens: 30, CompletionTokens: 10, Purpose: model.UsagePurposeChat},
			} {
				if err := us.AppendUsage(r); err != nil {
					t.Fatalf("AppendUsage: %v", err)
				}
			}
			summary, err := us.SummarizeUsage(day)
			if err != nil {
				t.Fatalf("SummarizeUsage: %v", err)
			}
			chat := summary.PurposeBreakdown[model.UsagePurposeChat]
			if chat.Calls != 3 {
				t.Fatalf("chat.Calls = %d, want 3", chat.Calls)
			}
			if len(chat.ByModel) != 2 {
				t.Fatalf("chat.ByModel buckets = %d, want 2; got %+v", len(chat.ByModel), chat.ByModel)
			}
			ds := chat.ByModel["deepseek/deepseek-v4-pro"]
			if ds.Calls != 2 || ds.PromptTokens != 150 || ds.CompletionTokens != 60 {
				t.Fatalf("deepseek by_model = %+v, want {Calls:2 PromptTokens:150 CompletionTokens:60}", ds)
			}
			kimi := chat.ByModel["kimi/kimi-k2"]
			if kimi.Calls != 1 || kimi.PromptTokens != 30 || kimi.CompletionTokens != 10 {
				t.Fatalf("kimi by_model = %+v, want {Calls:1 PromptTokens:30 CompletionTokens:10}", kimi)
			}
			// Reconcile: sum of by_model equals the parent purpose stats.
			var c, p, comp int
			for _, leaf := range chat.ByModel {
				c += leaf.Calls
				p += leaf.PromptTokens
				comp += leaf.CompletionTokens
			}
			if c != chat.Calls || p != chat.PromptTokens || comp != chat.CompletionTokens {
				t.Fatalf("by_model sum (%d/%d/%d) != chat bucket (%d/%d/%d)", c, p, comp, chat.Calls, chat.PromptTokens, chat.CompletionTokens)
			}
		})
	}
}

func TestSummarizeUsageByModelDoesNotMixAcrossPurposes(t *testing.T) {
	// The same provider/model billing two different purposes must keep
	// separate by_model entries under each purpose bucket -- a chat
	// call and a persona_extract call on deepseek-v4-pro are different
	// cost lines even though the model is identical.
	day := time.Date(2026, 5, 20, 0, 0, 0, 0, time.UTC)
	for _, b := range usageBackends(t) {
		b := b
		t.Run(b.name, func(t *testing.T) {
			us := b.open(t)
			for _, r := range []model.UsageRecord{
				{RecordedAt: day.Add(1 * time.Hour), Provider: "deepseek", Model: "deepseek-v4-pro", PromptTokens: 100, CompletionTokens: 40, Purpose: model.UsagePurposeChat},
				{RecordedAt: day.Add(2 * time.Hour), Provider: "deepseek", Model: "deepseek-v4-pro", PromptTokens: 10, CompletionTokens: 5, Purpose: model.UsagePurposePersonaExtract},
			} {
				if err := us.AppendUsage(r); err != nil {
					t.Fatalf("AppendUsage: %v", err)
				}
			}
			summary, err := us.SummarizeUsage(day)
			if err != nil {
				t.Fatalf("SummarizeUsage: %v", err)
			}
			chatModel := summary.PurposeBreakdown[model.UsagePurposeChat].ByModel["deepseek/deepseek-v4-pro"]
			if chatModel.Calls != 1 || chatModel.PromptTokens != 100 {
				t.Fatalf("chat deepseek by_model = %+v, want {Calls:1 PromptTokens:100 ...}", chatModel)
			}
			personaModel := summary.PurposeBreakdown[model.UsagePurposePersonaExtract].ByModel["deepseek/deepseek-v4-pro"]
			if personaModel.Calls != 1 || personaModel.PromptTokens != 10 {
				t.Fatalf("persona deepseek by_model = %+v, want {Calls:1 PromptTokens:10 ...}", personaModel)
			}
		})
	}
}

func TestSummarizeUsageByModelFallsBackToUnknown(t *testing.T) {
	// A record missing provider AND model folds into the "unknown"
	// by_model key; a record missing only the provider keeps the model
	// half ("unknown/<model>"). Guards the fallback rules in
	// UsageModelBucket across all backends.
	day := time.Date(2026, 5, 20, 0, 0, 0, 0, time.UTC)
	for _, b := range usageBackends(t) {
		b := b
		t.Run(b.name, func(t *testing.T) {
			us := b.open(t)
			for _, r := range []model.UsageRecord{
				{RecordedAt: day.Add(1 * time.Hour), PromptTokens: 5, CompletionTokens: 2, Purpose: model.UsagePurposeChat},
				{RecordedAt: day.Add(2 * time.Hour), Model: "orphan-model", PromptTokens: 7, CompletionTokens: 3, Purpose: model.UsagePurposeChat},
			} {
				if err := us.AppendUsage(r); err != nil {
					t.Fatalf("AppendUsage: %v", err)
				}
			}
			summary, err := us.SummarizeUsage(day)
			if err != nil {
				t.Fatalf("SummarizeUsage: %v", err)
			}
			byModel := summary.PurposeBreakdown[model.UsagePurposeChat].ByModel
			if _, ok := byModel["unknown"]; !ok {
				t.Fatalf("missing 'unknown' by_model bucket for fully-unidentified record: %+v", byModel)
			}
			if _, ok := byModel["unknown/orphan-model"]; !ok {
				t.Fatalf("missing 'unknown/orphan-model' bucket for provider-less record: %+v", byModel)
			}
		})
	}
}
