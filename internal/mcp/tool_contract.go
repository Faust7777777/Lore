package mcp

type toolContract struct {
	Name        string
	Description string
	Arguments   []toolArgument
	Required    []string
}

type toolArgument struct {
	Name            string
	Type            string
	Enum            []string
	DeprecatedAlias string
}

// toolContracts returns the legacy proposal-tool contracts that have not
// yet migrated to the shared tools.Registry. The 9 read-only tools
// (managed_status, system_doc_get, vault_read, vault_list,
// vault_search_text, vault_resolve, vault_backlinks, doc_classify,
// context_pack) now live in internal/tools and are surfaced through
// Server.registry. Commit 3 of the tool-registry refactor will move the
// proposal tools too, after which this function and its helpers will be
// deleted.
func toolContracts() []toolContract {
	return []toolContract{
		{
			Name:        "persona_update_propose",
			Description: "Submit a persona update proposal for Lore review. Creates a pending draft; does not write or apply the persona document.",
			Arguments: []toolArgument{
				{Name: "field", Type: "string"},
				{Name: "current_value", Type: "string"},
				{Name: "proposed_value", Type: "string"},
				{Name: "evidence", Type: "string"},
				{Name: "confidence", Type: "string", Enum: []string{"low", "medium", "high"}},
				{Name: "reason", Type: "string"},
				{Name: "source", Type: "string"},
				{Name: "observed_at", Type: "string"},
			},
			Required: []string{"field", "proposed_value", "evidence", "confidence", "reason", "source", "observed_at"},
		},
		{
			Name:        "markdown_note_propose",
			Description: "Submit an ordinary markdown note proposal for Lore review. Creates a pending draft; does not write the note and does not apply any draft.",
			Arguments: []toolArgument{
				{Name: "target_path", Type: "string"},
				{Name: "title", Type: "string"},
				{Name: "content", Type: "string"},
				{Name: "source_kind", Type: "string", Enum: []string{"class", "meeting", "development", "conversation", "research", "other"}},
				{Name: "evidence", Type: "string"},
				{Name: "reason", Type: "string"},
				{Name: "source", Type: "string"},
				{Name: "observed_at", Type: "string"},
				{Name: "task_context", Type: "string"},
				{Name: "course", Type: "string"},
				{Name: "topic", Type: "string"},
				{Name: "dedupe_key", Type: "string"},
			},
			Required: []string{"target_path", "title", "content", "source_kind", "evidence", "reason", "source", "observed_at"},
		},
	}
}

func toolDefinitions() []map[string]any {
	contracts := toolContracts()
	definitions := make([]map[string]any, 0, len(contracts))
	for _, contract := range contracts {
		definitions = append(definitions, contract.toolDefinition())
	}
	return definitions
}

func (c toolContract) toolDefinition() map[string]any {
	properties := make(map[string]any, len(c.Arguments))
	for _, argument := range c.Arguments {
		properties[argument.Name] = argument.schema()
	}
	schema := map[string]any{
		"type":       "object",
		"properties": properties,
	}
	if len(c.Required) > 0 {
		schema["required"] = append([]string(nil), c.Required...)
	}
	return map[string]any{
		"name":        c.Name,
		"description": c.Description,
		"inputSchema": schema,
	}
}

func (a toolArgument) schema() map[string]any {
	schema := map[string]any{"type": a.Type}
	if len(a.Enum) > 0 {
		schema["enum"] = append([]string(nil), a.Enum...)
	}
	if a.DeprecatedAlias != "" {
		schema["description"] = "deprecated alias for " + a.DeprecatedAlias
	}
	return schema
}
