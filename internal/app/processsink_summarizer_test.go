package app

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"obsidian-harness/internal/adapter/codexjsonl"
	openai "obsidian-harness/internal/llm/openai"
	"obsidian-harness/internal/model"
)

type stubProcessSinkClient struct {
	resp openai.ChatCompletionResponse
	err  error
	req  openai.ChatCompletionRequest
}

func (s *stubProcessSinkClient) ChatCompletion(_ context.Context, req openai.ChatCompletionRequest) (openai.ChatCompletionResponse, error) {
	s.req = req
	return s.resp, s.err
}

func newCheckpointWindow() codexjsonl.WindowSummary {
	return codexjsonl.WindowSummary{
		Window: model.SessionWindow{
			AgentID:     "codex",
			SessionID:   "sess-1",
			WindowStart: time.Date(2026, 4, 22, 9, 0, 0, 0, time.UTC),
			WindowEnd:   time.Date(2026, 4, 22, 9, 30, 0, 0, time.UTC),
		},
		Content:       "- source: demo\n",
		RawTranscript: "{\"role\":\"user\",\"message\":\"hello\"}\n",
		EventCount:    1,
	}
}

func TestModelProcessSinkSummarizerCheckpointEmitsUsageWithWindowIdentifiers(t *testing.T) {
	client := &stubProcessSinkClient{
		resp: openai.ChatCompletionResponse{
			Content:          `{"title":"t","content":"c"}`,
			PromptTokens:     31,
			CompletionTokens: 9,
		},
	}
	var sunk []model.UsageRecord
	s := &modelProcessSinkSummarizer{
		client:    client,
		provider:  "test",
		model:     "test-model",
		usageSink: func(rec model.UsageRecord) error { sunk = append(sunk, rec); return nil },
	}

	before := time.Now().UTC().Add(-time.Second)
	title, content, err := s.SummarizeCheckpoint(newCheckpointWindow())
	if err != nil {
		t.Fatalf("SummarizeCheckpoint() error = %v", err)
	}
	if title != "t" || content != "c" {
		t.Fatalf("summary = (%q, %q), want (t, c)", title, content)
	}
	if len(sunk) != 1 {
		t.Fatalf("usage records = %d, want 1; got %+v", len(sunk), sunk)
	}
	rec := sunk[0]
	if rec.Provider != "test" || rec.Model != "test-model" {
		t.Fatalf("provider/model = %q/%q", rec.Provider, rec.Model)
	}
	// Purpose must be stamped: a dropped/empty Purpose silently folds
	// process-sink spend into the chat bucket (empty -> chat), so the
	// cost report would misattribute it with no other signal. The
	// persona extractor test guards this symmetrically.
	if rec.Purpose != model.UsagePurposeProcessSink {
		t.Fatalf("Purpose = %q, want %q", rec.Purpose, model.UsagePurposeProcessSink)
	}
	if rec.AgentID != "codex" || rec.SessionID != "sess-1" {
		t.Fatalf("agent/session = %q/%q, want codex/sess-1", rec.AgentID, rec.SessionID)
	}
	if rec.PromptTokens != 31 || rec.CompletionTokens != 9 {
		t.Fatalf("tokens = %d/%d, want 31/9", rec.PromptTokens, rec.CompletionTokens)
	}
	if rec.RecordedAt.Before(before) || rec.RecordedAt.After(time.Now().UTC().Add(time.Second)) {
		t.Fatalf("RecordedAt = %v outside recent window", rec.RecordedAt)
	}
}

func TestModelProcessSinkSummarizerDailyEmitsUsageWithEmptySessionID(t *testing.T) {
	client := &stubProcessSinkClient{
		resp: openai.ChatCompletionResponse{
			Content:          `{"title":"day","content":"body"}`,
			PromptTokens:     12,
			CompletionTokens: 3,
		},
	}
	var sunk []model.UsageRecord
	s := &modelProcessSinkSummarizer{
		client:    client,
		provider:  "test",
		model:     "test-model",
		usageSink: func(rec model.UsageRecord) error { sunk = append(sunk, rec); return nil },
	}

	day := time.Date(2026, 4, 22, 0, 0, 0, 0, time.UTC)
	checkpoints := []model.CheckpointDoc{{
		Window:  model.SessionWindow{AgentID: "codex", WindowStart: day.Add(9 * time.Hour), WindowEnd: day.Add(9*time.Hour + 30*time.Minute)},
		Title:   "ck",
		Content: "body",
		State:   "primed",
	}}
	if _, _, err := s.SummarizeDaily("codex", day, checkpoints); err != nil {
		t.Fatalf("SummarizeDaily() error = %v", err)
	}
	if len(sunk) != 1 {
		t.Fatalf("usage records = %d, want 1", len(sunk))
	}
	rec := sunk[0]
	if rec.AgentID != "codex" {
		t.Fatalf("agent = %q, want codex", rec.AgentID)
	}
	if rec.SessionID != "" {
		t.Fatalf("daily SessionID = %q, want empty", rec.SessionID)
	}
	if rec.PromptTokens != 12 || rec.CompletionTokens != 3 {
		t.Fatalf("tokens = %d/%d, want 12/3", rec.PromptTokens, rec.CompletionTokens)
	}
}

