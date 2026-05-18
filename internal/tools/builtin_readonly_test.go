package tools

import (
	"errors"
	"reflect"
	"testing"
	"time"

	"obsidian-harness/internal/model"
)

// fakeHarness implements Harness for tool unit tests. Each method records
// the arguments it received so Call dispatch can be verified end-to-end.
type fakeHarness struct {
	managedStatusView model.ManagedStatusView
	managedStatusErr  error

	systemDocGetName string
	systemDocGetView model.VaultDocument
	systemDocGetErr  error

	vaultReadPath string
	vaultReadDoc  model.VaultDocument
	vaultReadErr  error

	vaultListDir     string
	vaultListEntries []model.VaultEntry
	vaultListErr     error

	vaultSearchTextQuery, vaultSearchTextDir string
	vaultSearchTextLimit                     int
	vaultSearchTextHits                      []model.SearchHit
	vaultSearchTextErr                       error

	vaultResolveQuery, vaultResolveDir string
	vaultResolveLimit                  int
	vaultResolveResult                 model.VaultResolveResult
	vaultResolveErr                    error

	vaultBacklinksPath  string
	vaultBacklinksLimit int
	vaultBacklinksHits  []model.SearchHit
	vaultBacklinksErr   error

	docClassifyPath string
	docClassifyView model.DocClassificationView

	contextPackTarget, contextPackTask string
	contextPackLimit                   int
	contextPackResult                  model.ContextPack
	contextPackErr                     error

	proposePersonaArg model.PersonaUpdateProposal
	proposePersonaAt  time.Time
	proposePersonaRes model.PersonaUpdateProposalResult
	proposePersonaErr error

	proposeMarkdownArg model.MarkdownNoteProposal
	proposeMarkdownAt  time.Time
	proposeMarkdownRes model.MarkdownNoteProposalResult
	proposeMarkdownErr error
}

func (f *fakeHarness) ManagedStatus() (model.ManagedStatusView, error) {
	return f.managedStatusView, f.managedStatusErr
}
func (f *fakeHarness) SystemDocGet(name string) (model.VaultDocument, error) {
	f.systemDocGetName = name
	return f.systemDocGetView, f.systemDocGetErr
}
func (f *fakeHarness) VaultRead(relPath string) (model.VaultDocument, error) {
	f.vaultReadPath = relPath
	return f.vaultReadDoc, f.vaultReadErr
}
func (f *fakeHarness) VaultList(relDir string) ([]model.VaultEntry, error) {
	f.vaultListDir = relDir
	return f.vaultListEntries, f.vaultListErr
}
func (f *fakeHarness) VaultSearchText(query, relDir string, limit int) ([]model.SearchHit, error) {
	f.vaultSearchTextQuery = query
	f.vaultSearchTextDir = relDir
	f.vaultSearchTextLimit = limit
	return f.vaultSearchTextHits, f.vaultSearchTextErr
}
func (f *fakeHarness) VaultResolve(query, relDir string, limit int) (model.VaultResolveResult, error) {
	f.vaultResolveQuery = query
	f.vaultResolveDir = relDir
	f.vaultResolveLimit = limit
	return f.vaultResolveResult, f.vaultResolveErr
}
func (f *fakeHarness) VaultBacklinks(relPath string, limit int) ([]model.SearchHit, error) {
	f.vaultBacklinksPath = relPath
	f.vaultBacklinksLimit = limit
	return f.vaultBacklinksHits, f.vaultBacklinksErr
}
func (f *fakeHarness) DocClassify(relPath string) model.DocClassificationView {
	f.docClassifyPath = relPath
	return f.docClassifyView
}
func (f *fakeHarness) ContextPack(targetPath, task string, limit int) (model.ContextPack, error) {
	f.contextPackTarget = targetPath
	f.contextPackTask = task
	f.contextPackLimit = limit
	return f.contextPackResult, f.contextPackErr
}
func (f *fakeHarness) ProposePersonaUpdate(p model.PersonaUpdateProposal, at time.Time) (model.PersonaUpdateProposalResult, error) {
	f.proposePersonaArg = p
	f.proposePersonaAt = at
	return f.proposePersonaRes, f.proposePersonaErr
}
func (f *fakeHarness) ProposeMarkdownNote(p model.MarkdownNoteProposal, at time.Time) (model.MarkdownNoteProposalResult, error) {
	f.proposeMarkdownArg = p
	f.proposeMarkdownAt = at
	return f.proposeMarkdownRes, f.proposeMarkdownErr
}

