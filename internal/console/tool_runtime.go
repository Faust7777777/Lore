package console

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	osruntime "runtime"
	"strings"
	"time"

	"obsidian-harness/internal/model"
	"obsidian-harness/internal/operatoragent"
	"obsidian-harness/internal/tui"
	"obsidian-harness/internal/vault"
)

const workspaceWriteTempSuffix = ".lore-console-tmp"

type toolRuntime struct {
	session *Session
	runtime Runtime
}

func newToolRuntime(session *Session, runtime Runtime) operatoragent.ToolRuntime {
	return toolRuntime{
		session: session,
		runtime: runtime,
	}
}

func (r toolRuntime) DescribeTools(_ operatoragent.Context) []operatoragent.ToolDefinition {
	tools := []operatoragent.ToolDefinition{
		{Name: "managed_status", Description: "Render the managed status view for the current Lore workspace."},
		{Name: "draft_list", Description: "Render the draft inbox. Use pending_only=true to focus on drafts awaiting review.", Arguments: `{"pending_only":true}`},
		{Name: "draft_review", Description: "Render the full review view for a draft and focus it for follow-up actions.", Arguments: `{"draft_id":"optional","use_focused_draft":false}`},
		{Name: "draft_approve", Description: "Approve a draft. Prefer use_focused_draft=true after reviewing.", Arguments: `{"draft_id":"optional","use_focused_draft":true}`},
		{Name: "draft_reject", Description: "Reject a draft.", Arguments: `{"draft_id":"optional","use_focused_draft":true}`},
		{Name: "draft_request_revision", Description: "Request revision on a draft.", Arguments: `{"draft_id":"optional","use_focused_draft":true}`},
		{Name: "draft_apply", Description: "Apply an approved draft to the vault.", Arguments: `{"draft_id":"optional","use_focused_draft":true}`},
		{Name: "process_sink_day", Description: "Render checkpoint and daily report status for one agent day.", Arguments: `{"agent_id":"codex","day":"YYYY-MM-DD"}`},
		{Name: "system_doc_get", Description: "Read a managed core document by name.", Arguments: `{"name":"system|progress|persona"}`},
		{Name: "vault_read", Description: "Read one markdown note from the managed vault.", Arguments: `{"path":"relative/path.md"}`},
		{Name: "vault_list", Description: "List files and folders under one vault directory.", Arguments: `{"dir":"relative/dir"}`},
		{Name: "vault_search_text", Description: "Search markdown text in the vault.", Arguments: `{"query":"text","dir":"","limit":5}`},
		{Name: "vault_backlinks", Description: "Find backlink mentions of one vault note.", Arguments: `{"path":"relative/path.md","limit":5}`},
		{Name: "doc_classify", Description: "Return the inferred document class for one vault path.", Arguments: `{"path":"relative/path.md"}`},
		{Name: "context_pack", Description: "Assemble a read-only Lore context pack for a task or target note.", Arguments: `{"target_path":"optional/path.md","task":"what you need","limit":6}`},
		{Name: "vault_write_low", Description: "Write a low-governance markdown note inside the vault. Runtime rejects managed core docs, plans, process-sink docs, hidden dirs, and non-markdown files.", Arguments: `{"path":"notes/diary.md","content":"...","overwrite":false}`},
	}
	if r.localWorkToolsEnabled() {
		tools = append(tools,
			operatoragent.ToolDefinition{Name: "workspace_list", Description: "List files under the local workdir outside Lore vault/state.", Arguments: `{"path":"."}`},
			operatoragent.ToolDefinition{Name: "workspace_read", Description: "Read a local workspace file outside Lore vault/state.", Arguments: `{"path":"relative/path"}`},
			operatoragent.ToolDefinition{Name: "workspace_write", Description: "Write or overwrite a local workspace file outside Lore vault/state.", Arguments: `{"path":"relative/path","content":"..."}`},
			operatoragent.ToolDefinition{Name: "workspace_edit", Description: "Replace exact text inside a local workspace file outside Lore vault/state.", Arguments: `{"path":"relative/path","old":"exact old text","new":"replacement","replace_all":false}`},
		)
		if shellToolsEnabled() {
			tools = append(tools, operatoragent.ToolDefinition{
				Name:        "shell_exec",
				Description: "Run one local shell command in the workdir. Unsafe profile only; use only for explicit local execution requests.",
				Arguments:   `{"command":"dir","timeout_seconds":30}`,
			})
		}
	}
	return tools
}

