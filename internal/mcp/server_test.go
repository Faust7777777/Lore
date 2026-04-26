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
	"obsidian-harness/internal/model"
	"obsidian-harness/internal/orchestrator"
	"obsidian-harness/internal/store/memory"
	"obsidian-harness/internal/vault"
)

func TestToolDefinitionsExposeSDKContract(t *testing.T) {
	tools := toolDefinitionsByName(t)
	contracts := toolContracts()
	if len(tools) != len(contracts) {
		t.Fatalf("tool count = %d, want %d", len(tools), len(contracts))
	}
	for _, contract := range contracts {
		tool, ok := tools[contract.Name]
		if !ok {
			t.Fatalf("tools/list missing %s", contract.Name)
		}
		if tool["description"] != contract.Description {
			t.Fatalf("%s description = %q, want %q", contract.Name, tool["description"], contract.Description)
		}
		wantProperties := make([]string, 0, len(contract.Arguments))
		for _, argument := range contract.Arguments {
			wantProperties = append(wantProperties, argument.Name)
			if argument.DeprecatedAlias != "" {
				assertDeprecatedAlias(t, tools, contract.Name, argument.Name, argument.DeprecatedAlias)
			}
		}
		assertToolProperties(t, tools, contract.Name, wantProperties)
		assertRequired(t, tools, contract.Name, contract.Required)
	}
}

func TestMCPDeprecatedAliasesRemainSupported(t *testing.T) {
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
		t.Fatalf("WriteFileAtomic(note) error = %v", err)
	}

	server := NewServer(h, "test")
	listResult, err := server.callTool("vault_list", map[string]any{"path": "03-notes"})
	if err != nil {
		t.Fatalf("vault_list alias callTool() error = %v", err)
	}
	entries, ok := listResult.([]model.VaultEntry)
	if !ok {
		t.Fatalf("vault_list result type = %T", listResult)
	}
	if len(entries) != 1 || entries[0].Path != "03-notes/note.md" {
		t.Fatalf("vault_list alias entries = %+v, want note", entries)
	}

	packResult, err := server.callTool("context_pack", map[string]any{"path": "03-notes/note.md", "task": "SQL", "limit": 3})
	if err != nil {
		t.Fatalf("context_pack alias callTool() error = %v", err)
	}
	pack, ok := packResult.(model.ContextPack)
	if !ok {
		t.Fatalf("context_pack result type = %T", packResult)
	}
	if pack.TargetPath != "03-notes/note.md" || pack.TargetDoc == nil || pack.TargetDoc.Path != "03-notes/note.md" {
		t.Fatalf("context_pack alias = %+v, want target note", pack)
	}
}

