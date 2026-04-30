package tools

import (
	"errors"
	"reflect"
	"testing"
)

// fakeTool is a minimal Tool implementation used by unit tests in this
// package. It is local to the test file so the production package keeps
// no exported test-only types.
type fakeTool struct {
	name        string
	description string
	surfaces    Surface
	arguments   []Argument
	required    []string
	callResult  any
	callErr     error
	callCount   int
	lastArgs    map[string]any
}

func (f *fakeTool) Name() string             { return f.name }
func (f *fakeTool) Description() string      { return f.description }
func (f *fakeTool) Surfaces() Surface        { return f.surfaces }
func (f *fakeTool) Arguments() []Argument    { return f.arguments }
func (f *fakeTool) Required() []string       { return f.required }
func (f *fakeTool) Call(args map[string]any) (any, error) {
	f.callCount++
	f.lastArgs = args
	return f.callResult, f.callErr
}

func TestSurfaceHas(t *testing.T) {
	s := SurfaceMCP | SurfaceConsole | SurfaceProposal

	if !s.Has(SurfaceMCP) {
		t.Error("Has(SurfaceMCP) = false, want true")
	}
	if !s.Has(SurfaceConsole | SurfaceProposal) {
		t.Error("Has(SurfaceConsole|SurfaceProposal) = false, want true (combined bits)")
	}
	if s.Has(SurfaceLocalExec) {
		t.Error("Has(SurfaceLocalExec) = true, want false (bit not set)")
	}
	if !s.Has(0) {
		t.Error("Has(0) = false, want true (zero treated as no requirement)")
	}
}

func TestRegistryRegisterAndGet(t *testing.T) {
	r := NewRegistry()
	tool := &fakeTool{name: "managed_status", description: "Status", surfaces: SurfaceMCP | SurfaceConsole}

	if err := r.Register(tool); err != nil {
		t.Fatalf("Register() error = %v", err)
	}

	got, ok := r.Get("managed_status")
	if !ok {
		t.Fatal("Get(managed_status) ok = false, want true")
	}
	if got.Name() != "managed_status" {
		t.Errorf("Get().Name() = %q, want managed_status", got.Name())
	}
}

func TestRegistryRegisterRejectsDuplicates(t *testing.T) {
	r := NewRegistry()
	if err := r.Register(&fakeTool{name: "vault_read"}); err != nil {
		t.Fatalf("first Register() error = %v", err)
	}
	err := r.Register(&fakeTool{name: "vault_read"})
	if err == nil {
		t.Fatal("second Register() error = nil, want duplicate error")
	}
}

func TestRegistryRegisterRejectsEmptyName(t *testing.T) {
	r := NewRegistry()
	err := r.Register(&fakeTool{name: "   "})
	if err == nil {
		t.Fatal("Register(empty name) error = nil, want non-nil")
	}
}

func TestRegistryRegisterRejectsNil(t *testing.T) {
	r := NewRegistry()
	err := r.Register(nil)
	if err == nil {
		t.Fatal("Register(nil) error = nil, want non-nil")
	}
}

func TestRegistryMustRegisterPanicsOnDuplicate(t *testing.T) {
	r := NewRegistry()
	r.MustRegister(&fakeTool{name: "vault_read"})

	defer func() {
		if recover() == nil {
			t.Fatal("MustRegister(duplicate) did not panic")
		}
	}()
	r.MustRegister(&fakeTool{name: "vault_read"})
}