func TestModelProcessSinkSummarizerEmitsUsageEvenWhenParseFails(t *testing.T) {
	client := &stubProcessSinkClient{
		resp: openai.ChatCompletionResponse{
			Content:          `not json`,
			PromptTokens:     7,
			CompletionTokens: 2,
		},
	}
	var sunk []model.UsageRecord
	s := &modelProcessSinkSummarizer{
		client:    client,
		provider:  "test",
		model:     "test-model",
		usageSink: func(rec model.UsageRecord) error { sunk = append(sunk, rec); return nil },
	}

	_, _, err := s.SummarizeCheckpoint(newCheckpointWindow())
	if err == nil {
		t.Fatal("SummarizeCheckpoint() error = nil, want parse failure")
	}
	if len(sunk) != 1 {
		t.Fatalf("usage records = %d, want 1 (cost must be recorded even on parse failure)", len(sunk))
	}
	if sunk[0].PromptTokens != 7 || sunk[0].CompletionTokens != 2 {
		t.Fatalf("tokens = %d/%d, want 7/2", sunk[0].PromptTokens, sunk[0].CompletionTokens)
	}
}

func TestModelProcessSinkSummarizerSkipsUsageWhenChatCompletionFails(t *testing.T) {
	client := &stubProcessSinkClient{err: errors.New("upstream timeout")}
	var sunk []model.UsageRecord
	s := &modelProcessSinkSummarizer{
		client:    client,
		provider:  "test",
		model:     "test-model",
		usageSink: func(rec model.UsageRecord) error { sunk = append(sunk, rec); return nil },
	}

	if _, _, err := s.SummarizeCheckpoint(newCheckpointWindow()); err == nil {
		t.Fatal("SummarizeCheckpoint() error = nil, want upstream error")
	}
	if len(sunk) != 0 {
		t.Fatalf("usage records = %d, want 0 (no record when call itself fails)", len(sunk))
	}
}

func TestModelProcessSinkSummarizerSwallowsSinkErrors(t *testing.T) {
	client := &stubProcessSinkClient{
		resp: openai.ChatCompletionResponse{
			Content:          `{"title":"t","content":"c"}`,
			PromptTokens:     1,
			CompletionTokens: 1,
		},
	}
	s := &modelProcessSinkSummarizer{
		client:    client,
		provider:  "test",
		model:     "test-model",
		usageSink: func(rec model.UsageRecord) error { return fmt.Errorf("disk full") },
	}

	if _, _, err := s.SummarizeCheckpoint(newCheckpointWindow()); err != nil {
		t.Fatalf("SummarizeCheckpoint() error = %v, want nil (sink failure must not break summarization)", err)
	}
}

func TestAttachUsageSinkIsNoOpForNonModelSummarizers(t *testing.T) {
	called := false
	attachUsageSink(&fakeProcessSinkSummarizer{}, func(model.UsageRecord) error {
		called = true
		return nil
	})
	if called {
		t.Fatal("attachUsageSink should not invoke the sink for non-model summarizers")
	}
}

func TestAttachUsageSinkWiresModelBackedSummarizer(t *testing.T) {
	client := &stubProcessSinkClient{
		resp: openai.ChatCompletionResponse{
			Content:          `{"title":"t","content":"c"}`,
			PromptTokens:     4,
			CompletionTokens: 2,
		},
	}
	s := &modelProcessSinkSummarizer{client: client, provider: "test", model: "test-model"}

	var sunk []model.UsageRecord
	attachUsageSink(s, func(rec model.UsageRecord) error { sunk = append(sunk, rec); return nil })

	if _, _, err := s.SummarizeCheckpoint(newCheckpointWindow()); err != nil {
		t.Fatalf("SummarizeCheckpoint() error = %v", err)
	}
	if len(sunk) != 1 {
		t.Fatalf("usage records = %d, want 1", len(sunk))
	}
}
