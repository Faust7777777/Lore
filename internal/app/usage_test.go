package app

import (
	"testing"
	"time"

	"obsidian-harness/internal/model"
	"obsidian-harness/internal/store/memory"
)

func TestRuntimeRecordUsageAppendsAllRecords(t *testing.T) {
	store := memory.New()
	runtime := &Runtime{Store: store}

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

func TestRuntimeRecordUsageEmptyIsNoOp(t *testing.T) {
	runtime := &Runtime{Store: memory.New()}
	if err := runtime.RecordUsage(nil); err != nil {
		t.Fatalf("RecordUsage(nil) error = %v", err)
	}
	if err := runtime.RecordUsage([]model.UsageRecord{}); err != nil {
		t.Fatalf("RecordUsage(empty) error = %v", err)
	}
}