func (r toolRuntime) CallTool(name string, arguments map[string]any) (operatoragent.ToolResult, error) {
	switch strings.TrimSpace(name) {
	case "managed_status":
		view, err := r.runtime.ManagedStatus()
		if err != nil {
			return operatoragent.ToolResult{}, err
		}
		return operatoragent.ToolResult{Content: tui.RenderManagedStatus(r.session.Version, view)}, nil
	case "draft_list":
		drafts, err := r.runtime.ListDrafts()
		if err != nil {
			return operatoragent.ToolResult{}, err
		}
		if boolArg(arguments, "pending_only") {
			drafts = filterDraftsByStateLocal(drafts, model.DraftPendingReview)
		}
		return operatoragent.ToolResult{Content: tui.RenderDraftList(drafts)}, nil
	case "draft_review":
		draftID, err := r.session.resolveDraftID(r.draftDecision(arguments), r.runtime, model.DraftPendingReview)
		if err != nil {
			return operatoragent.ToolResult{}, err
		}
		review, err := r.runtime.ReviewDraft(draftID)
		if err != nil {
			return operatoragent.ToolResult{}, err
		}
		r.session.CurrentDraftID = review.Draft.ID
		return operatoragent.ToolResult{Content: tui.RenderDraftReview(review)}, nil
	case "draft_approve":
		draftID, err := r.session.resolveDraftID(r.draftDecision(arguments), r.runtime, model.DraftPendingReview)
		if err != nil {
			return operatoragent.ToolResult{}, err
		}
		draft, err := r.runtime.ApproveDraft(draftID)
		if err != nil {
			return operatoragent.ToolResult{}, err
		}
		r.session.CurrentDraftID = draft.ID
		return operatoragent.ToolResult{Content: tui.RenderDraftActionResult("approve", draft)}, nil
	case "draft_reject":
		draftID, err := r.session.resolveDraftID(r.draftDecision(arguments), r.runtime, model.DraftPendingReview)
		if err != nil {
			return operatoragent.ToolResult{}, err
		}
		draft, err := r.runtime.RejectDraft(draftID)
		if err != nil {
			return operatoragent.ToolResult{}, err
		}
		r.session.CurrentDraftID = draft.ID
		return operatoragent.ToolResult{Content: tui.RenderDraftActionResult("reject", draft)}, nil
	case "draft_request_revision":
		draftID, err := r.session.resolveDraftID(r.draftDecision(arguments), r.runtime, model.DraftPendingReview)
		if err != nil {
			return operatoragent.ToolResult{}, err
		}
		draft, err := r.runtime.RequestDraftRevision(draftID)
		if err != nil {
			return operatoragent.ToolResult{}, err
		}
		r.session.CurrentDraftID = draft.ID
		return operatoragent.ToolResult{Content: tui.RenderDraftActionResult("request-revision", draft)}, nil
	case "draft_apply":
		draftID, err := r.session.resolveDraftID(r.draftDecision(arguments), r.runtime, model.DraftApproved)
		if err != nil {
			return operatoragent.ToolResult{}, err
		}
		draft, err := r.runtime.ApplyDraft(draftID)
		if err != nil {
			return operatoragent.ToolResult{}, err
		}
		r.session.CurrentDraftID = draft.ID
		return operatoragent.ToolResult{Content: tui.RenderDraftActionResult("apply", draft)}, nil
	case "process_sink_day":
		day, err := r.dayArg(arguments)
		if err != nil {
			return operatoragent.ToolResult{}, err
		}
		view, err := r.runtime.ProcessSinkDay(stringArg(arguments, "agent_id", r.session.DefaultAgentID), day)
		if err != nil {
			return operatoragent.ToolResult{}, err
		}
		return operatoragent.ToolResult{Content: tui.RenderProcessSinkDay(view)}, nil
	case "system_doc_get":
		name, err := requiredStringArg(arguments, "name")
		if err != nil {
			return operatoragent.ToolResult{}, err
		}
		doc, err := r.runtime.SystemDocGet(name)
		if err != nil {
			return operatoragent.ToolResult{}, err
		}
		return jsonToolResult(doc)
	case "vault_read":
		path, err := requiredStringArg(arguments, "path")
		if err != nil {
			return operatoragent.ToolResult{}, err
		}
		doc, err := r.runtime.VaultRead(path)
		if err != nil {
			return operatoragent.ToolResult{}, err
		}
		return jsonToolResult(doc)
	case "vault_list":
		entries, err := r.runtime.VaultList(stringArg(arguments, "dir", ""))
		if err != nil {
			return operatoragent.ToolResult{}, err
		}
		return jsonToolResult(entries)
	case "vault_search_text":
		query, err := requiredStringArg(arguments, "query")
		if err != nil {
			return operatoragent.ToolResult{}, err
		}
		hits, err := r.runtime.VaultSearchText(
			query,
			stringArg(arguments, "dir", ""),
			intArg(arguments, "limit", 5),
		)
		if err != nil {
			return operatoragent.ToolResult{}, err
		}
		return jsonToolResult(hits)
	case "vault_backlinks":
		path, err := requiredStringArg(arguments, "path")
		if err != nil {
			return operatoragent.ToolResult{}, err
		}
		hits, err := r.runtime.VaultBacklinks(path, intArg(arguments, "limit", 5))
		if err != nil {
			return operatoragent.ToolResult{}, err
		}
		return jsonToolResult(hits)
	case "doc_classify":
		path, err := requiredStringArg(arguments, "path")
		if err != nil {
			return operatoragent.ToolResult{}, err
		}
		return jsonToolResult(r.runtime.DocClassify(path))
	case "context_pack":
		pack, err := r.runtime.ContextPack(
			stringArg(arguments, "target_path", ""),
			stringArg(arguments, "task", ""),
			intArg(arguments, "limit", 6),
		)
		if err != nil {
			return operatoragent.ToolResult{}, err
		}
		return jsonToolResult(pack)
	case "vault_write_low":
		path, err := requiredStringArg(arguments, "path")
		if err != nil {
			return operatoragent.ToolResult{}, err
		}
		content, ok := rawStringArg(arguments, "content")
		if !ok {
			return operatoragent.ToolResult{}, fmt.Errorf("missing required argument content")
		}
		doc, err := r.runtime.WriteLowRiskNote(path, content, boolArg(arguments, "overwrite"))
		if err != nil {
			return operatoragent.ToolResult{}, err
		}
		return jsonToolResult(map[string]any{
			"status":       "written",
			"path":         doc.Path,
			"doc_class":    doc.DocClass,
			"base_version": doc.BaseVersion,
		})
	case "workspace_list":
		return r.workspaceList(arguments)
	case "workspace_read":
		return r.workspaceRead(arguments)
	case "workspace_write":
		return r.workspaceWrite(arguments)
	case "workspace_edit":
		return r.workspaceEdit(arguments)
	case "shell_exec":
		return r.shellExec(arguments)
	default:
		return operatoragent.ToolResult{}, fmt.Errorf("unknown tool: %s", name)
	}
}

