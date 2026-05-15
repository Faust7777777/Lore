# 5.3 Codex Review Handoff: Shell Confirm Boundary

## Scope

This change adds regression coverage for the frozen local-tool safety boundary.

Changed file:
- `internal/console/tool_runtime_test.go`

## Frozen Behavior

- `shell_exec` is the only tool that enters a pending confirmation state.
- `shell_exec` is exposed only when both conditions are true:
  - session `EnableLocalWorkTools` is true (`--local-exec`)
  - `LORE_AGENT_ENABLE_SHELL=1`
- Non-shell tools do not use keyword gating or confirmation prompts.
- `workspace_write` still requires `--local-exec`, but once local tools are enabled it executes immediately subject to runtime path boundaries.
- `vault_write_low` is governed by vault/runtime protections, not by natural-language keyword gates.

## New Coverage

`TestToolRuntimeWorkspaceWriteRunsWithoutConfirmation` verifies that:
- `workspace_write` writes the file immediately under local-exec mode.
- no `PendingShellCommand` is set for a non-shell tool.
- the result does not contain a pending confirmation prompt.

Existing tests already cover:
- `shell_exec` queues confirmation instead of executing immediately.
- `shell_exec` is hidden/disabled unless `LORE_AGENT_ENABLE_SHELL=1`.
- git tools run without keyword gating under local-exec.
- `vault_write_low` calls runtime without keyword gating.

## Review Focus

- Confirm no prompt-level or runtime keyword whitelist was introduced.
- Confirm only shell uses `PendingShellCommand`.
- Confirm this is a test-only freeze; production behavior is unchanged.
- Confirm current Opus TUI changes are not part of this handoff.

## Verification

```powershell
.\.tools\go\bin\go.exe test ./internal/console ./internal/operatoragent -count=1
```