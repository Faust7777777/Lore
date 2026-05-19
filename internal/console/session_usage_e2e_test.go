package console

import (
	"context"
	"strings"
	"testing"
	"time"

	"obsidian-harness/internal/app"
	"obsidian-harness/internal/config/configtest"
	openai "obsidian-harness/internal/llm/openai"
	"obsidian-harness/internal/operatoragent"
)

// scriptedCompletionClient satisfies operatoragent's unexported
// completionClient interface via Go's structural typing. We define a
// local fake here (rather than reuse the operatoragent package's
// internal one) because internal test helpers are not exported.
type scriptedCompletionClient struct {
	responses []openai.ChatCompletionResponse
	errs      []error
	idx       int
}

func (c *scriptedCompletionClient) ChatCompletion(_ context.Context, _ openai.ChatCompletionRequest) (openai.ChatCompletionResponse, error) {
	if c.idx >= len(c.responses) {
		return openai.ChatCompletionResponse{}, nil
	}
	resp := c.responses[c.idx]
	var err error
	if c.idx < len(c.errs) {
		err = c.errs[c.idx]
	}
	c.idx++
	return resp, err
}

// TestEndToEndUsagePipelineSuccessAndFailureTurns drives two real
// Session.Handle turns through a real *app.Runtime backed by a sqlite
// state store, using a fake completion client. It proves that the full
// pipeline -- operatoragent.Response.Usage on success, *UsageError on
// post-ChatCompletion failure, console.persistResponseUsage, and
// Runtime.RecordUsage -- lands both billed turns in store.Usage so that
// SummarizeUsage reflects the total.
func TestEndToEndUsagePipelineSuccessAndFailureTurns(t *testing.T) {
	workDir := t.TempDir()
	runtime, err := app.OpenRuntimeWithConfigOptions(workDir, configtest.IsolatedOptions(t))
	if err != nil {
		t.Fatalf("OpenRuntime() error = %v", err)
	}
	t.Cleanup(func() {
		if err := runtime.Close(); err != nil {
			t.Fatalf("runtime.Close() error = %v", err)
		}
	})
	if _, err := runtime.Bootstrap(time.Now()); err != nil {
		t.Fatalf("Bootstrap() error = %v", err)
	}

	client := &scriptedCompletionClient{
		responses: []openai.ChatCompletionResponse{
			// Turn 1: clean final response. usage = 30/4.
			{Content: `{"type":"final","message":"hello"}`, PromptTokens: 30, CompletionTokens: 4},
			// Turn 2: malformed loop response. Tokens (50/8) are
			// already billed by the provider so they must still
			// reach the store even though Handle returns an error.
			{Content: `definitely not json`, PromptTokens: 50, CompletionTokens: 8},
		},
	}
	agent := operatoragent.NewModelAgent(client, "openai-compatible", "fake-model")
	session := NewSessionWithAgent("test", agent)
	session.DefaultAgentID = "codex"
	// Capture wall time once for the SummarizeUsage assertion below.
	// The actual RecordedAt on stored usage comes from
	// operatoragent.Respond's own time.Now().UTC() and is not under
	// our control here; session.Now would only influence agentContext
	// metadata. Both sides land in the same local-day bucket via
	// model.NormalizeUsageDay as long as the test does not cross a
	// local-midnight boundary mid-run (sub-second runtime).
	now := time.Now()

	out, err := session.Handle("first turn", runtime)
	if err != nil {
		t.Fatalf("Handle(turn 1) error = %v", err)
	}
	if !strings.Contains(out, "hello") {
		t.Fatalf("Handle(turn 1) output = %q, want hello", out)
	}

	_, err = session.Handle("second turn", runtime)
	if err == nil || !strings.Contains(err.Error(), "invalid loop response") {
		t.Fatalf("Handle(turn 2) error = %v, want parse failure surfaced", err)
	}

	summary, err := runtime.SummarizeUsage(now)
	if err != nil {
		t.Fatalf("SummarizeUsage() error = %v", err)
	}
	if summary.Calls != 2 {
		t.Fatalf("summary.Calls = %d, want 2 (success + parse-failure both billed)", summary.Calls)
	}
	if summary.PromptTokens != 80 || summary.CompletionTokens != 12 {
		t.Fatalf("summary tokens = %d/%d, want 80/12", summary.PromptTokens, summary.CompletionTokens)
	}
	if summary.TotalTokens != 92 {
		t.Fatalf("summary.TotalTokens = %d, want 92", summary.TotalTokens)
	}
}
