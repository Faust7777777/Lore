package app

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"

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
