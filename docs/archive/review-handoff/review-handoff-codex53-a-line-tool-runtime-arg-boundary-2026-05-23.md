# Review Handoff: A-Line Tool Runtime Argument Boundary

Date: 2026-05-23

## Scope

- `internal/console/tool_runtime.go`
- `internal/console/tool_runtime_test.go`
- `scripts/release-gate.ps1`
- `README.md`

## Summary

Centralizes high-risk local/write tool argument parsing inside console tool runtime without doing a larger ToolRegistry rewrite.

Covered tools:

- `draft_supersede`
- `vault_write_low`
- `workspace_list`
- `workspace_read`
- `workspace_write`
- `workspace_edit`
- `shell_exec`

Behavior preserved:

- MCP surface remains read + proposal intake only.
- `workspace_*`, `git_*`, and `shell_exec` still require local-exec mode.
- `shell_exec` still only queues confirmation; it does not execute immediately.
- `workspace_write` still preserves whitespace and allows empty string content when the `content` key exists.
- `vault_write_low` still allows empty string content when the `content` key exists.
- `workspace_edit` still rejects missing/empty `old`, allows missing `new` as empty string, rejects not-found and ambiguous multi-match without `replace_all`.
- Path safety remains in `resolveWorkspacePath`, including vault/state block and symlink/workdir-escape checks.

## Tests Added

- `TestToolRuntimeHighRiskArgsPreserveWriteSemantics`
- `TestToolRuntimeHighRiskArgsPreserveEditAndShellSemantics`

These tests lock the main parser edge cases: missing vs empty content, draft supersede required content/reason, workspace edit old/new semantics, and shell timeout normalization.

## Release Gate

The console gate now includes the new high-risk argument boundary tests. README release-gate wording was updated to mention this targeted coverage.

## Validation

```powershell
go test ./internal/console -run "TestToolRuntime" -count=1 -v
go test ./internal/operatoragent ./internal/console -count=1
powershell.exe -NoProfile -ExecutionPolicy Bypass -File .\scripts\release-gate.ps1 -SkipDiffCheck
```