func TestRegistryListBySurfacePreservesRegistrationOrder(t *testing.T) {
	r := NewRegistry()
	r.MustRegister(&fakeTool{name: "managed_status", surfaces: SurfaceMCP | SurfaceConsole})
	r.MustRegister(&fakeTool{name: "draft_review", surfaces: SurfaceConsole})
	r.MustRegister(&fakeTool{name: "vault_read", surfaces: SurfaceMCP | SurfaceConsole})
	r.MustRegister(&fakeTool{name: "shell_exec", surfaces: SurfaceConsole | SurfaceLocalExec})

	mcp := r.ListBySurface(SurfaceMCP)
	mcpNames := names(mcp)
	wantMCP := []string{"managed_status", "vault_read"}
	if !reflect.DeepEqual(mcpNames, wantMCP) {
		t.Errorf("ListBySurface(MCP) = %v, want %v", mcpNames, wantMCP)
	}

	console := r.ListBySurface(SurfaceConsole)
	consoleNames := names(console)
	wantConsole := []string{"managed_status", "draft_review", "vault_read", "shell_exec"}
	if !reflect.DeepEqual(consoleNames, wantConsole) {
		t.Errorf("ListBySurface(Console) = %v, want %v", consoleNames, wantConsole)
	}

	localExec := r.ListBySurface(SurfaceLocalExec)
	localExecNames := names(localExec)
	wantLocalExec := []string{"shell_exec"}
	if !reflect.DeepEqual(localExecNames, wantLocalExec) {
		t.Errorf("ListBySurface(LocalExec) = %v, want %v", localExecNames, wantLocalExec)
	}
}

func TestRegistryListBySurfaceCombinedBits(t *testing.T) {
	r := NewRegistry()
	r.MustRegister(&fakeTool{name: "shared", surfaces: SurfaceMCP | SurfaceConsole})
	r.MustRegister(&fakeTool{name: "console_only", surfaces: SurfaceConsole})
	r.MustRegister(&fakeTool{name: "mcp_only", surfaces: SurfaceMCP})

	got := names(r.ListBySurface(SurfaceMCP | SurfaceConsole))
	want := []string{"shared"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("ListBySurface(MCP|Console) = %v, want %v (only tools with both bits)", got, want)
	}
}

func TestRegistryGetReturnsFalseForUnknown(t *testing.T) {
	r := NewRegistry()
	if _, ok := r.Get("nonexistent"); ok {
		t.Fatal("Get(unknown) ok = true, want false")
	}
}

func TestRegistryCallPropagatesError(t *testing.T) {
	r := NewRegistry()
	wantErr := errors.New("tool failure")
	tool := &fakeTool{name: "vault_read", callErr: wantErr}
	r.MustRegister(tool)

	got, _ := r.Get("vault_read")
	_, err := got.Call(map[string]any{"path": "x"})
	if !errors.Is(err, wantErr) {
		t.Errorf("Call() err = %v, want %v", err, wantErr)
	}
	if tool.callCount != 1 {
		t.Errorf("callCount = %d, want 1", tool.callCount)
	}
	if tool.lastArgs["path"] != "x" {
		t.Errorf("lastArgs = %v, want path=x", tool.lastArgs)
	}
}

func TestMCPDefinitionMatchesLegacyShape(t *testing.T) {
	tool := &fakeTool{
		name:        "system_doc_get",
		description: "Read one of the managed core documents.",
		surfaces:    SurfaceMCP | SurfaceConsole,
		arguments: []Argument{
			{Name: "name", Type: ArgString, Enum: []string{"system", "progress", "persona", "agent", "identity"}},
		},
		required: []string{"name"},
	}

	got := MCPDefinition(tool)
	want := map[string]any{
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
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("MCPDefinition() mismatch\n got = %#v\nwant = %#v", got, want)
	}
}

func TestMCPInputSchemaOmitsRequiredWhenEmpty(t *testing.T) {
	tool := &fakeTool{
		name:        "managed_status",
		description: "Return managed mode and core document status.",
		arguments:   nil,
		required:    nil,
	}

	got := MCPInputSchema(tool)
	want := map[string]any{
		"type":       "object",
		"properties": map[string]any{},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("MCPInputSchema(no args) = %#v, want %#v", got, want)
	}
	if _, hasRequired := got["required"]; hasRequired {
		t.Error(`MCPInputSchema(no required) included "required" key, want omitted`)
	}
}

