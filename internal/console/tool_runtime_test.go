package console

import (
	"os"
	"path/filepath"
	osruntime "runtime"
	"strings"
	"testing"
	"time"

	"obsidian-harness/internal/model"
	"obsidian-harness/internal/operatoragent"
)

func TestToolRuntimeWorkspaceWritePreservesWhitespace(t *testing.T) {
	workDir := t.TempDir()
	runtime := &fakeRuntime{
		managed: model.ManagedStatusView{
			WorkDir:   workDir,
			VaultRoot: filepath.Join(workDir, "vault"),
		},
	}
	tools := newToolRuntime(NewSessionWithAgent("test", &fakeAgent{}), runtime)

	content := "  leading\ntrailing  \n"
	_, err := tools.CallTool("workspace_write", map[string]any{
		"path":    "notes.txt",
		"content": content,
	})
	if err != nil {
		t.Fatalf("workspace_write error = %v", err)
	}
	data, err := os.ReadFile(filepath.Join(workDir, "notes.txt"))
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}
	if string(data) != content {
		t.Fatalf("content = %q, want preserved %q", string(data), content)
	}
}

func TestToolRuntimeWorkspaceWriteBlocksVaultAndState(t *testing.T) {
	workDir := t.TempDir()
	runtime := &fakeRuntime{
		managed: model.ManagedStatusView{
			WorkDir:   workDir,
			VaultRoot: filepath.Join(workDir, "vault"),
		},
	}
	tools := newToolRuntime(NewSessionWithAgent("test", &fakeAgent{}), runtime)

	for _, path := range []string{"vault/note.md", "state/store.json"} {
		_, err := tools.CallTool("workspace_write", map[string]any{
			"path":    path,
			"content": "x",
		})
		if err == nil {
			t.Fatalf("workspace_write(%s) error = nil, want blocked", path)
		}
		if !strings.Contains(err.Error(), "Lore-managed vault/state") {
			t.Fatalf("workspace_write(%s) error = %q, want vault/state block", path, err)
		}
	}
}

func TestToolRuntimeWorkspaceWriteAllowsNewNestedPath(t *testing.T) {
	workDir := t.TempDir()
	runtime := &fakeRuntime{
		managed: model.ManagedStatusView{
			WorkDir:   workDir,
			VaultRoot: filepath.Join(workDir, "vault"),
		},
	}
	tools := newToolRuntime(NewSessionWithAgent("test", &fakeAgent{}), runtime)

	_, err := tools.CallTool("workspace_write", map[string]any{
		"path":    "src/new/file.txt",
		"content": "nested",
	})
	if err != nil {
		t.Fatalf("workspace_write nested error = %v", err)
	}
	data, err := os.ReadFile(filepath.Join(workDir, "src", "new", "file.txt"))
	if err != nil {
		t.Fatalf("ReadFile(nested) error = %v", err)
	}
	if string(data) != "nested" {
		t.Fatalf("nested content = %q, want nested", string(data))
	}
}

func TestToolRuntimeWorkspaceWriteBlocksSymlinkToVault(t *testing.T) {
	workDir := t.TempDir()
	vaultRoot := filepath.Join(workDir, "vault")
	if err := os.MkdirAll(vaultRoot, 0o755); err != nil {
		t.Fatalf("MkdirAll(vault) error = %v", err)
	}
	linkPath := filepath.Join(workDir, "vault-link")
	if err := os.Symlink(vaultRoot, linkPath); err != nil {
		t.Skipf("symlink unavailable on this platform: %v", err)
	}
	runtime := &fakeRuntime{
		managed: model.ManagedStatusView{
			WorkDir:   workDir,
			VaultRoot: vaultRoot,
		},
	}
	tools := newToolRuntime(NewSessionWithAgent("test", &fakeAgent{}), runtime)

	_, err := tools.CallTool("workspace_write", map[string]any{
		"path":    "vault-link/note.md",
		"content": "bypass",
	})
	if err == nil {
		t.Fatal("workspace_write through vault symlink error = nil, want blocked")
	}
	if !strings.Contains(err.Error(), "Lore-managed vault/state") {
		t.Fatalf("workspace_write symlink error = %q, want vault/state block", err)
	}
}

func TestToolRuntimeShellExecReturnsOutputOnFailure(t *testing.T) {
	t.Setenv("LORE_AGENT_ENABLE_SHELL", "1")
	workDir := t.TempDir()
	runtime := &fakeRuntime{
		managed: model.ManagedStatusView{
			WorkDir:   workDir,
			VaultRoot: filepath.Join(workDir, "vault"),
		},
	}
	session := NewSessionWithAgent("test", &fakeAgent{})
	session.Now = func() time.Time { return time.Date(2026, 4, 22, 9, 0, 0, 0, time.Local) }
	tools := newToolRuntime(session, runtime)

	command := "printf lore-shell-failure; exit 7"
	if osruntime.GOOS == "windows" {
		command = "Write-Output lore-shell-failure; exit 7"
	}
	result, err := tools.CallTool("shell_exec", map[string]any{
		"command":         command,
		"timeout_seconds": 5,
	})
	if err == nil {
		t.Fatal("shell_exec error = nil, want non-zero exit error")
	}
	if !strings.Contains(result.Content, "lore-shell-failure") || !strings.Contains(result.Content, "exit_code: 7") {
		t.Fatalf("shell_exec content = %q, want failure output and exit code", result.Content)
	}
}

func TestToolRuntimeShellExecDisabledByDefault(t *testing.T) {
	t.Setenv("LORE_AGENT_ENABLE_SHELL", "")
	workDir := t.TempDir()
	runtime := &fakeRuntime{
		managed: model.ManagedStatusView{
			WorkDir:   workDir,
			VaultRoot: filepath.Join(workDir, "vault"),
		},
	}
	tools := newToolRuntime(NewSessionWithAgent("test", &fakeAgent{}), runtime)

	definitions := tools.DescribeTools(operatoragent.Context{})
	for _, definition := range definitions {
		if definition.Name == "shell_exec" {
			t.Fatal("shell_exec was registered without LORE_AGENT_ENABLE_SHELL")
		}
	}

	_, err := tools.CallTool("shell_exec", map[string]any{
		"command": "echo nope",
	})
	if err == nil {
		t.Fatal("shell_exec error = nil, want disabled error")
	}
	if !strings.Contains(err.Error(), "disabled") {
		t.Fatalf("shell_exec error = %q, want disabled", err)
	}
}