func TestRegisterReadOnlyRegistersAllNineToolsInLegacyOrder(t *testing.T) {
	r := NewRegistry()
	if err := RegisterReadOnly(r, &fakeHarness{}); err != nil {
		t.Fatalf("RegisterReadOnly() error = %v", err)
	}

	wantOrder := []string{
		"managed_status",
		"system_doc_get",
		"vault_read",
		"vault_list",
		"vault_search_text",
		"vault_resolve",
		"vault_backlinks",
		"doc_classify",
		"context_pack",
	}
	got := names(r.ListBySurface(SurfaceMCP))
	if !reflect.DeepEqual(got, wantOrder) {
		t.Errorf("ListBySurface(MCP) order = %v, want %v", got, wantOrder)
	}

	gotConsole := names(r.ListBySurface(SurfaceConsole))
	if !reflect.DeepEqual(gotConsole, wantOrder) {
		t.Errorf("ListBySurface(Console) order = %v, want %v (read-only tools share both surfaces)", gotConsole, wantOrder)
	}
}

func TestRegisterReadOnlyMCPDefinitionsMatchLegacyShape(t *testing.T) {
	r := NewRegistry()
	if err := RegisterReadOnly(r, &fakeHarness{}); err != nil {
		t.Fatalf("RegisterReadOnly() error = %v", err)
	}

	defs := MCPDefinitions(r.ListBySurface(SurfaceMCP))
	want := readOnlyLegacyDefinitions()
	if len(defs) != len(want) {
		t.Fatalf("definitions count = %d, want %d", len(defs), len(want))
	}
	for i := range defs {
		if !reflect.DeepEqual(defs[i], want[i]) {
			t.Errorf("definitions[%d] mismatch\n got = %#v\nwant = %#v", i, defs[i], want[i])
		}
	}
}

func TestManagedStatusToolCallDelegates(t *testing.T) {
	h := &fakeHarness{managedStatusView: model.ManagedStatusView{Ready: true}}
	tool := managedStatusTool{h: h}

	got, err := tool.Call(nil)
	if err != nil {
		t.Fatalf("Call() error = %v", err)
	}
	view, ok := got.(model.ManagedStatusView)
	if !ok {
		t.Fatalf("Call() returned %T, want model.ManagedStatusView", got)
	}
	if !view.Ready {
		t.Error("returned view.Ready = false, want true")
	}
}

func TestSystemDocGetToolCallExtractsName(t *testing.T) {
	h := &fakeHarness{}
	tool := systemDocGetTool{h: h}

	if _, err := tool.Call(map[string]any{"name": "  agent  "}); err != nil {
		t.Fatalf("Call() error = %v", err)
	}
	if h.systemDocGetName != "agent" {
		t.Errorf("h.systemDocGetName = %q, want trimmed 'agent'", h.systemDocGetName)
	}
}

func TestVaultListToolDirAlias(t *testing.T) {
	h := &fakeHarness{}
	tool := vaultListTool{h: h}

	// "dir" present: deprecated "path" alias is ignored.
	if _, err := tool.Call(map[string]any{"dir": "03-notes", "path": "04-other"}); err != nil {
		t.Fatalf("Call() error = %v", err)
	}
	if h.vaultListDir != "03-notes" {
		t.Errorf("dir = %q, want 03-notes (dir takes precedence over deprecated path alias)", h.vaultListDir)
	}

	// Only "path" alias provided: falls back.
	h.vaultListDir = ""
	if _, err := tool.Call(map[string]any{"path": "04-other"}); err != nil {
		t.Fatalf("Call() error = %v", err)
	}
	if h.vaultListDir != "04-other" {
		t.Errorf("dir = %q, want 04-other (fallback to deprecated path alias)", h.vaultListDir)
	}
}

func TestVaultSearchTextToolLimitDefaultsTo10(t *testing.T) {
	h := &fakeHarness{}
	tool := vaultSearchTextTool{h: h}

	if _, err := tool.Call(map[string]any{"query": "sql"}); err != nil {
		t.Fatalf("Call() error = %v", err)
	}
	if h.vaultSearchTextLimit != 10 {
		t.Errorf("limit = %d, want default 10", h.vaultSearchTextLimit)
	}
	if h.vaultSearchTextQuery != "sql" {
		t.Errorf("query = %q, want sql", h.vaultSearchTextQuery)
	}
}

func TestVaultResolveToolLimitDefaultsTo5(t *testing.T) {
	h := &fakeHarness{}
	tool := vaultResolveTool{h: h}

	if _, err := tool.Call(map[string]any{"query": "note"}); err != nil {
		t.Fatalf("Call() error = %v", err)
	}
	if h.vaultResolveLimit != 5 {
		t.Errorf("limit = %d, want default 5", h.vaultResolveLimit)
	}
}

