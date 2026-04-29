package mcp

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
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

func TestSDKFacingToolContractSnapshot(t *testing.T) {
	tools := toolDefinitionsByName(t)
	snapshot := loadSDKToolContractSnapshot(t)
	for _, want := range snapshot {
		if _, ok := tools[want.Name]; !ok {
			t.Fatalf("snapshot tool %s missing from tools/list", want.Name)
		}
		assertExactToolProperties(t, tools, want.Name, want.Properties)
		assertRequired(t, tools, want.Name, want.Required)
		for alias, target := range want.AliasFor {
			assertDeprecatedAlias(t, tools, want.Name, alias, target)
		}
		for _, forbidden := range want.Forbidden {
			if _, ok := toolProperties(t, tools, want.Name)[forbidden]; ok {
				t.Fatalf("%s unexpectedly exposes forbidden property %s", want.Name, forbidden)
			}
		}
	}
}

func TestMCPV1ExposesOnlyReadAndProposalTools(t *testing.T) {
	tools := toolDefinitionsByName(t)
	allowed := []string{
		"managed_status",
		"system_doc_get",
		"vault_read",
		"vault_list",
		"vault_search_text",
		"vault_resolve",
		"vault_backlinks",
		"doc_classify",
		"context_pack",
		"persona_update_propose",
		"markdown_note_propose",
	}
	if len(tools) != len(allowed) {
		t.Fatalf("tools/list tool count = %d, want %d allowed tools: %#v", len(tools), len(allowed), toolNames(tools))
	}
	for _, name := range allowed {
		if _, ok := tools[name]; !ok {
			t.Fatalf("tools/list missing allowed tool %s; got %#v", name, toolNames(tools))
		}
	}
}

func TestExternalMCPDoesNotExposeDirectWrites(t *testing.T) {
	tools := toolDefinitionsByName(t)
	for _, name := range []string{
		"vault_write_low",
		"workspace_write",
		"workspace_edit",
		"shell_exec",
		"shell_run",
		"draft_apply",
		"draft_approve",
		"draft_supersede",
		"draft_refine",
		"proposal_submit",
		"note_write",
		"markdown_note_write",
	} {
		if _, ok := tools[name]; ok {
			t.Fatalf("external MCP exposes forbidden write/apply tool %s", name)
		}
	}
}

func TestProposalToolContract(t *testing.T) {
	tools := toolDefinitionsByName(t)
	tool, ok := tools["persona_update_propose"]
	if !ok {
		t.Fatal("tools/list missing persona_update_propose")
	}
	if description, _ := tool["description"].(string); !strings.Contains(description, "pending draft") || !strings.Contains(description, "does not write or apply") {
		t.Fatalf("persona_update_propose description = %q, want proposal-only boundary", description)
	}
	assertExactToolProperties(t, tools, "persona_update_propose", []string{
		"field",
		"current_value",
		"proposed_value",
		"evidence",
		"confidence",
		"reason",
		"source",
		"observed_at",
	})
	assertRequired(t, tools, "persona_update_propose", []string{
		"field",
		"proposed_value",
		"evidence",
		"confidence",
		"reason",
		"source",
		"observed_at",
	})
	for _, forbidden := range []string{"apply", "approve", "content", "overwrite", "path"} {
		if _, ok := toolProperties(t, tools, "persona_update_propose")[forbidden]; ok {
			t.Fatalf("persona_update_propose unexpectedly exposes %s", forbidden)
		}
	}
	if _, ok := tools["vault_write_low"]; ok {
		t.Fatal("tools/list exposes vault_write_low through external MCP")
	}
	if _, ok := tools["draft_apply"]; ok {
		t.Fatal("tools/list exposes draft_apply")
	}
}

