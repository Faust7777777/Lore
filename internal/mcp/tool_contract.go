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

func toolContracts() []toolContract {
	return []toolContract{
		{
			Name:        "managed_status",
			Description: "Return managed mode and core document status.",
		},
		{
			Name:        "system_doc_get",
			Description: "Read one of the managed core documents.",
			Arguments: []toolArgument{
				{Name: "name", Type: "string", Enum: []string{"system", "progress", "persona", "agent", "identity"}},
			},
			Required: []string{"name"},
		},
		{
			Name:        "vault_read",
			Description: "Read a markdown document from the vault.",
			Arguments:   []toolArgument{{Name: "path", Type: "string"}},
			Required:    []string{"path"},
		},
		{
			Name:        "vault_list",
			Description: "List files under a vault directory.",
			Arguments: []toolArgument{
				{Name: "dir", Type: "string"},
				{Name: "path", Type: "string", DeprecatedAlias: "dir"},
			},
		},
		{
			Name:        "vault_search_text",
			Description: "Search vault text content.",
			Arguments: []toolArgument{
				{Name: "query", Type: "string"},
				{Name: "dir", Type: "string"},
				{Name: "path", Type: "string", DeprecatedAlias: "dir"},
				{Name: "limit", Type: "integer"},
			},
			Required: []string{"query"},
		},
		{
			Name:        "vault_resolve",
			Description: "Resolve a natural-language note reference to vault markdown paths. Returns unique, ambiguous, or not_found.",
			Arguments: []toolArgument{
				{Name: "query", Type: "string"},
				{Name: "dir", Type: "string"},
				{Name: "path", Type: "string", DeprecatedAlias: "dir"},
				{Name: "limit", Type: "integer"},
			},
			Required: []string{"query"},
		},
		{
			Name:        "vault_backlinks",
			Description: "Find backlinks to a vault document.",
			Arguments: []toolArgument{
				{Name: "path", Type: "string"},
				{Name: "limit", Type: "integer"},
			},
			Required: []string{"path"},
		},
		{
			Name:        "doc_classify",
			Description: "Classify a vault document by the current rules.",
			Arguments:   []toolArgument{{Name: "path", Type: "string"}},
			Required:    []string{"path"},
		},
		{
			Name:        "context_pack",
			Description: "Assemble a read-only context pack for a task or document.",
			Arguments: []toolArgument{
				{Name: "target_path", Type: "string"},
				{Name: "path", Type: "string", DeprecatedAlias: "target_path"},
				{Name: "task", Type: "string"},
				{Name: "limit", Type: "integer"},
			},
		},
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