func (r toolRuntime) draftDecision(arguments map[string]any) operatoragent.Decision {
	return operatoragent.Decision{
		DraftID:         stringArg(arguments, "draft_id", ""),
		UseFocusedDraft: boolArg(arguments, "use_focused_draft"),
	}
}

func (r toolRuntime) dayArg(arguments map[string]any) (time.Time, error) {
	raw := strings.TrimSpace(stringArg(arguments, "day", ""))
	if raw == "" {
		now := time.Now()
		if r.session.Now != nil {
			now = r.session.Now()
		}
		return model.NormalizeDay(now), nil
	}
	parsed, err := time.ParseInLocation("2006-01-02", raw, time.Local)
	if err != nil {
		return time.Time{}, fmt.Errorf("invalid day: %s", raw)
	}
	return model.NormalizeDay(parsed), nil
}

func (r toolRuntime) workspaceList(arguments map[string]any) (operatoragent.ToolResult, error) {
	if err := r.requireLocalWorkTools("workspace_list"); err != nil {
		return operatoragent.ToolResult{}, err
	}
	if err := r.requireExplicitLocalIntent("workspace_list"); err != nil {
		return operatoragent.ToolResult{}, err
	}
	absPath, relPath, err := r.resolveWorkspacePath(stringArg(arguments, "path", "."))
	if err != nil {
		return operatoragent.ToolResult{}, err
	}
	entries, err := os.ReadDir(absPath)
	if err != nil {
		return operatoragent.ToolResult{}, err
	}
	type workspaceEntry struct {
		Path string `json:"path"`
		Kind string `json:"kind"`
	}
	out := make([]workspaceEntry, 0, len(entries))
	for _, entry := range entries {
		kind := "file"
		if entry.IsDir() {
			kind = "dir"
		}
		name := filepath.ToSlash(filepath.Join(relPath, entry.Name()))
		if relPath == "." {
			name = entry.Name()
		}
		out = append(out, workspaceEntry{Path: name, Kind: kind})
	}
	return jsonToolResult(out)
}

