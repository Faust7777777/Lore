# 5.3 Codex Review Handoff: Explicit Sessions CLI

## Scope

This change exposes explicit session transcript inspection through a new top-level `sessions` command.

Changed files:
- `internal/cli/cli.go`
- `cmd/obsidian-harness/main_test.go`

## Commands

- `lore sessions list --workdir <dir> --limit 20`
- `lore sessions search --workdir <dir> --limit 20 <query>`
- `lore sessions show --workdir <dir> <session-id>`

## Contract

- This is explicit transcript inspection only.
- Fresh `console`/`tui` sessions still do not auto-load old transcripts.
- `sessions search` reads only workspace-local `<workdir>/state/sessions` transcripts through `sessionlog.Search`.
- List/search output is a compact TSV-like list: session id, updated time, turn count, title.
- Show output is a read-only summary of metadata, working set, and conversation turns.
- No tool calls are replayed and no session is resumed by these commands.

## Review Focus

- Confirm the new top-level `sessions` command does not alter `console` or `tui` startup paths.
- Confirm `sessions search` requires an explicit query.
- Confirm `sessions list/search` use `sessionLogRoot(runtime)`, not the vault tree.
- Confirm no TUI files are part of this change.

## Verification

```powershell
.\.tools\go\bin\go.exe test ./internal/cli ./internal/sessionlog ./cmd/obsidian-harness -count=1
```