package lore

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

type sdkContractTool struct {
	Name       string            `json:"name"`
	Properties []string          `json:"properties"`
	AliasFor   map[string]string `json:"alias_for,omitempty"`
}

func TestREADMEArgumentTableMatchesContractArtifact(t *testing.T) {
	contract := loadSDKContractArtifact(t)
	readmeRows := loadREADMEArgumentRows(t)
	if len(readmeRows) != len(contract) {
		t.Fatalf("README argument rows = %d, contract tools = %d", len(readmeRows), len(contract))
	}
	for _, tool := range contract {
		row, ok := readmeRows[tool.Name]
		if !ok {
			t.Fatalf("README missing argument row for %s", tool.Name)
		}
		standardArgs := standardArgs(tool)
		if !reflect.DeepEqual(normalizeSlice(row.StandardArgs), normalizeSlice(standardArgs)) {
			t.Fatalf("%s README standard args = %#v, want %#v", tool.Name, row.StandardArgs, standardArgs)
		}
		aliases := aliases(tool)
		if !reflect.DeepEqual(normalizeMap(row.DeprecatedAliases), normalizeMap(aliases)) {
			t.Fatalf("%s README deprecated aliases = %#v, want %#v", tool.Name, row.DeprecatedAliases, aliases)
		}
	}
}

func loadSDKContractArtifact(t *testing.T) []sdkContractTool {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("..", "..", "..", "docs", "contracts", "mcp-sdk-tools-v0.json"))
	if err != nil {
		t.Fatalf("ReadFile(mcp-sdk-tools-v0.json) error = %v", err)
	}
	var tools []sdkContractTool
	if err := json.Unmarshal(data, &tools); err != nil {
		t.Fatalf("json.Unmarshal(mcp-sdk-tools-v0.json) error = %v", err)
	}
	return tools
}

type readmeArgumentRow struct {
	StandardArgs      []string
	DeprecatedAliases map[string]string
}

func loadREADMEArgumentRows(t *testing.T) map[string]readmeArgumentRow {
	t.Helper()
	data, err := os.ReadFile("README.md")
	if err != nil {
		t.Fatalf("ReadFile(README.md) error = %v", err)
	}
	rows := make(map[string]readmeArgumentRow)
	for _, line := range strings.Split(readmeSection(t, string(data), "## Argument Contract"), "\n") {
		line = strings.TrimSpace(line)
		if !strings.HasPrefix(line, "| `") || strings.Contains(line, "---") {
			continue
		}
		columns := strings.Split(line, "|")
		if len(columns) < 4 {
			continue
		}
		tool := strings.Trim(strings.TrimSpace(columns[1]), "`")
		if tool == "Tool" || tool == "" {
			continue
		}
		rows[tool] = readmeArgumentRow{
			StandardArgs:      parseREADMEArgs(columns[2]),
			DeprecatedAliases: parseREADMEAliases(columns[3]),
		}
	}
	return rows
}

func readmeSection(t *testing.T, content string, heading string) string {
	t.Helper()
	start := strings.Index(content, heading)
	if start < 0 {
		t.Fatalf("README missing section %q", heading)
	}
	section := content[start+len(heading):]
	if end := strings.Index(section, "\n## "); end >= 0 {
		section = section[:end]
	}
	return section
}

func standardArgs(tool sdkContractTool) []string {
	args := make([]string, 0, len(tool.Properties))
	for _, property := range tool.Properties {
		if _, deprecated := tool.AliasFor[property]; deprecated {
			continue
		}
		args = append(args, property)
	}
	return args
}

func aliases(tool sdkContractTool) map[string]string {
	if len(tool.AliasFor) == 0 {
		return nil
	}
	out := make(map[string]string, len(tool.AliasFor))
	for alias, target := range tool.AliasFor {
		out[alias] = target
	}
	return out
}

func parseREADMEArgs(column string) []string {
	column = strings.TrimSpace(column)
	if column == "none" {
		return nil
	}
	parts := strings.Split(column, ",")
	args := make([]string, 0, len(parts))
	for _, part := range parts {
		arg := strings.Trim(strings.TrimSpace(part), "`")
		if arg != "" {
			args = append(args, arg)
		}
	}
	return args
}

func parseREADMEAliases(column string) map[string]string {
	column = strings.TrimSpace(column)
	if column == "none" {
		return nil
	}
	aliases := make(map[string]string)
	for _, segment := range strings.Split(column, ",") {
		fields := strings.Fields(segment)
		if len(fields) >= 4 && strings.Trim(fields[1], "`") == "alias" && fields[2] == "for" {
			aliases[strings.Trim(fields[0], "`")] = strings.Trim(fields[3], "`")
		}
	}
	if len(aliases) == 0 {
		return nil
	}
	return aliases
}

func normalizeSlice(values []string) []string {
	if len(values) == 0 {
		return nil
	}
	return values
}

func normalizeMap(values map[string]string) map[string]string {
	if len(values) == 0 {
		return nil
	}
	return values
}