func TestMCPInputSchemaDeprecatedAliasDescription(t *testing.T) {
	tool := &fakeTool{
		name:        "vault_list",
		description: "List files under a vault directory.",
		arguments: []Argument{
			{Name: "dir", Type: ArgString},
			{Name: "path", Type: ArgString, DeprecatedAlias: "dir"},
		},
	}

	got := MCPInputSchema(tool)
	properties := got["properties"].(map[string]any)
	pathSchema := properties["path"].(map[string]any)
	if got, want := pathSchema["description"], "deprecated alias for dir"; got != want {
		t.Errorf("path.description = %v, want %v", got, want)
	}
	if dirSchema := properties["dir"].(map[string]any); dirSchema["description"] != nil {
		t.Errorf("dir.description = %v, want absent (no alias, no explicit description)", dirSchema["description"])
	}
}

func TestMCPInputSchemaExplicitDescriptionOverridesAlias(t *testing.T) {
	tool := &fakeTool{
		name: "x",
		arguments: []Argument{
			{
				Name:            "path",
				Type:            ArgString,
				Description:     "explicit description wins",
				DeprecatedAlias: "dir",
			},
		},
	}

	got := MCPInputSchema(tool)
	properties := got["properties"].(map[string]any)
	pathSchema := properties["path"].(map[string]any)
	if got, want := pathSchema["description"], "explicit description wins"; got != want {
		t.Errorf("description = %v, want %v (explicit Description must beat DeprecatedAlias)", got, want)
	}
}

func TestMCPInputSchemaDoesNotAliasRequiredSlice(t *testing.T) {
	required := []string{"a", "b"}
	tool := &fakeTool{
		name:      "x",
		arguments: []Argument{{Name: "a", Type: ArgString}, {Name: "b", Type: ArgString}},
		required:  required,
	}

	got := MCPInputSchema(tool)
	gotRequired := got["required"].([]string)
	gotRequired[0] = "MUTATED"
	if required[0] == "MUTATED" {
		t.Error("MCPInputSchema returned a slice aliasing the tool's Required(); must defensively copy")
	}
}

func TestMCPInputSchemaDoesNotAliasEnumSlice(t *testing.T) {
	enum := []string{"system", "progress"}
	tool := &fakeTool{
		name:      "x",
		arguments: []Argument{{Name: "name", Type: ArgString, Enum: enum}},
		required:  []string{"name"},
	}

	got := MCPInputSchema(tool)
	properties := got["properties"].(map[string]any)
	gotEnum := properties["name"].(map[string]any)["enum"].([]string)
	gotEnum[0] = "MUTATED"
	if enum[0] == "MUTATED" {
		t.Error("MCPInputSchema returned an enum slice aliasing the Argument's Enum; must defensively copy")
	}
}

func TestMCPDefinitionsPreservesOrder(t *testing.T) {
	tools := []Tool{
		&fakeTool{name: "managed_status", description: "A"},
		&fakeTool{name: "vault_read", description: "B", arguments: []Argument{{Name: "path", Type: ArgString}}, required: []string{"path"}},
		&fakeTool{name: "context_pack", description: "C"},
	}

	got := MCPDefinitions(tools)
	if len(got) != 3 {
		t.Fatalf("len = %d, want 3", len(got))
	}
	wantNames := []string{"managed_status", "vault_read", "context_pack"}
	for i, want := range wantNames {
		if got[i]["name"] != want {
			t.Errorf("MCPDefinitions[%d].name = %v, want %v", i, got[i]["name"], want)
		}
	}
}

func TestRegistryNamesIsLexicographic(t *testing.T) {
	r := NewRegistry()
	r.MustRegister(&fakeTool{name: "vault_read"})
	r.MustRegister(&fakeTool{name: "managed_status"})
	r.MustRegister(&fakeTool{name: "draft_list"})

	got := r.Names()
	want := []string{"draft_list", "managed_status", "vault_read"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("Names() = %v, want %v (lexicographic)", got, want)
	}
}

func names(toolList []Tool) []string {
	out := make([]string, 0, len(toolList))
	for _, tool := range toolList {
		out = append(out, tool.Name())
	}
	return out
}