func (r toolRuntime) workspaceRead(arguments map[string]any) (operatoragent.ToolResult, error) {
	if err := r.requireLocalWorkTools("workspace_read"); err != nil {
		return operatoragent.ToolResult{}, err
	}
	if err := r.requireExplicitLocalIntent("workspace_read"); err != nil {
		return operatoragent.ToolResult{}, err
	}
	path, err := requiredStringArg(arguments, "path")
	if err != nil {
		return operatoragent.ToolResult{}, err
	}
	absPath, relPath, err := r.resolveWorkspacePath(path)
	if err != nil {
		return operatoragent.ToolResult{}, err
	}
	data, err := os.ReadFile(absPath)
	if err != nil {
		return operatoragent.ToolResult{}, err
	}
	return operatoragent.ToolResult{
		Content: fmt.Sprintf("Path: %s\n\n%s", relPath, string(data)),
	}, nil
}

func (r toolRuntime) workspaceWrite(arguments map[string]any) (operatoragent.ToolResult, error) {
	if err := r.requireLocalWorkTools("workspace_write"); err != nil {
		return operatoragent.ToolResult{}, err
	}
	if err := r.requireExplicitLocalIntent("workspace_write"); err != nil {
		return operatoragent.ToolResult{}, err
	}
	path, err := requiredStringArg(arguments, "path")
	if err != nil {
		return operatoragent.ToolResult{}, err
	}
	content, ok := rawStringArg(arguments, "content")
	if !ok {
		return operatoragent.ToolResult{}, fmt.Errorf("missing required argument content")
	}
	absPath, relPath, err := r.resolveWorkspacePath(path)
	if err != nil {
		return operatoragent.ToolResult{}, err
	}
	hash, err := vault.WriteFileAtomic(absPath, []byte(content), workspaceWriteTempSuffix)
	if err != nil {
		return operatoragent.ToolResult{}, err
	}
	return operatoragent.ToolResult{
		Content: fmt.Sprintf("Wrote %s\nsha256: %s", relPath, hash),
	}, nil
}

