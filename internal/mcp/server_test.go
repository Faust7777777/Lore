package mcp

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"obsidian-harness/internal/config"
	"obsidian-harness/internal/orchestrator"
	"obsidian-harness/internal/store/memory"
	"obsidian-harness/internal/vault"
)

func TestServerToolsListAndCall(t *testing.T) {
	workDir := t.TempDir()
	cfg := config.Default(workDir)
	st := memory.New()
	h, err := orchestrator.New(cfg, st)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	if _, err := h.BootstrapManagedVault(time.Date(2026, 4, 22, 9, 0, 0, 0, time.UTC)); err != nil {
		t.Fatalf("BootstrapManagedVault() error = %v", err)
	}
	if _, err := vault.WriteFileAtomic(filepath.Join(cfg.Paths.VaultRoot, "03-notes", "note.md"), []byte("SQL practice"), cfg.Vault.TempSuffix); err != nil {
		t.Fatalf("WriteFileAtomic() error = %v", err)
	}

	server := NewServer(h, "test")
	input := buildFrame(`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{}}`) +
		buildFrame(`{"jsonrpc":"2.0","id":2,"method":"tools/list","params":{}}`) +
		buildFrame(`{"jsonrpc":"2.0","id":3,"method":"tools/call","params":{"name":"managed_status","arguments":{}}}`) +
		buildFrame(`{"jsonrpc":"2.0","id":4,"method":"tools/call","params":{"name":"context_pack","arguments":{"task":"SQL","limit":3}}}`)

	var output bytes.Buffer
	if err := server.Serve(context.Background(), strings.NewReader(input), &output); err != nil {
		t.Fatalf("Serve() error = %v", err)
	}

	responses := decodeFrames(t, output.Bytes())
	if len(responses) != 4 {
		t.Fatalf("response count = %d, want 4", len(responses))
	}

	if responses[1]["result"] == nil {
		t.Fatalf("tools/list result is nil: %#v", responses[1])
	}
	toolsList := responses[1]["result"].(map[string]any)["tools"].([]any)
	foundSystemDocEnum := false
	for _, item := range toolsList {
		tool := item.(map[string]any)
		if tool["name"] != "system_doc_get" {
			continue
		}
		properties := tool["inputSchema"].(map[string]any)["properties"].(map[string]any)
		enumValues := properties["name"].(map[string]any)["enum"].([]any)
		joined := make([]string, 0, len(enumValues))
		for _, value := range enumValues {
			joined = append(joined, value.(string))
		}
		if strings.Contains(strings.Join(joined, ","), "agent") && strings.Contains(strings.Join(joined, ","), "identity") {
			foundSystemDocEnum = true
		}
	}
	if !foundSystemDocEnum {
		t.Fatal("tools/list missing agent/identity system_doc_get enum values")
	}

	result := responses[2]["result"].(map[string]any)
	structured := result["structuredContent"].(map[string]any)
	if structured["ready"] != true {
		t.Fatalf("managed_status ready = %#v, want true", structured["ready"])
	}

	contextResult := responses[3]["result"].(map[string]any)
	contextStructured := contextResult["structuredContent"].(map[string]any)
	if contextStructured["progress_doc"] == nil {
		t.Fatal("context_pack missing progress_doc")
	}
	if _, err := st.Audit().ListAudit(10); err != nil {
		t.Fatalf("ListAudit() error = %v", err)
	}
}

func TestServerAuthFailure(t *testing.T) {
	t.Setenv("LORE_MCP_API_KEY", "expected")
	t.Setenv("LORE_CLIENT_KEY", "wrong")

	server := NewServer(nil, "test")
	err := server.Serve(context.Background(), strings.NewReader(""), &bytes.Buffer{})
	if err == nil || !strings.Contains(err.Error(), "authentication failed") {
		t.Fatalf("Serve() error = %v, want auth failure", err)
	}
}

func buildFrame(jsonPayload string) string {
	return fmt.Sprintf("Content-Length: %d\r\n\r\n%s", len(jsonPayload), jsonPayload)
}

func decodeFrames(t *testing.T, raw []byte) []map[string]any {
	t.Helper()

	parts := bytes.Split(raw, []byte("Content-Length: "))
	out := make([]map[string]any, 0)
	for _, part := range parts {
		if len(part) == 0 {
			continue
		}
		headerAndBody := bytes.SplitN(part, []byte("\r\n\r\n"), 2)
		if len(headerAndBody) != 2 {
			t.Fatalf("invalid frame: %q", string(part))
		}
		var envelope map[string]any
		if err := json.Unmarshal(headerAndBody[1], &envelope); err != nil {
			t.Fatalf("json.Unmarshal() error = %v", err)
		}
		out = append(out, envelope)
	}
	return out
}
