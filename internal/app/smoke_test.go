package app

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"obsidian-harness/internal/adapter/codexjsonl"
	"obsidian-harness/internal/mcp"
	"obsidian-harness/internal/model"
)

func TestRuntimeSmokeP0(t *testing.T) {
	runtime, err := openRuntimeWithFakeProcessSinkSummarizer(t, t.TempDir())
	if err != nil {
		t.Fatalf("OpenRuntime() error = %v", err)
	}

	result, err := runtime.SmokeP0(time.Date(2026, 4, 23, 9, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatalf("SmokeP0() error = %v", err)
	}
	if !result.OK() {
		t.Fatalf("SmokeP0() checks failed: %+v", result.Checks)
	}
	if !result.Managed.Ready {
		t.Fatal("Managed.Ready = false, want true")
	}
	if result.Draft.ID == "" || result.Checkpoint.Path == "" || result.Report.Path == "" {
		t.Fatalf("result = %+v, want populated draft/checkpoint/report", result)
	}
}

func TestRuntimeDemoP0BFallsBackWhenCheckpointSummaryIsEmpty(t *testing.T) {
	runtime, err := openRuntimeWithFakeProcessSinkSummarizer(t, t.TempDir())
	if err != nil {
		t.Fatalf("OpenRuntime() error = %v", err)
	}
	runtime.ProcessSinkSummarizer = emptyCheckpointSummarySummarizer{}

	result, err := runtime.DemoP0B(time.Date(2026, 4, 23, 9, 30, 0, 0, time.UTC))
	if err != nil {
		t.Fatalf("DemoP0B() error = %v", err)
	}
	if result.Checkpoint.State != model.CheckpointMaterialized {
		t.Fatalf("checkpoint state = %s, want %s", result.Checkpoint.State, model.CheckpointMaterialized)
	}
	if !strings.Contains(result.Checkpoint.Content, "demo-week governance draft") {
		t.Fatalf("checkpoint content missing deterministic fallback summary:\n%s", result.Checkpoint.Content)
	}
}

func TestRuntimeSmokeGovernedMarkdownNoteIntake(t *testing.T) {
	runtime, err := openRuntimeWithFakeProcessSinkSummarizer(t, t.TempDir())
	if err != nil {
		t.Fatalf("OpenRuntime() error = %v", err)
	}

	result, err := runtime.SmokeGovernedMarkdownNoteIntake(time.Date(2026, 4, 28, 10, 30, 0, 0, time.UTC))
	if err != nil {
		t.Fatalf("SmokeGovernedMarkdownNoteIntake() error = %v", err)
	}
	if !result.OK() {
		t.Fatalf("SmokeGovernedMarkdownNoteIntake() checks failed: %+v", result.Checks)
	}
	if result.Proposal.DraftID == "" || result.Applied.ID != result.Proposal.DraftID || result.Target.Path == "" {
		t.Fatalf("result = %+v, want populated proposal/applied/target", result)
	}

	assertExternalMCPNoDirectWriteTools(t, runtime)
}

func TestRuntimeSmokeExternalMCPGovernedMarkdownNoteIntake(t *testing.T) {
	runtime, err := openRuntimeWithFakeProcessSinkSummarizer(t, t.TempDir())
	if err != nil {
		t.Fatalf("OpenRuntime() error = %v", err)
	}

	now := time.Date(2026, 4, 29, 11, 0, 0, 0, time.UTC)
	if _, err := runtime.Bootstrap(now); err != nil {
		t.Fatalf("Bootstrap() error = %v", err)
	}

	targetPath := "03-notes/smoke/external-mcp-governed-note.md"
	result := callMCPToolForSmoke(t, runtime, "markdown_note_propose", map[string]any{
		"target_path":  targetPath,
		"title":        "External MCP Governed Note Smoke",
		"content":      "# External MCP Governed Note Smoke\n\n- External MCP can submit a note proposal.\n- Local Lore must review, approve, and apply before vault write.",
		"source_kind":  "development",
		"evidence":     "external MCP smoke test transcript",
		"reason":       "verify external MCP proposal intake remains governed",
		"source":       "external_mcp_smoke",
		"observed_at":  now.Format(time.RFC3339),
		"task_context": "external agent governed markdown note intake",
		"topic":        "Lore governance",
		"dedupe_key":   "external-mcp-governed-note-smoke",
	})
	if result["isError"] == true {
		t.Fatalf("MCP tool returned error: %#v", result)
	}
	structured, ok := result["structuredContent"].(map[string]any)
	if !ok {
		t.Fatalf("structuredContent = %T", result["structuredContent"])
	}
	draftID, _ := structured["draft_id"].(string)
	if structured["status"] != "draft_created" || draftID == "" || structured["target"] != targetPath || structured["review_required"] != true {
		t.Fatalf("structuredContent = %#v, want draft_created review-required target %s", structured, targetPath)
	}
	if _, err := os.Stat(filepath.Join(runtime.Config.Paths.VaultRoot, filepath.FromSlash(targetPath))); !os.IsNotExist(err) {
		t.Fatalf("target stat after MCP proposal error = %v, want not exist", err)
	}

	review, err := runtime.ReviewDraft(draftID)
	if err != nil {
		t.Fatalf("ReviewDraft(%s) error = %v", draftID, err)
	}
	if review.Draft.State != model.DraftPendingReview || review.Draft.Kind != model.DraftKindMarkdownNoteWrite {
		t.Fatalf("review draft state/kind = %s/%s, want pending markdown_note_write", review.Draft.State, review.Draft.Kind)
	}
	if _, err := runtime.Harness.ApproveDraft(draftID, now.Add(time.Minute)); err != nil {
		t.Fatalf("ApproveDraft(%s) error = %v", draftID, err)
	}
	applied, err := runtime.Harness.ApplyDraft(draftID, now.Add(2*time.Minute))
	if err != nil {
		t.Fatalf("ApplyDraft(%s) error = %v", draftID, err)
	}
	if applied.State != model.DraftApplied {
		t.Fatalf("applied state = %s, want applied", applied.State)
	}
	doc, err := runtime.Harness.VaultRead(targetPath)
	if err != nil {
		t.Fatalf("VaultRead(%s) error = %v", targetPath, err)
	}
	if !strings.Contains(doc.Content, "Local Lore must review, approve, and apply before vault write.") {
		t.Fatalf("applied note content missing governed review line:\n%s", doc.Content)
	}

	assertExternalMCPNoDirectWriteTools(t, runtime)
}

func TestAuditHasDraftChainRequiresSameDraftAndTarget(t *testing.T) {
	records := []model.AuditRecord{
		{
			Kind:          model.AuditDraftCreated,
			CorrelationID: "draft-a",
			Target:        "03-notes/a.md",
		},
		{
			Kind:          model.AuditDraftStateChange,
			CorrelationID: "draft-b",
			Target:        "03-notes/a.md",
			Metadata:      map[string]string{"state": string(model.DraftApproved)},
		},
		{
			Kind:          model.AuditDraftApplied,
			CorrelationID: "draft-a",
			Target:        "03-notes/b.md",
		},
	}

	if auditHasDraftChain(records, "draft-a", "03-notes/a.md") {
		t.Fatal("auditHasDraftChain() = true for mismatched draft/target chain, want false")
	}
}

func callMCPToolForSmoke(t *testing.T, runtime *Runtime, name string, args map[string]any) map[string]any {
	t.Helper()
	t.Setenv("LORE_MCP_API_KEY", "")
	t.Setenv("OBSIDIAN_HARNESS_MCP_API_KEY", "")
	t.Setenv("LORE_CLIENT_KEY", "")
	t.Setenv("OBSIDIAN_HARNESS_CLIENT_KEY", "")

	payload, err := json.Marshal(map[string]any{
		"jsonrpc": "2.0",
		"id":      1,
		"method":  "tools/call",
		"params": map[string]any{
			"name":      name,
			"arguments": args,
		},
	})
	if err != nil {
		t.Fatalf("json.Marshal(MCP tool call) error = %v", err)
	}
	server := mcp.NewServer(runtime.Harness, "test")
	var output bytes.Buffer
	if err := server.Serve(context.Background(), strings.NewReader(buildMCPFrame(string(payload))), &output); err != nil {
		t.Fatalf("MCP Serve(tools/call %s) error = %v", name, err)
	}
	responses := decodeMCPFrames(t, output.Bytes())
	if len(responses) != 1 {
		t.Fatalf("MCP response count = %d, want 1", len(responses))
	}
	if rawErr := responses[0]["error"]; rawErr != nil {
		t.Fatalf("MCP response error = %#v", rawErr)
	}
	result, ok := responses[0]["result"].(map[string]any)
	if !ok {
		t.Fatalf("MCP result = %T", responses[0]["result"])
	}
	return result
}

func assertExternalMCPNoDirectWriteTools(t *testing.T, runtime *Runtime) {
	t.Helper()
	t.Setenv("LORE_MCP_API_KEY", "")
	t.Setenv("OBSIDIAN_HARNESS_MCP_API_KEY", "")
	t.Setenv("LORE_CLIENT_KEY", "")
	t.Setenv("OBSIDIAN_HARNESS_CLIENT_KEY", "")

	server := mcp.NewServer(runtime.Harness, "test")
	input := buildMCPFrame(`{"jsonrpc":"2.0","id":1,"method":"tools/list","params":{}}`)
	var output bytes.Buffer
	if err := server.Serve(context.Background(), strings.NewReader(input), &output); err != nil {
		t.Fatalf("MCP Serve(tools/list) error = %v", err)
	}

	responses := decodeMCPFrames(t, output.Bytes())
	if len(responses) != 1 {
		t.Fatalf("MCP response count = %d, want 1", len(responses))
	}
	result, ok := responses[0]["result"].(map[string]any)
	if !ok {
		t.Fatalf("MCP result = %T", responses[0]["result"])
	}
	rawTools, ok := result["tools"].([]any)
	if !ok {
		t.Fatalf("MCP tools = %T", result["tools"])
	}
	toolNames := make(map[string]bool, len(rawTools))
	for _, raw := range rawTools {
		tool, ok := raw.(map[string]any)
		if !ok {
			t.Fatalf("MCP tool = %T", raw)
		}
		name, _ := tool["name"].(string)
		toolNames[name] = true
	}
	if !toolNames["markdown_note_propose"] {
		t.Fatalf("MCP tools missing markdown_note_propose: %#v", toolNames)
	}
	for _, forbidden := range []string{
		"vault_write_low",
		"workspace_write",
		"workspace_edit",
		"shell_exec",
		"draft_apply",
		"draft_approve",
		"draft_supersede",
		"proposal_submit",
	} {
		if toolNames[forbidden] {
			t.Fatalf("external MCP exposes forbidden tool %s", forbidden)
		}
	}
}

func buildMCPFrame(jsonPayload string) string {
	return fmt.Sprintf("Content-Length: %d\r\n\r\n%s", len(jsonPayload), jsonPayload)
}

func decodeMCPFrames(t *testing.T, raw []byte) []map[string]any {
	t.Helper()

	parts := bytes.Split(raw, []byte("Content-Length: "))
	out := make([]map[string]any, 0)
	for _, part := range parts {
		if len(part) == 0 {
			continue
		}
		headerAndBody := bytes.SplitN(part, []byte("\r\n\r\n"), 2)
		if len(headerAndBody) != 2 {
			t.Fatalf("invalid MCP frame: %q", string(part))
		}
		var envelope map[string]any
		if err := json.Unmarshal(headerAndBody[1], &envelope); err != nil {
			t.Fatalf("json.Unmarshal(MCP frame) error = %v", err)
		}
		out = append(out, envelope)
	}
	return out
}

type emptyCheckpointSummarySummarizer struct{}

func (emptyCheckpointSummarySummarizer) SummarizeCheckpoint(codexjsonl.WindowSummary) (string, string, error) {
	return "", "", nil
}

func (emptyCheckpointSummarySummarizer) SummarizeDaily(agentID string, _ time.Time, checkpoints []model.CheckpointDoc) (string, string, error) {
	if len(checkpoints) == 0 {
		return agentID + " daily report", "- no checkpoints recorded", nil
	}
	return agentID + " daily report", "- checkpoint fallback was materialized", nil
}

func (e emptyCheckpointSummarySummarizer) SummarizeCheckpointContext(_ context.Context, window codexjsonl.WindowSummary) (string, string, error) {
	return e.SummarizeCheckpoint(window)
}

func (e emptyCheckpointSummarySummarizer) SummarizeDailyContext(_ context.Context, agentID string, day time.Time, checkpoints []model.CheckpointDoc) (string, string, error) {
	return e.SummarizeDaily(agentID, day, checkpoints)
}