func TestMCPStandardArgsOverrideDeprecatedAliases(t *testing.T) {
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
		t.Fatalf("WriteFileAtomic(note) error = %v", err)
	}
	if _, err := vault.WriteFileAtomic(filepath.Join(cfg.Paths.VaultRoot, "04-other", "other.md"), []byte("Other"), cfg.Vault.TempSuffix); err != nil {
		t.Fatalf("WriteFileAtomic(other) error = %v", err)
	}

	server := NewServer(h, "test")
	listResult, err := server.callTool("vault_list", map[string]any{"dir": "03-notes", "path": "04-other"})
	if err != nil {
		t.Fatalf("vault_list callTool() error = %v", err)
	}
	entries, ok := listResult.([]model.VaultEntry)
	if !ok {
		t.Fatalf("vault_list result type = %T", listResult)
	}
	if len(entries) != 1 || entries[0].Path != "03-notes/note.md" {
		t.Fatalf("vault_list entries = %+v, want dir to override path alias", entries)
	}

	packResult, err := server.callTool("context_pack", map[string]any{"target_path": "03-notes/note.md", "path": cfg.Vault.ManagedCore.ProgressIndex, "task": "SQL", "limit": 3})
	if err != nil {
		t.Fatalf("context_pack callTool() error = %v", err)
	}
	pack, ok := packResult.(model.ContextPack)
	if !ok {
		t.Fatalf("context_pack result type = %T", packResult)
	}
	if pack.TargetPath != "03-notes/note.md" || pack.TargetDoc == nil || pack.TargetDoc.Path != "03-notes/note.md" {
		t.Fatalf("context_pack = %+v, want target_path to override path alias", pack)
	}
}

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
		buildFrame(`{"jsonrpc":"2.0","id":4,"method":"tools/call","params":{"name":"context_pack","arguments":{"task":"SQL","limit":3}}}`) +
		buildFrame(`{"jsonrpc":"2.0","id":5,"method":"tools/call","params":{"name":"vault_resolve","arguments":{"query":"note","dir":"03-notes","limit":3}}}`)

	var output bytes.Buffer
	if err := server.Serve(context.Background(), strings.NewReader(input), &output); err != nil {
		t.Fatalf("Serve() error = %v", err)
	}

	responses := decodeFrames(t, output.Bytes())
	if len(responses) != 5 {
		t.Fatalf("response count = %d, want 5", len(responses))
	}

	if responses[1]["result"] == nil {
		t.Fatalf("tools/list result is nil: %#v", responses[1])
	}
	toolsList := responses[1]["result"].(map[string]any)["tools"].([]any)
	foundSystemDocEnum := false
	foundVaultResolve := false
	for _, item := range toolsList {
		tool := item.(map[string]any)
		if tool["name"] == "vault_resolve" {
			foundVaultResolve = true
			continue
		}
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
	if !foundVaultResolve {
		t.Fatal("tools/list missing vault_resolve")
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

	resolveResult := responses[4]["result"].(map[string]any)
	resolveStructured := resolveResult["structuredContent"].(map[string]any)
	if resolveStructured["status"] != "unique" {
		t.Fatalf("vault_resolve status = %#v, want unique", resolveStructured["status"])
	}
	if resolveStructured["selected_path"] != "03-notes/note.md" {
		t.Fatalf("vault_resolve selected_path = %#v", resolveStructured["selected_path"])
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

func toolDefinitionsByName(t *testing.T) map[string]map[string]any {
	t.Helper()
	out := make(map[string]map[string]any)
	for _, tool := range toolDefinitions() {
		name, ok := tool["name"].(string)
		if !ok || name == "" {
			t.Fatalf("tool missing string name: %#v", tool)
		}
		out[name] = tool
	}
	return out
}

func assertToolProperties(t *testing.T, tools map[string]map[string]any, name string, want []string) {
	t.Helper()
	properties := toolProperties(t, tools, name)
	for _, property := range want {
		if _, ok := properties[property]; !ok {
			t.Fatalf("%s missing property %s in %#v", name, property, properties)
		}
	}
}

func assertRequired(t *testing.T, tools map[string]map[string]any, name string, want []string) {
	t.Helper()
	tool, ok := tools[name]
	if !ok {
		t.Fatalf("missing tool %s", name)
	}
	schema, ok := tool["inputSchema"].(map[string]any)
	if !ok {
		t.Fatalf("%s inputSchema = %T", name, tool["inputSchema"])
	}
	requiredRaw, ok := schema["required"]
	if !ok {
		if len(want) == 0 {
			return
		}
		t.Fatalf("%s missing required list", name)
	}
	required, ok := requiredRaw.([]string)
	if !ok {
		t.Fatalf("%s required = %T", name, requiredRaw)
	}
	got := make(map[string]bool, len(required))
	for _, value := range required {
		got[value] = true
	}
	for _, value := range want {
		if !got[value] {
			t.Fatalf("%s required = %#v, want %s", name, required, value)
		}
	}
}

func assertDeprecatedAlias(t *testing.T, tools map[string]map[string]any, name string, alias string, target string) {
	t.Helper()
	properties := toolProperties(t, tools, name)
	property, ok := properties[alias].(map[string]any)
	if !ok {
		t.Fatalf("%s alias %s schema = %T", name, alias, properties[alias])
	}
	description, _ := property["description"].(string)
	if !strings.Contains(description, "deprecated") || !strings.Contains(description, target) {
		t.Fatalf("%s alias %s description = %q, want deprecated alias for %s", name, alias, description, target)
	}
}

func toolProperties(t *testing.T, tools map[string]map[string]any, name string) map[string]any {
	t.Helper()
	tool, ok := tools[name]
	if !ok {
		t.Fatalf("missing tool %s", name)
	}
	schema, ok := tool["inputSchema"].(map[string]any)
	if !ok {
		t.Fatalf("%s inputSchema = %T", name, tool["inputSchema"])
	}
	properties, ok := schema["properties"].(map[string]any)
	if !ok {
		t.Fatalf("%s properties = %T", name, schema["properties"])
	}
	return properties
}