func (r toolRuntime) workspaceEdit(arguments map[string]any) (operatoragent.ToolResult, error) {
	if err := r.requireLocalWorkTools("workspace_edit"); err != nil {
		return operatoragent.ToolResult{}, err
	}
	if err := r.requireExplicitLocalIntent("workspace_edit"); err != nil {
		return operatoragent.ToolResult{}, err
	}
	path, err := requiredStringArg(arguments, "path")
	if err != nil {
		return operatoragent.ToolResult{}, err
	}
	oldValue, ok := rawStringArg(arguments, "old")
	if !ok || oldValue == "" {
		return operatoragent.ToolResult{}, fmt.Errorf("missing required argument old")
	}
	absPath, relPath, err := r.resolveWorkspacePath(path)
	if err != nil {
		return operatoragent.ToolResult{}, err
	}
	before, err := os.ReadFile(absPath)
	if err != nil {
		return operatoragent.ToolResult{}, err
	}
	newValue, _ := rawStringArg(arguments, "new")
	replaceAll := boolArg(arguments, "replace_all")

	content := string(before)
	count := strings.Count(content, oldValue)
	if count == 0 {
		return operatoragent.ToolResult{}, fmt.Errorf("workspace_edit: old text not found")
	}
	if !replaceAll && count > 1 {
		return operatoragent.ToolResult{}, fmt.Errorf("workspace_edit: old text matched %d times; set replace_all=true or narrow the snippet", count)
	}

	after := strings.Replace(content, oldValue, newValue, 1)
	replaced := 1
	if replaceAll {
		after = strings.ReplaceAll(content, oldValue, newValue)
		replaced = count
	}
	hash, err := vault.WriteFileAtomic(absPath, []byte(after), workspaceWriteTempSuffix)
	if err != nil {
		return operatoragent.ToolResult{}, err
	}
	return operatoragent.ToolResult{
		Content: fmt.Sprintf("Edited %s\nreplacements: %d\nsha256: %s", relPath, replaced, hash),
	}, nil
}

func (r toolRuntime) shellExec(arguments map[string]any) (operatoragent.ToolResult, error) {
	if err := r.requireLocalWorkTools("shell_exec"); err != nil {
		return operatoragent.ToolResult{}, err
	}
	if !shellToolsEnabled() {
		return operatoragent.ToolResult{}, fmt.Errorf("shell_exec is disabled; set LORE_AGENT_ENABLE_SHELL=1 to enable the unsafe local shell profile")
	}
	if err := r.requireExplicitLocalIntent("shell_exec"); err != nil {
		return operatoragent.ToolResult{}, err
	}
	command, err := requiredStringArg(arguments, "command")
	if err != nil {
		return operatoragent.ToolResult{}, err
	}
	timeoutSeconds := intArg(arguments, "timeout_seconds", 30)
	if timeoutSeconds <= 0 {
		timeoutSeconds = 30
	}

	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(timeoutSeconds)*time.Second)
	defer cancel()

	var cmd *exec.Cmd
	if osruntime.GOOS == "windows" {
		cmd = exec.CommandContext(ctx, "powershell.exe", "-NoProfile", "-Command", command)
	} else {
		cmd = exec.CommandContext(ctx, "sh", "-lc", command)
	}
	cmd.Dir = r.runtime.WorkDirPath()
	output, err := cmd.CombinedOutput()

	status := "ok"
	if err != nil {
		status = "error"
	}
	exitCode := 0
	if cmd.ProcessState != nil {
		exitCode = cmd.ProcessState.ExitCode()
	}
	return operatoragent.ToolResult{
		Content: fmt.Sprintf("status: %s\nexit_code: %d\nworkdir: %s\n\n%s", status, exitCode, r.runtime.WorkDirPath(), strings.TrimSpace(string(output))),
	}, err
}

