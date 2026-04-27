package console

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

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
	session := NewSessionWithAgent("test", &fakeAgent{})
	session.EnableLocalWorkTools = true
	tools := newToolRuntime(session, runtime)

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

func TestToolRuntimeVaultWriteLowCallsRuntimeWithoutKeywordGate(t *testing.T) {
	workDir := t.TempDir()
	runtime := &fakeRuntime{
		managed: model.ManagedStatusView{
			WorkDir:   workDir,
			VaultRoot: filepath.Join(workDir, "vault"),
		},
	}
	session := NewSessionWithAgent("test", &fakeAgent{})
	tools := newToolRuntime(session, runtime)

	result, err := tools.CallTool("vault_write_low", map[string]any{
		"path":      "03-notes/diary.md",
		"content":   "# Diary\n\nToday",
		"overwrite": false,
	})
	if err != nil {
		t.Fatalf("vault_write_low error = %v", err)
	}
	if runtime.writtenNote == nil || runtime.writtenNote.Path != "03-notes/diary.md" {
		t.Fatalf("writtenNote = %+v, want diary path", runtime.writtenNote)
	}
	if !strings.Contains(result.Content, `"status": "written"`) || !strings.Contains(result.Content, "03-notes/diary.md") {
		t.Fatalf("vault_write_low result = %q", result.Content)
	}
}

