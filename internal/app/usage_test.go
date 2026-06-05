package app

import (
	"testing"
	"time"

	"obsidian-harness/internal/config"
	"obsidian-harness/internal/model"
	"obsidian-harness/internal/store/memory"
)

// trackingConfig enables usage.track_usage so bare test Runtimes record
// usage; a zero-value Config has TrackUsage=false, which now (correctly)
// disables recording.
func trackingConfig() config.Config {
	return config.Config{Usage: config.UsageConfig{TrackUsage: true}}
}

func TestRuntimeRecordUsageAppendsAllRecords(t *testing.T) {
	store := memory.New()
	runtime := &Runtime{Store: store, Config: trackingConfig()}

	day := time.Date(2026, 4, 22, 9, 0, 0, 0, time.UTC)
	records := []model.UsageRecord{
		{Provider: "openai-compatible", Model: "gpt-x", AgentID: "codex", SessionID: "s1", PromptTokens: 30, CompletionTokens: 4, RecordedAt: day},
		{Provider: "openai-compatible", Model: "gpt-x", AgentID: "codex", SessionID: "s1", PromptTokens: 55, CompletionTokens: 12, RecordedAt: day.Add(time.Second)},
	}
	if err := runtime.RecordUsage(records); err != nil {
		t.Fatalf("RecordUsage() error = %v", err)
	}

	summary, err := store.Usage().SummarizeUsage(day)
	if err != nil {
		t.Fatalf("SummarizeUsage() error = %v", err)
	}
	if summary.Calls != 2 {
		t.Fatalf("summary.Calls = %d, want 2", summary.Calls)
	}
	if summary.PromptTokens != 85 || summary.CompletionTokens != 16 {
		t.Fatalf("summary tokens = %d/%d, want 85/16", summary.PromptTokens, summary.CompletionTokens)
	}
	if summary.TotalTokens != 101 {
		t.Fatalf("summary.TotalTokens = %d, want 101", summary.TotalTokens)
	}
}

func TestRuntimeRecordUsageNormalizesPurposeBuckets(t *testing.T) {
	store := memory.New()
	runtime := &Runtime{Store: store, Config: trackingConfig()}

	day := time.Date(2026, 6, 5, 9, 0, 0, 0, time.UTC)
	// Casing/whitespace variants of the same purpose must collapse into one
	// bucket, not fan out into "Persona_Extract" + " persona_extract "
	// (review-v1 P2-6).
	records := []model.UsageRecord{
		{Model: "m", Purpose: "Persona_Extract", PromptTokens: 1, RecordedAt: day},
		{Model: "m", Purpose: " persona_extract ", PromptTokens: 2, RecordedAt: day},
	}
	if err := runtime.RecordUsage(records); err != nil {
		t.Fatalf("RecordUsage() error = %v", err)
	}
	summary, err := store.Usage().SummarizeUsage(day)
	if err != nil {
		t.Fatalf("SummarizeUsage() error = %v", err)
	}
	if len(summary.PurposeBreakdown) != 1 {
		t.Fatalf("want exactly 1 purpose bucket (variants collapsed), got %d: %v",
			len(summary.PurposeBreakdown), summary.PurposeBreakdown)
	}
	bucket, ok := summary.PurposeBreakdown[model.UsagePurposePersonaExtract]
	if !ok {
		t.Fatalf("expected a %q bucket, got %v", model.UsagePurposePersonaExtract, summary.PurposeBreakdown)
	}
	if bucket.Calls != 2 {
		t.Fatalf("persona_extract bucket Calls = %d, want 2", bucket.Calls)
	}
}

func TestRuntimeRecordUsageEmptyIsNoOp(t *testing.T) {
	runtime := &Runtime{Store: memory.New(), Config: trackingConfig()}
	if err := runtime.RecordUsage(nil); err != nil {
		t.Fatalf("RecordUsage(nil) error = %v", err)
	}
	if err := runtime.RecordUsage([]model.UsageRecord{}); err != nil {
		t.Fatalf("RecordUsage(empty) error = %v", err)
	}
}

