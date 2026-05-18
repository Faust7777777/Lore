package tools

// builtin_readonly.go defines the 9 read-only Lore tools that the MCP
// server and the console operator agent both surface today. Each tool is
// a small struct that holds the Harness handle and translates a generic
// arguments map into the typed Harness call.
//
// Names, descriptions, argument schemas, and registration order are the
// canonical live MCP read-only contract. The MCP server derives tools/list
// and tools/call dispatch from this registry data.
//
// All read-only tools live on SurfaceMCP|SurfaceConsole. They have no
// session dependency (Harness alone is sufficient) which is why they
// belong in this package and not in console/consoletools.

const readOnlySurfaces = SurfaceMCP | SurfaceConsole

// RegisterReadOnly registers the 9 built-in read-only tools into reg. The
// order is the externally observable order in MCP tools/list output and must
// remain stable.
func RegisterReadOnly(reg *Registry, h Harness) error {
	tools := []Tool{
		managedStatusTool{h: h},
		systemDocGetTool{h: h},
		vaultReadTool{h: h},
		vaultListTool{h: h},
		vaultSearchTextTool{h: h},
		vaultResolveTool{h: h},
		vaultBacklinksTool{h: h},
		docClassifyTool{h: h},
		contextPackTool{h: h},
	}
	for _, t := range tools {
		if err := reg.Register(t); err != nil {
			return err
		}
	}
	return nil
}

type managedStatusTool struct{ h Harness }

func (t managedStatusTool) Name() string { return "managed_status" }
func (t managedStatusTool) Description() string {
	return "Return managed mode and core document status."
}
func (t managedStatusTool) Surfaces() Surface                  { return readOnlySurfaces }
func (t managedStatusTool) Arguments() []Argument              { return nil }
func (t managedStatusTool) Required() []string                 { return nil }
func (t managedStatusTool) Call(_ map[string]any) (any, error) { return t.h.ManagedStatus() }

type systemDocGetTool struct{ h Harness }

func (t systemDocGetTool) Name() string        { return "system_doc_get" }
func (t systemDocGetTool) Description() string { return "Read one of the managed core documents." }
func (t systemDocGetTool) Surfaces() Surface   { return readOnlySurfaces }
func (t systemDocGetTool) Arguments() []Argument {
	return []Argument{
		{Name: "name", Type: ArgString, Enum: []string{"system", "progress", "persona", "agent", "identity"}},
	}
}
func (t systemDocGetTool) Required() []string { return []string{"name"} }
func (t systemDocGetTool) Call(args map[string]any) (any, error) {
	return t.h.SystemDocGet(getString(args, "name"))
}

type vaultReadTool struct{ h Harness }

func (t vaultReadTool) Name() string        { return "vault_read" }
func (t vaultReadTool) Description() string { return "Read a markdown document from the vault." }
func (t vaultReadTool) Surfaces() Surface   { return readOnlySurfaces }
func (t vaultReadTool) Arguments() []Argument {
	return []Argument{{Name: "path", Type: ArgString}}
}
func (t vaultReadTool) Required() []string { return []string{"path"} }
func (t vaultReadTool) Call(args map[string]any) (any, error) {
	return t.h.VaultRead(getString(args, "path"))
}

type vaultListTool struct{ h Harness }

func (t vaultListTool) Name() string        { return "vault_list" }
func (t vaultListTool) Description() string { return "List files under a vault directory." }
func (t vaultListTool) Surfaces() Surface   { return readOnlySurfaces }
func (t vaultListTool) Arguments() []Argument {
	return []Argument{
		{Name: "dir", Type: ArgString},
		{Name: "path", Type: ArgString, DeprecatedAlias: "dir"},
	}
}
func (t vaultListTool) Required() []string { return nil }
func (t vaultListTool) Call(args map[string]any) (any, error) {
	return t.h.VaultList(getDirArg(args))
}

type vaultSearchTextTool struct{ h Harness }