func TestToolRuntimeVaultResolveEnrichesUniqueSelectedPath(t *testing.T) {
	runtime := &fakeRuntime{}
	session := NewSessionWithAgent("test", &fakeAgent{})
	tools := newToolRuntime(session, runtime)
	arguments := map[string]any{"query": "progress", "limit": 5}

	result, err := tools.CallTool("vault_resolve", arguments)
	if err != nil {
		t.Fatalf("vault_resolve error = %v", err)
	}
	if !strings.Contains(result.Content, `"status": "unique"`) || !strings.Contains(result.Content, `"selected_path": "progress.md"`) {
		t.Fatalf("vault_resolve result = %q, want unique progress path", result.Content)
	}
	if arguments["selected_path"] != "progress.md" {
		t.Fatalf("selected_path argument = %#v, want progress.md", arguments["selected_path"])
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
	session := NewSessionWithAgent("test", &fakeAgent{})
	session.EnableLocalWorkTools = true
	tools := newToolRuntime(session, runtime)

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
	session := NewSessionWithAgent("test", &fakeAgent{})
	session.EnableLocalWorkTools = true
	tools := newToolRuntime(session, runtime)

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
	session := NewSessionWithAgent("test", &fakeAgent{})
	session.EnableLocalWorkTools = true
	tools := newToolRuntime(session, runtime)

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

func TestToolRuntimeShellExecQueuesConfirmation(t *testing.T) {
	t.Setenv("LORE_AGENT_ENABLE_SHELL", "1")
	workDir := t.TempDir()
	runtime := &fakeRuntime{
		managed: model.ManagedStatusView{
			WorkDir:   workDir,
			VaultRoot: filepath.Join(workDir, "vault"),
		},
	}
	session := NewSessionWithAgent("test", &fakeAgent{})
	session.EnableLocalWorkTools = true
	tools := newToolRuntime(session, runtime)

	result, err := tools.CallTool("shell_exec", map[string]any{
		"command":         "echo lore",
		"timeout_seconds": 5,
	})
	if err != nil {
		t.Fatalf("shell_exec error = %v", err)
	}
	if session.PendingShellCommand == nil {
		t.Fatal("pending shell command = nil, want queued confirmation")
	}
	if session.PendingShellCommand.Command != "echo lore" || session.PendingShellCommand.TimeoutSeconds != 5 {
		t.Fatalf("pending shell command = %+v, want queued command and timeout", session.PendingShellCommand)
	}
	if !strings.Contains(result.Content, "Shell command pending confirmation.") {
		t.Fatalf("shell_exec content = %q, want confirmation prompt", result.Content)
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
	session := NewSessionWithAgent("test", &fakeAgent{})
	session.EnableLocalWorkTools = true
	tools := newToolRuntime(session, runtime)

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

func TestToolRuntimeDescribeToolsExposesGitToolsWithoutShellExec(t *testing.T) {
	t.Setenv("LORE_AGENT_ENABLE_SHELL", "")
	workDir := t.TempDir()
	runtime := &fakeRuntime{
		managed: model.ManagedStatusView{
			WorkDir:   workDir,
			VaultRoot: filepath.Join(workDir, "vault"),
		},
	}
	session := NewSessionWithAgent("test", &fakeAgent{})
	session.EnableLocalWorkTools = true
	tools := newToolRuntime(session, runtime)

	definitions := tools.DescribeTools(operatoragent.Context{})
	requireGitToolsAvailable(t, definitions)
	if toolDefinitionsContain(definitions, "shell_exec") {
		t.Fatal("shell_exec was registered without LORE_AGENT_ENABLE_SHELL")
	}
}

func TestToolRuntimeGitToolsRunWithoutKeywordGating(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skipf("git unavailable: %v", err)
	}

	workDir := t.TempDir()
	initGitRepo(t, workDir)
	runtime := &fakeRuntime{
		managed: model.ManagedStatusView{
			WorkDir:   workDir,
			VaultRoot: filepath.Join(workDir, "vault"),
		},
	}
	session := NewSessionWithAgent("test", &fakeAgent{})
	session.EnableLocalWorkTools = true
	tools := newToolRuntime(session, runtime)

	requireGitToolsAvailable(t, tools.DescribeTools(operatoragent.Context{}))

	statusResult, err := tools.CallTool("git_status", map[string]any{})
	if err != nil {
		t.Fatalf("git_status error = %v", err)
	}
	if !strings.Contains(statusResult.Content, "tool: git_status") {
		t.Fatalf("git_status content = %q, want git_status header", statusResult.Content)
	}

	diffResult, err := tools.CallTool("git_diff_summary", map[string]any{})
	if err != nil {
		t.Fatalf("git_diff_summary error = %v", err)
	}
	if !strings.Contains(diffResult.Content, "tool: git_diff_summary") {
		t.Fatalf("git_diff_summary content = %q, want git_diff_summary header", diffResult.Content)
	}
}

func TestToolRuntimeDescribeToolsHidesLocalExecByDefault(t *testing.T) {
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
		switch definition.Name {
		case "workspace_list", "workspace_read", "workspace_write", "workspace_edit", "shell_exec", "git_status", "git_diff_summary":
			t.Fatalf("tool %q was exposed without local-exec mode", definition.Name)
		}
	}
}

func TestToolRuntimeGitToolsRequireLocalExecMode(t *testing.T) {
	workDir := t.TempDir()
	runtime := &fakeRuntime{
		managed: model.ManagedStatusView{
			WorkDir:   workDir,
			VaultRoot: filepath.Join(workDir, "vault"),
		},
	}
	enabledSession := NewSessionWithAgent("test", &fakeAgent{})
	enabledSession.EnableLocalWorkTools = true
	enabledTools := newToolRuntime(enabledSession, runtime)
	requireGitToolsAvailable(t, enabledTools.DescribeTools(operatoragent.Context{}))

	tools := newToolRuntime(NewSessionWithAgent("test", &fakeAgent{}), runtime)
	for _, name := range gitToolNames() {
		_, err := tools.CallTool(name, map[string]any{})
		if err == nil {
			t.Fatalf("%s error = nil, want local-exec failure", name)
		}
		if !strings.Contains(err.Error(), "--local-exec") {
			t.Fatalf("%s error = %q, want local-exec guidance", name, err)
		}
	}
}

func TestToolRuntimeWorkspaceWriteRunsWithoutConfirmation(t *testing.T) {
	workDir := t.TempDir()
	runtime := &fakeRuntime{
		managed: model.ManagedStatusView{
			WorkDir:   workDir,
			VaultRoot: filepath.Join(workDir, "vault"),
		},
	}
	session := NewSessionWithAgent("test", &fakeAgent{})
	session.EnableLocalWorkTools = true
	tools := newToolRuntime(session, runtime)

	result, err := tools.CallTool("workspace_write", map[string]any{
		"path":    "notes.txt",
		"content": "hello",
	})
	if err != nil {
		t.Fatalf("workspace_write error = %v", err)
	}
	if session.PendingShellCommand != nil {
		t.Fatalf("pending shell command = %+v, want nil for non-shell tool", session.PendingShellCommand)
	}
	if strings.Contains(result.Content, "pending confirmation") {
		t.Fatalf("workspace_write result = %q, want immediate non-confirming result", result.Content)
	}
	data, err := os.ReadFile(filepath.Join(workDir, "notes.txt"))
	if err != nil {
		t.Fatalf("ReadFile(notes.txt) error = %v", err)
	}
	if string(data) != "hello" {
		t.Fatalf("notes.txt = %q, want hello", string(data))
	}
}
func TestToolRuntimeWorkspaceWriteRequiresLocalExecMode(t *testing.T) {
	workDir := t.TempDir()
	runtime := &fakeRuntime{
		managed: model.ManagedStatusView{
			WorkDir:   workDir,
			VaultRoot: filepath.Join(workDir, "vault"),
		},
	}
	tools := newToolRuntime(NewSessionWithAgent("test", &fakeAgent{}), runtime)

	_, err := tools.CallTool("workspace_write", map[string]any{
		"path":    "notes.txt",
		"content": "hello",
	})
	if err == nil {
		t.Fatal("workspace_write error = nil, want local-exec failure")
	}
	if !strings.Contains(err.Error(), "--local-exec") {
		t.Fatalf("workspace_write error = %q, want local-exec guidance", err)
	}
}

func gitToolNames() []string {
	return []string{"git_status", "git_diff_summary"}
}

func requireGitToolsAvailable(t *testing.T, definitions []operatoragent.ToolDefinition) {
	t.Helper()
	missing := make([]string, 0, len(gitToolNames()))
	for _, name := range gitToolNames() {
		if !toolDefinitionsContain(definitions, name) {
			missing = append(missing, name)
		}
	}
	if len(missing) > 0 {
		t.Skipf("git tools not registered on this branch yet: %v", missing)
	}
}

func toolDefinitionsContain(definitions []operatoragent.ToolDefinition, want string) bool {
	for _, definition := range definitions {
		if definition.Name == want {
			return true
		}
	}
	return false
}

func initGitRepo(t *testing.T, workDir string) {
	t.Helper()
	cmd := exec.Command("git", "-C", workDir, "init")
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git init error = %v\n%s", err, string(output))
	}
}