// TestRuntimeSummarizeUsageBucketsAcrossUTCBoundary is the regression
// reviewer asked for: a UTC RecordedAt that falls on the previous UTC
// day must still be counted against the local "today" the CLI queries
// for. Without NormalizeUsageDay this scenario silently dropped the
// usage near every Asia midnight.
func TestRuntimeSummarizeUsageBucketsAcrossUTCBoundary(t *testing.T) {
	originalLocal := time.Local
	t.Cleanup(func() { time.Local = originalLocal })
	time.Local = time.FixedZone("CST", 8*3600)

	runtime := &Runtime{Store: memory.New(), Config: trackingConfig()}
	// 2026-05-17 16:30 UTC == 2026-05-18 00:30 +0800 local.
	recordedUTC := time.Date(2026, 5, 17, 16, 30, 0, 0, time.UTC)
	if err := runtime.RecordUsage([]model.UsageRecord{{
		Provider: "openai-compatible", Model: "fake",
		AgentID: "codex", SessionID: "s1",
		PromptTokens: 30, CompletionTokens: 4,
		RecordedAt: recordedUTC,
	}}); err != nil {
		t.Fatalf("RecordUsage() error = %v", err)
	}

	// CLI semantics: time.Now() returns local; here a user opens
	// `lore usage` at 2026-05-18 00:30 local.
	queryToday := time.Date(2026, 5, 18, 0, 30, 0, 0, time.Local)
	summary, err := runtime.SummarizeUsage(queryToday)
	if err != nil {
		t.Fatalf("SummarizeUsage(today) error = %v", err)
	}
	if summary.Calls != 1 || summary.PromptTokens != 30 || summary.CompletionTokens != 4 {
		t.Fatalf("today summary = %+v, want 1 call / 30 prompt / 4 completion", summary)
	}
	if got := summary.Day.In(time.Local).Format("2006-01-02"); got != "2026-05-18" {
		t.Fatalf("summary.Day = %q, want 2026-05-18 local", got)
	}

	// A query for the previous local day must miss the record.
	queryYesterday := time.Date(2026, 5, 17, 12, 0, 0, 0, time.Local)
	prev, err := runtime.SummarizeUsage(queryYesterday)
	if err != nil {
		t.Fatalf("SummarizeUsage(yesterday) error = %v", err)
	}
	if prev.Calls != 0 {
		t.Fatalf("yesterday summary should be empty, got %+v", prev)
	}
}

func TestRuntimeRecordUsageRespectsTrackUsageDisabled(t *testing.T) {
	// usage.track_usage = false must actually disable recording: an
	// operator who opts out of billing capture should get zero usage rows.
	// Previously the flag was parsed/validated but never enforced, so
	// RecordUsage wrote rows regardless. RecordUsage is the single gate so
	// chat, persona_extract, and process_sink billers all honour it.
	store := memory.New()
	runtime := &Runtime{Store: store, Config: config.Config{Usage: config.UsageConfig{TrackUsage: false}}}

	day := time.Date(2026, 4, 22, 9, 0, 0, 0, time.UTC)
	if err := runtime.RecordUsage([]model.UsageRecord{
		{Provider: "openai-compatible", Model: "gpt-x", AgentID: "codex", SessionID: "s1", PromptTokens: 30, CompletionTokens: 4, RecordedAt: day},
	}); err != nil {
		t.Fatalf("RecordUsage() error = %v", err)
	}

	summary, err := store.Usage().SummarizeUsage(day)
	if err != nil {
		t.Fatalf("SummarizeUsage() error = %v", err)
	}
	if summary.Calls != 0 || summary.TotalTokens != 0 {
		t.Fatalf("track_usage=false should record nothing, got %d calls / %d tokens", summary.Calls, summary.TotalTokens)
	}
}