func (r toolRuntime) resolveWorkspacePath(rawPath string) (string, string, error) {
	cleaned := strings.TrimSpace(rawPath)
	if cleaned == "" {
		return "", "", fmt.Errorf("path is required")
	}
	workDir := filepath.Clean(r.runtime.WorkDirPath())
	workRoot, err := resolveExistingPath(workDir)
	if err != nil {
		return "", "", err
	}
	vaultPath := filepath.Clean(r.runtime.VaultRootPath())
	statePath := filepath.Clean(r.runtime.StateDirPath())
	vaultRoot, err := resolveExistingPath(r.runtime.VaultRootPath())
	if err != nil && !os.IsNotExist(err) {
		return "", "", err
	}
	stateRoot, err := resolveExistingPath(r.runtime.StateDirPath())
	if err != nil && !os.IsNotExist(err) {
		return "", "", err
	}

	absPath := cleaned
	if !filepath.IsAbs(absPath) {
		absPath = filepath.Join(workDir, filepath.FromSlash(cleaned))
	}
	absPath = filepath.Clean(absPath)
	if blockedPath(absPath, vaultPath) || blockedPath(absPath, statePath) {
		return "", "", fmt.Errorf("path is inside Lore-managed vault/state and must use Lore tools instead: %s", cleaned)
	}
	realPath, err := resolveTargetPath(absPath)
	if err != nil {
		return "", "", err
	}

	if !isWithinPath(workRoot, realPath) {
		return "", "", fmt.Errorf("path escapes workdir: %s", cleaned)
	}
	if blockedPath(realPath, vaultRoot) || blockedPath(realPath, stateRoot) {
		return "", "", fmt.Errorf("path is inside Lore-managed vault/state and must use Lore tools instead: %s", cleaned)
	}

	relPath, err := filepath.Rel(workRoot, realPath)
	if err != nil {
		return "", "", err
	}
	return realPath, filepath.ToSlash(relPath), nil
}

func (r toolRuntime) requireExplicitLocalIntent(toolName string) error {
	if hasExplicitLocalWorkIntent(r.session.LastInput) {
		return nil
	}
	return fmt.Errorf("%s requires explicit local file/code/run intent in the current request", toolName)
}

func (r toolRuntime) requireLocalWorkTools(toolName string) error {
	if r.localWorkToolsEnabled() {
		return nil
	}
	return fmt.Errorf("%s is unavailable in the default Lore chat profile; restart Lore with --local-exec to expose local workspace tools", toolName)
}

func (r toolRuntime) localWorkToolsEnabled() bool {
	return r.session != nil && r.session.EnableLocalWorkTools
}

func blockedPath(absPath string, blockedRoot string) bool {
	return blockedRoot != "" && isWithinPath(filepath.Clean(blockedRoot), filepath.Clean(absPath))
}

func resolveExistingPath(path string) (string, error) {
	if strings.TrimSpace(path) == "" {
		return "", fmt.Errorf("path is required")
	}
	resolved, err := filepath.EvalSymlinks(filepath.Clean(path))
	if err != nil {
		return "", err
	}
	return filepath.Clean(resolved), nil
}

func resolveTargetPath(path string) (string, error) {
	cleaned := filepath.Clean(path)
	resolved, err := filepath.EvalSymlinks(cleaned)
	if err == nil {
		return filepath.Clean(resolved), nil
	}
	if !os.IsNotExist(err) {
		return "", err
	}

	missing := []string{filepath.Base(cleaned)}
	parent := filepath.Dir(cleaned)
	for {
		resolvedParent, parentErr := filepath.EvalSymlinks(parent)
		if parentErr == nil {
			parts := append([]string{resolvedParent}, missing...)
			return filepath.Clean(filepath.Join(parts...)), nil
		}
		if !os.IsNotExist(parentErr) {
			return "", parentErr
		}

		nextParent := filepath.Dir(parent)
		if nextParent == parent {
			return "", parentErr
		}
		missing = append([]string{filepath.Base(parent)}, missing...)
		parent = nextParent
	}
}

func isWithinPath(root string, candidate string) bool {
	if root == "" || candidate == "" {
		return false
	}
	root = filepath.Clean(root)
	candidate = filepath.Clean(candidate)
	if root == candidate {
		return true
	}
	prefix := root + string(os.PathSeparator)
	return strings.HasPrefix(candidate, prefix)
}

func jsonToolResult(value any) (operatoragent.ToolResult, error) {
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return operatoragent.ToolResult{}, err
	}
	return operatoragent.ToolResult{Content: string(data)}, nil
}

