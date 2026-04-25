package operatoragent

import (
	"encoding/json"
	"strings"

	openai "obsidian-harness/internal/llm/openai"
)

func openAIToolDefinitions(tools []ToolDefinition) []openai.ToolDefinition {
	if len(tools) == 0 {
		return nil
	}

	definitions := make([]openai.ToolDefinition, 0, len(tools))
	for _, tool := range tools {
		name := strings.TrimSpace(tool.Name)
		if name == "" {
			continue
		}
		definitions = append(definitions, openai.ToolDefinition{
			Name:        name,
			Description: toolDescription(tool),
			Parameters:  inferToolParametersSchema(tool.Arguments),
			Strict:      false,
		})
	}
	return definitions
}

func toolDescription(tool ToolDefinition) string {
	description := strings.TrimSpace(tool.Description)
	if description != "" {
		return description
	}
	return "Lore tool " + strings.TrimSpace(tool.Name)
}
func inferToolParametersSchema(example string) map[string]any {
	example = strings.TrimSpace(example)
	if example == "" {
		return emptyObjectSchema()
	}

	var parsed any
	if err := json.Unmarshal([]byte(example), &parsed); err != nil {
		return emptyObjectSchema()
	}

	object, ok := parsed.(map[string]any)
	if !ok {
		return emptyObjectSchema()
	}
	return inferJSONSchema(object)
}

func inferJSONSchema(value any) map[string]any {
	switch typed := value.(type) {
	case map[string]any:
		properties := make(map[string]any, len(typed))
		for key, child := range typed {
			properties[key] = inferJSONSchema(child)
		}
		return map[string]any{
			"type":       "object",
			"properties": properties,
		}
	case []any:
		items := map[string]any{}
		if len(typed) > 0 {
			items = inferJSONSchema(typed[0])
		}
		return map[string]any{
			"type":  "array",
			"items": items,
		}
	case bool:
		return map[string]any{"type": "boolean"}
	case float64:
		return map[string]any{"type": "number"}
	case string:
		return map[string]any{"type": "string"}
	case nil:
		return map[string]any{}
	default:
		return map[string]any{"type": "string"}
	}
}

func emptyObjectSchema() map[string]any {
	return map[string]any{
		"type":       "object",
		"properties": map[string]any{},
	}
}