func TestContextPackToolLimitDefaultsTo6AndTargetPathAlias(t *testing.T) {
	h := &fakeHarness{}
	tool := contextPackTool{h: h}

	if _, err := tool.Call(map[string]any{"path": "03-notes/x.md", "task": "study"}); err != nil {
		t.Fatalf("Call() error = %v", err)
	}
	if h.contextPackTarget != "03-notes/x.md" {
		t.Errorf("target = %q, want 03-notes/x.md (deprecated path alias fallback)", h.contextPackTarget)
	}
	if h.contextPackLimit != 6 {
		t.Errorf("limit = %d, want default 6", h.contextPackLimit)
	}
}

func TestDocClassifyToolWrapsValueIntoCallSignature(t *testing.T) {
	h := &fakeHarness{docClassifyView: model.DocClassificationView{Path: "x.md"}}
	tool := docClassifyTool{h: h}

	got, err := tool.Call(map[string]any{"path": "x.md"})
	if err != nil {
		t.Fatalf("Call() error = %v", err)
	}
	view, ok := got.(model.DocClassificationView)
	if !ok {
		t.Fatalf("Call() returned %T, want model.DocClassificationView", got)
	}
	if view.Path != "x.md" {
		t.Errorf("view.Path = %q, want x.md", view.Path)
	}
}

func TestReadOnlyToolPropagatesError(t *testing.T) {
	wantErr := errors.New("vault read failed")
	h := &fakeHarness{vaultReadErr: wantErr}
	tool := vaultReadTool{h: h}

	_, err := tool.Call(map[string]any{"path": "x.md"})
	if !errors.Is(err, wantErr) {
		t.Errorf("Call() error = %v, want %v", err, wantErr)
	}
}

// readOnlyLegacyDefinitions returns the expected historical live MCP output
// for the 9 read-only tools. Tests in this file compare the registry's
// MCPDefinitions output against this fixture so schema drift is explicit.
func readOnlyLegacyDefinitions() []map[string]any {
	return []map[string]any{
		{
			"name":        "managed_status",
			"description": "Return managed mode and core document status.",
			"inputSchema": map[string]any{
				"type":       "object",
				"properties": map[string]any{},
			},
		},
		{
			"name":        "system_doc_get",
			"description": "Read one of the managed core documents.",
			"inputSchema": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"name": map[string]any{
						"type": "string",
						"enum": []string{"system", "progress", "persona", "agent", "identity"},
					},
				},
				"required": []string{"name"},
			},
		},
		{
			"name":        "vault_read",
			"description": "Read a markdown document from the vault.",
			"inputSchema": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"path": map[string]any{"type": "string"},
				},
				"required": []string{"path"},
			},
		},
		{
			"name":        "vault_list",
			"description": "List files under a vault directory.",
			"inputSchema": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"dir":  map[string]any{"type": "string"},
					"path": map[string]any{"type": "string", "description": "deprecated alias for dir"},
				},
			},
		},
		{
			"name":        "vault_search_text",
			"description": "Search vault text content.",
			"inputSchema": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"query": map[string]any{"type": "string"},
					"dir":   map[string]any{"type": "string"},
					"path":  map[string]any{"type": "string", "description": "deprecated alias for dir"},
					"limit": map[string]any{"type": "integer"},
				},
				"required": []string{"query"},
			},
		},
		{
			"name":        "vault_resolve",
			"description": "Resolve a natural-language note reference to vault markdown paths. Returns unique, ambiguous, or not_found.",
			"inputSchema": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"query": map[string]any{"type": "string"},
					"dir":   map[string]any{"type": "string"},
					"path":  map[string]any{"type": "string", "description": "deprecated alias for dir"},
					"limit": map[string]any{"type": "integer"},
				},
				"required": []string{"query"},
			},
		},
		{
			"name":        "vault_backlinks",
			"description": "Find backlinks to a vault document.",
			"inputSchema": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"path":  map[string]any{"type": "string"},
					"limit": map[string]any{"type": "integer"},
				},
				"required": []string{"path"},
			},
		},
		{
			"name":        "doc_classify",
			"description": "Classify a vault document by the current rules.",
			"inputSchema": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"path": map[string]any{"type": "string"},
				},
				"required": []string{"path"},
			},
		},
		{
			"name":        "context_pack",
			"description": "Assemble a read-only context pack for a task or document.",
			"inputSchema": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"target_path": map[string]any{"type": "string"},
					"path":        map[string]any{"type": "string", "description": "deprecated alias for target_path"},
					"task":        map[string]any{"type": "string"},
					"limit":       map[string]any{"type": "integer"},
				},
			},
		},
	}
}