func TestMarkdownNoteProposalToolContract(t *testing.T) {
	tools := toolDefinitionsByName(t)
	tool, ok := tools["markdown_note_propose"]
	if !ok {
		t.Fatal("tools/list missing markdown_note_propose")
	}
	description, _ := tool["description"].(string)
	for _, want := range []string{"pending draft", "does not write", "does not apply"} {
		if !strings.Contains(description, want) {
			t.Fatalf("markdown_note_propose description = %q, want %q", description, want)
		}
	}
	assertExactToolProperties(t, tools, "markdown_note_propose", []string{
		"target_path",
		"title",
		"content",
		"source_kind",
		"evidence",
		"reason",
		"source",
		"observed_at",
		"task_context",
		"course",
		"topic",
		"dedupe_key",
	})
	assertRequired(t, tools, "markdown_note_propose", []string{
		"target_path",
		"title",
		"content",
		"source_kind",
		"evidence",
		"reason",
		"source",
		"observed_at",
	})
	assertEnum(t, tools, "markdown_note_propose", "source_kind", []string{"class", "meeting", "development", "conversation", "research", "other"})
	for _, forbidden := range []string{"overwrite", "apply", "approve", "shell", "tags", "related_paths"} {
		if _, ok := toolProperties(t, tools, "markdown_note_propose")[forbidden]; ok {
			t.Fatalf("markdown_note_propose unexpectedly exposes %s", forbidden)
		}
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

func TestPersonaUpdateProposeCreatesDraftOnly(t *testing.T) {
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
	personaAbs := filepath.Join(cfg.Paths.VaultRoot, cfg.Vault.ManagedCore.Persona)
	before, err := os.ReadFile(personaAbs)
	if err != nil {
		t.Fatalf("ReadFile(persona before) error = %v", err)
	}

	server := NewServer(h, "test")
	result, err := server.callTool("persona_update_propose", map[string]any{
		"field":          "education.major",
		"current_value":  "",
		"proposed_value": "电子商务",
		"evidence":       "用户说：我是大连理工大学学生，专业电子商务",
		"confidence":     "high",
		"reason":         "这是用户长期教育背景事实",
		"source":         "external_agent",
		"observed_at":    "2026-04-28T10:30:00+08:00",
	})
	if err != nil {
		t.Fatalf("persona_update_propose callTool() error = %v", err)
	}
	proposalResult, ok := result.(model.PersonaUpdateProposalResult)
	if !ok {
		t.Fatalf("result type = %T, want PersonaUpdateProposalResult", result)
	}
	if proposalResult.Status != "draft_created" || proposalResult.DraftID == "" || proposalResult.Target != cfg.Vault.ManagedCore.Persona || !proposalResult.ReviewRequired {
		t.Fatalf("result = %+v, want draft_created review-required persona target", proposalResult)
	}
	after, err := os.ReadFile(personaAbs)
	if err != nil {
		t.Fatalf("ReadFile(persona after) error = %v", err)
	}
	if string(after) != string(before) {
		t.Fatalf("persona document changed on proposal creation")
	}
	draft, err := st.Drafts().GetDraft(proposalResult.DraftID)
	if err != nil {
		t.Fatalf("GetDraft(%s) error = %v", proposalResult.DraftID, err)
	}
	if draft.Kind != model.DraftKindPersonaUpdate || draft.State != model.DraftPendingReview {
		t.Fatalf("draft kind/state = %s/%s, want persona_update/pending_review", draft.Kind, draft.State)
	}
	if !strings.Contains(draft.Summary, "Proposal creation is not apply") || !strings.Contains(draft.Summary, "persona document is unchanged") {
		t.Fatalf("draft summary missing no-apply warning: %q", draft.Summary)
	}
}

func TestMarkdownNoteProposeCreatesDraftOnly(t *testing.T) {
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

	target := "03-notes/inbox/ecommerce-platforms.md"
	server := NewServer(h, "test")
	result, err := server.callTool("markdown_note_propose", map[string]any{
		"target_path":  target,
		"title":        "E-commerce Platforms",
		"content":      "# E-commerce Platforms\n\n- Marketplaces coordinate buyers and sellers.",
		"source_kind":  "class",
		"evidence":     "class transcript discussed marketplace coordination",
		"reason":       "durable class note for later review",
		"source":       "external_agent",
		"observed_at":  "2026-04-28T10:30:00+08:00",
		"task_context": "class note extraction",
		"course":       "E-commerce",
		"topic":        "platforms",
		"dedupe_key":   "ecommerce-platforms-2026-04-28",
	})
	if err != nil {
		t.Fatalf("markdown_note_propose callTool() error = %v", err)
	}
	proposalResult, ok := result.(model.MarkdownNoteProposalResult)
	if !ok {
		t.Fatalf("result type = %T, want MarkdownNoteProposalResult", result)
	}
	if proposalResult.Status != "draft_created" || proposalResult.DraftID == "" || proposalResult.Target != target || !proposalResult.ReviewRequired {
		t.Fatalf("result = %+v, want draft_created review-required note target", proposalResult)
	}
	if _, err := os.Stat(filepath.Join(cfg.Paths.VaultRoot, filepath.FromSlash(target))); !os.IsNotExist(err) {
		t.Fatalf("target note exists after proposal creation: %v", err)
	}
	draft, err := st.Drafts().GetDraft(proposalResult.DraftID)
	if err != nil {
		t.Fatalf("GetDraft(%s) error = %v", proposalResult.DraftID, err)
	}
	if draft.Kind != model.DraftKindMarkdownNoteWrite || draft.State != model.DraftPendingReview {
		t.Fatalf("draft kind/state = %s/%s, want markdown_note_write/pending_review", draft.Kind, draft.State)
	}
	if draft.Target.BaseVersion != model.DraftBaseVersionNewFile {
		t.Fatalf("BaseVersion = %q, want new", draft.Target.BaseVersion)
	}
}

func TestMarkdownNoteProposeRejectsInvalidProposal(t *testing.T) {
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

	server := NewServer(h, "test")
	if _, err := server.callTool("markdown_note_propose", map[string]any{
		"target_path": "../escape.md",
		"title":       "E-commerce Platforms",
		"content":     "# E-commerce Platforms",
		"source_kind": "class",
		"evidence":    "class transcript",
		"reason":      "durable note",
		"source":      "external_agent",
		"observed_at": "2026-04-28T10:30:00+08:00",
	}); err == nil {
		t.Fatal("markdown_note_propose invalid target error = nil, want error")
	}
	drafts, err := st.Drafts().ListDrafts()
	if err != nil {
		t.Fatalf("ListDrafts() error = %v", err)
	}
	if len(drafts) != 0 {
		t.Fatalf("drafts = %+v, want no draft for invalid proposal", drafts)
	}
}

func TestPersonaUpdateProposeRejectsInvalidProposal(t *testing.T) {
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

	server := NewServer(h, "test")
	if _, err := server.callTool("persona_update_propose", map[string]any{
		"field":          "education.major",
		"proposed_value": "电子商务",
		"evidence":       "用户说：我是大连理工大学学生，专业电子商务",
		"confidence":     "high",
		"reason":         "这是用户长期教育背景事实",
		"source":         "external_agent",
	}); err == nil || !strings.Contains(err.Error(), "observed_at") {
		t.Fatalf("persona_update_propose missing observed_at error = %v, want observed_at error", err)
	}
	drafts, err := st.Drafts().ListDrafts()
	if err != nil {
		t.Fatalf("ListDrafts() error = %v", err)
	}
	if len(drafts) != 0 {
		t.Fatalf("drafts = %+v, want no draft for invalid proposal", drafts)
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

func TestReadMessageRejectsOversizedFrameBeforeBodyRead(t *testing.T) {
	payload := fmt.Sprintf("Content-Length: %d\r\n\r\n", maxFrameContentLength+1)
	if _, err := readMessage(bufio.NewReader(strings.NewReader(payload))); err == nil || !strings.Contains(err.Error(), "content length exceeds") {
		t.Fatalf("readMessage(oversized) error = %v, want content length exceeds", err)
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

func toolNames(tools map[string]map[string]any) []string {
	names := make([]string, 0, len(tools))
	for name := range tools {
		names = append(names, name)
	}
	return names
}

type sdkToolContractSnapshot struct {
	Name       string            `json:"name"`
	Properties []string          `json:"properties"`
	Required   []string          `json:"required,omitempty"`
	AliasFor   map[string]string `json:"alias_for,omitempty"`
	Forbidden  []string          `json:"forbidden,omitempty"`
}

func loadSDKToolContractSnapshot(t *testing.T) []sdkToolContractSnapshot {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("..", "..", "docs", "contracts", "mcp-sdk-tools-v0.json"))
	if err != nil {
		t.Fatalf("ReadFile(mcp-sdk-tools-v0.json) error = %v", err)
	}
	var snapshot []sdkToolContractSnapshot
	if err := json.Unmarshal(data, &snapshot); err != nil {
		t.Fatalf("json.Unmarshal(mcp-sdk-tools-v0.json) error = %v", err)
	}
	return snapshot
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
	if len(required) != len(want) {
		t.Fatalf("%s required = %#v, want exactly %#v", name, required, want)
	}
	got := make(map[string]bool, len(required))
	for _, value := range required {
		got[value] = true
	}
	for _, value := range want {
		if !got[value] {
			t.Fatalf("%s required = %#v, want exactly %#v", name, required, want)
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

func assertEnum(t *testing.T, tools map[string]map[string]any, name string, propertyName string, want []string) {
	t.Helper()
	property, ok := toolProperties(t, tools, name)[propertyName].(map[string]any)
	if !ok {
		t.Fatalf("%s property %s schema = %T", name, propertyName, toolProperties(t, tools, name)[propertyName])
	}
	raw, ok := property["enum"].([]string)
	if !ok {
		t.Fatalf("%s property %s enum = %T", name, propertyName, property["enum"])
	}
	if len(raw) != len(want) {
		t.Fatalf("%s property %s enum = %#v, want %#v", name, propertyName, raw, want)
	}
	for i := range want {
		if raw[i] != want[i] {
			t.Fatalf("%s property %s enum = %#v, want %#v", name, propertyName, raw, want)
		}
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

func assertExactToolProperties(t *testing.T, tools map[string]map[string]any, name string, want []string) {
	t.Helper()
	properties := toolProperties(t, tools, name)
	if len(properties) != len(want) {
		t.Fatalf("%s property count = %d, want %d: %#v", name, len(properties), len(want), properties)
	}
	assertToolProperties(t, tools, name, want)
}