func (t vaultSearchTextTool) Name() string        { return "vault_search_text" }
func (t vaultSearchTextTool) Description() string { return "Search vault text content." }
func (t vaultSearchTextTool) Surfaces() Surface   { return readOnlySurfaces }
func (t vaultSearchTextTool) Arguments() []Argument {
	return []Argument{
		{Name: "query", Type: ArgString},
		{Name: "dir", Type: ArgString},
		{Name: "path", Type: ArgString, DeprecatedAlias: "dir"},
		{Name: "limit", Type: ArgInteger},
	}
}
func (t vaultSearchTextTool) Required() []string { return []string{"query"} }
func (t vaultSearchTextTool) Call(args map[string]any) (any, error) {
	return t.h.VaultSearchText(getString(args, "query"), getDirArg(args), getInt(args, "limit", 10))
}

type vaultResolveTool struct{ h Harness }

func (t vaultResolveTool) Name() string { return "vault_resolve" }
func (t vaultResolveTool) Description() string {
	return "Resolve a natural-language note reference to vault markdown paths. Returns unique, ambiguous, or not_found."
}
func (t vaultResolveTool) Surfaces() Surface { return readOnlySurfaces }
func (t vaultResolveTool) Arguments() []Argument {
	return []Argument{
		{Name: "query", Type: ArgString},
		{Name: "dir", Type: ArgString},
		{Name: "path", Type: ArgString, DeprecatedAlias: "dir"},
		{Name: "limit", Type: ArgInteger},
	}
}
func (t vaultResolveTool) Required() []string { return []string{"query"} }
func (t vaultResolveTool) Call(args map[string]any) (any, error) {
	return t.h.VaultResolve(getString(args, "query"), getDirArg(args), getInt(args, "limit", 5))
}

type vaultBacklinksTool struct{ h Harness }

func (t vaultBacklinksTool) Name() string        { return "vault_backlinks" }
func (t vaultBacklinksTool) Description() string { return "Find backlinks to a vault document." }
func (t vaultBacklinksTool) Surfaces() Surface   { return readOnlySurfaces }
func (t vaultBacklinksTool) Arguments() []Argument {
	return []Argument{
		{Name: "path", Type: ArgString},
		{Name: "limit", Type: ArgInteger},
	}
}
func (t vaultBacklinksTool) Required() []string { return []string{"path"} }
func (t vaultBacklinksTool) Call(args map[string]any) (any, error) {
	return t.h.VaultBacklinks(getString(args, "path"), getInt(args, "limit", 10))
}

type docClassifyTool struct{ h Harness }

func (t docClassifyTool) Name() string { return "doc_classify" }
func (t docClassifyTool) Description() string {
	return "Classify a vault document by the current rules."
}
func (t docClassifyTool) Surfaces() Surface { return readOnlySurfaces }
func (t docClassifyTool) Arguments() []Argument {
	return []Argument{{Name: "path", Type: ArgString}}
}
func (t docClassifyTool) Required() []string { return []string{"path"} }
func (t docClassifyTool) Call(args map[string]any) (any, error) {
	// DocClassify on Harness returns a single value, not an (any, error)
	// pair. Wrap it so the registry's uniform Call signature holds.
	return t.h.DocClassify(getString(args, "path")), nil
}

type contextPackTool struct{ h Harness }

func (t contextPackTool) Name() string { return "context_pack" }
func (t contextPackTool) Description() string {
	return "Assemble a read-only context pack for a task or document."
}
func (t contextPackTool) Surfaces() Surface { return readOnlySurfaces }
func (t contextPackTool) Arguments() []Argument {
	return []Argument{
		{Name: "target_path", Type: ArgString},
		{Name: "path", Type: ArgString, DeprecatedAlias: "target_path"},
		{Name: "task", Type: ArgString},
		{Name: "limit", Type: ArgInteger},
	}
}
func (t contextPackTool) Required() []string { return nil }
func (t contextPackTool) Call(args map[string]any) (any, error) {
	return t.h.ContextPack(getTargetPathArg(args), getString(args, "task"), getInt(args, "limit", 6))
}