func requiredStringArg(arguments map[string]any, key string) (string, error) {
	value := strings.TrimSpace(stringArg(arguments, key, ""))
	if value == "" {
		return "", fmt.Errorf("missing required argument %s", key)
	}
	return value, nil
}

func stringArg(arguments map[string]any, key string, fallback string) string {
	value, ok := rawStringArg(arguments, key)
	if !ok {
		return fallback
	}
	return strings.TrimSpace(value)
}

func rawStringArg(arguments map[string]any, key string) (string, bool) {
	value, ok := arguments[key]
	if !ok || value == nil {
		return "", false
	}
	if text, ok := value.(string); ok {
		return text, true
	}
	return "", false
}

func boolArg(arguments map[string]any, key string) bool {
	value, ok := arguments[key]
	if !ok || value == nil {
		return false
	}
	if flag, ok := value.(bool); ok {
		return flag
	}
	return false
}

func intArg(arguments map[string]any, key string, fallback int) int {
	value, ok := arguments[key]
	if !ok || value == nil {
		return fallback
	}
	switch typed := value.(type) {
	case float64:
		return int(typed)
	case int:
		return typed
	default:
		return fallback
	}
}

func filterDraftsByStateLocal(drafts []model.Draft, state model.DraftState) []model.Draft {
	out := make([]model.Draft, 0, len(drafts))
	for _, draft := range drafts {
		if draft.State == state {
			out = append(out, draft)
		}
	}
	return out
}

func shellToolsEnabled() bool {
	value := strings.TrimSpace(os.Getenv("LORE_AGENT_ENABLE_SHELL"))
	return value == "1" || strings.EqualFold(value, "true") || strings.EqualFold(value, "yes")
}

func hasExplicitLocalWorkIntent(input string) bool {
	value := normalizeIntentInput(input)
	if value == "" {
		return false
	}

	for _, phrase := range []string{
		"workspace",
		"workdir",
		"local file",
		"local files",
		"local repo",
		"local repository",
		"repository",
		"repo",
		"codebase",
		"source file",
		"project file",
		"project directory",
		"write code",
		"edit code",
		"modify code",
		"change code",
		"source code",
		"implement",
		"patch",
		"refactor",
		"debug",
		"fix bug",
		"fix the bug",
		"run test",
		"run tests",
		"execute command",
		"run command",
		"shell command",
		"terminal command",
		"open file",
		"read file",
		"list files",
		"create file",
		"write file",
		"edit file",
		"update file",
		"modify file",
		"terminal",
		"shell",
		"powershell",
		"bash",
		"git",
		"compile",
		"build",
		"代码",
		"本地文件",
		"本地代码",
		"工作区",
		"本地仓库",
		"仓库",
		"代码库",
		"项目目录",
		"工程目录",
		"打开文件",
		"读取文件",
		"查看文件",
		"列出文件",
		"创建文件",
		"写文件",
		"改文件",
		"编辑文件",
		"修改文件",
		"写代码",
		"改代码",
		"编辑代码",
		"修改代码",
		"修bug",
		"修复bug",
		"调试",
		"跑测试",
		"运行测试",
		"执行命令",
		"运行命令",
		"终端",
		"命令行",
		"编译",
		"构建",
	} {
		if strings.Contains(value, phrase) {
			return true
		}
	}

	for _, hint := range []string{
		".go",
		".py",
		".ts",
		".tsx",
		".js",
		".jsx",
		".json",
		".yaml",
		".yml",
		".toml",
		".sh",
		".ps1",
		".sql",
	} {
		if strings.Contains(value, hint) {
			return true
		}
	}

	return false
}

func normalizeIntentInput(value string) string {
	replacer := strings.NewReplacer(
		"\r\n", " ",
		"\n", " ",
		"\t", " ",
		"，", ",",
		"。", ".",
		"：", ":",
		"（", "(",
		"）", ")",
	)
	return strings.ToLower(strings.TrimSpace(replacer.Replace(value)))
}
