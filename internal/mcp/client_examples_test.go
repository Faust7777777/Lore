package mcp

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestExternalClientExamplesStartLoreMCP(t *testing.T) {
	exampleDir := filepath.Join("..", "..", "docs", "integrations", "examples")
	for _, name := range []string{
		"generic-mcp.json",
		"claude-desktop-mcp.json",
		"claude-code-project.mcp.json",
		"gemini-cli-settings.json",
	} {
		t.Run(name, func(t *testing.T) {
			var config struct {
				MCPServers map[string]stdioExampleServer `json:"mcpServers"`
			}
			readExampleJSON(t, filepath.Join(exampleDir, name), &config)
			server, ok := config.MCPServers["lore"]
			if !ok {
				t.Fatalf("%s missing mcpServers.lore", name)
			}
			assertStdioLoreCommand(t, name, server.Command, server.Args)
			assertClientKeyEnv(t, name, server.Env)
		})
	}
}

func TestOpenCodeExampleStartsLoreMCP(t *testing.T) {
	exampleDir := filepath.Join("..", "..", "docs", "integrations", "examples")
	var config struct {
		MCP map[string]struct {
			Type        string            `json:"type"`
			Command     []string          `json:"command"`
			Enabled     bool              `json:"enabled"`
			Environment map[string]string `json:"environment"`
		} `json:"mcp"`
	}
	readExampleJSON(t, filepath.Join(exampleDir, "opencode.jsonc"), &config)
	server, ok := config.MCP["lore"]
	if !ok {
		t.Fatal("opencode.jsonc missing mcp.lore")
	}
	if server.Type != "local" {
		t.Fatalf("opencode lore type = %q, want local", server.Type)
	}
	if !server.Enabled {
		t.Fatal("opencode lore server is not enabled")
	}
	if len(server.Command) < 3 {
		t.Fatalf("opencode lore command = %#v, want lore executable, mcp, workdir", server.Command)
	}
	assertStdioLoreCommand(t, "opencode.jsonc", server.Command[0], server.Command[1:])
	assertClientKeyEnv(t, "opencode.jsonc", server.Environment)
}

type stdioExampleServer struct {
	Command string            `json:"command"`
	Args    []string          `json:"args"`
	Env     map[string]string `json:"env"`
}

func readExampleJSON(t *testing.T, path string, out any) {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile(%s) error = %v", path, err)
	}
	if err := json.Unmarshal(data, out); err != nil {
		t.Fatalf("json.Unmarshal(%s) error = %v", path, err)
	}
}

func assertStdioLoreCommand(t *testing.T, name string, command string, args []string) {
	t.Helper()
	if !strings.Contains(strings.ToLower(command), "lore") {
		t.Fatalf("%s command = %q, want lore executable", name, command)
	}
	if len(args) < 2 {
		t.Fatalf("%s args = %#v, want mcp and explicit workdir", name, args)
	}
	if args[0] != "mcp" {
		t.Fatalf("%s args[0] = %q, want mcp", name, args[0])
	}
	if strings.TrimSpace(args[1]) == "" {
		t.Fatalf("%s args[1] is empty, want explicit workdir", name)
	}
}

func assertClientKeyEnv(t *testing.T, name string, env map[string]string) {
	t.Helper()
	if strings.TrimSpace(env["LORE_CLIENT_KEY"]) == "" {
		t.Fatalf("%s missing non-empty LORE_CLIENT_KEY placeholder", name)
	}
}
