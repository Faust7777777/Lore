package tools

// MCPInputSchema renders a Tool's argument list as a JSON-serializable
// inputSchema map matching the live MCP tools/list shape.
//
// Output shape:
//
//	{
//	  "type":       "object",
//	  "properties": { argname: { "type": "...", "enum": [...], "description": "..." }, ... },
//	  "required":   [ "argname", ... ]    // omitted when no required arguments
//	}
//
// Each argument's per-property schema:
//
//   - "type" is always present
//   - "enum" appears only when the argument has a non-empty Enum list (a
//     defensive copy is used so the registry's slice is not aliased)
//   - "description" is set from Argument.Description when non-empty;
//     otherwise, when DeprecatedAlias is set, it is auto-generated as
//     "deprecated alias for <alias>"; otherwise omitted
//
// The output is deliberately a map[string]any because the MCP server
// already produces map values for json.Marshal; switching to a typed
// struct would alter field ordering and complicate the byte-equivalence
// test in commit 2.
func MCPInputSchema(tool Tool) map[string]any {
	args := tool.Arguments()
	properties := make(map[string]any, len(args))
	for _, arg := range args {
		properties[arg.Name] = mcpArgumentSchema(arg)
	}
	schema := map[string]any{
		"type":       "object",
		"properties": properties,
	}
	if required := tool.Required(); len(required) > 0 {
		// Copy to avoid leaking the registry's backing slice into the
		// JSON-bound map. This keeps MCP schema output stable and prevents
		// callers from mutating registry-owned slices.
		copied := make([]string, len(required))
		copy(copied, required)
		schema["required"] = copied
	}
	return schema
}

// MCPDefinition renders one entry of an MCP tools/list response:
//
//	{
//	  "name":        "...",
//	  "description": "...",
//	  "inputSchema": { ... as MCPInputSchema ... }
//	}
//
// Used by internal/mcp/server.go as the only live MCP schema source. The
// shape must remain stable so external MCP callers and snapshot tests do not
// perceive unintended drift.
func MCPDefinition(tool Tool) map[string]any {
	return map[string]any{
		"name":        tool.Name(),
		"description": tool.Description(),
		"inputSchema": MCPInputSchema(tool),
	}
}

// MCPDefinitions renders the supplied tools as a tools/list array. Order is
// preserved exactly; callers should pass the result of
// Registry.ListBySurface(SurfaceMCP) to keep MCP output deterministic.
func MCPDefinitions(toolList []Tool) []map[string]any {
	out := make([]map[string]any, 0, len(toolList))
	for _, tool := range toolList {
		out = append(out, MCPDefinition(tool))
	}
	return out
}

func mcpArgumentSchema(arg Argument) map[string]any {
	schema := map[string]any{"type": string(arg.Type)}
	if len(arg.Enum) > 0 {
		// Defensive copy; callers should not observe or mutate registry-owned
		// enum slices through the JSON schema map.
		copied := make([]string, len(arg.Enum))
		copy(copied, arg.Enum)
		schema["enum"] = copied
	}
	switch {
	case arg.Description != "":
		schema["description"] = arg.Description
	case arg.DeprecatedAlias != "":
		schema["description"] = "deprecated alias for " + arg.DeprecatedAlias
	}
	return schema
}
