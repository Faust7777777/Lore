# Session Resume Smoke

This smoke verifies the frozen transcript semantics:

- Fresh `console`/`tui` runs record transcripts under `<workdir>/state/sessions/`.
- Fresh runs do not load previous transcripts by default.
- `--resume-id <id>` explicitly restores the selected session history and appends to the same JSONL file.
- `--resume` lists the recent 20 workspace-local sessions for selection.

## Prerequisites

Use a bootstrapped Lore workdir and configured model environment.

```powershell
$wd = "tmp/chat-demo"
.\.tools\go\bin\go.exe test ./cmd/obsidian-harness -run TestRunConsoleStartsFreshSessionWithoutResume -count=1
```

## Manual Smoke

Start a fresh one-shot session:

```powershell
.\.tools\go\bin\go.exe run ./cmd/obsidian-harness console --workdir $wd --once "show current status"
```

List transcripts:

```powershell
Get-ChildItem "$wd\state\sessions" -Filter *.jsonl | Sort-Object LastWriteTime -Descending | Select-Object -First 5 Name,LastWriteTime
```

Run the same command again without resume. Expected: a second `.jsonl` file appears; the first one is not appended.

```powershell
.\.tools\go\bin\go.exe run ./cmd/obsidian-harness console --workdir $wd --once "show current status"
```

Resume a specific session. Expected: the selected `.jsonl` gains another `user_message`/`assistant_message` pair.

```powershell
$sid = (Get-ChildItem "$wd\state\sessions" -Filter *.jsonl | Sort-Object LastWriteTime -Descending | Select-Object -First 1).BaseName
.\.tools\go\bin\go.exe run ./cmd/obsidian-harness console --workdir $wd --resume-id $sid --once "show current status"
```

Interactive selection:

```powershell
.\.tools\go\bin\go.exe run ./cmd/obsidian-harness console --workdir $wd --resume
```

## Review Notes For 5.3 Codex

Focus on CLI semantics, not TUI rendering:

- No old transcript is read unless `--resume` or `--resume-id` is present.
- `--resume-id` appends to the existing JSONL instead of creating a new one.
- Loading a session restores history and the latest working set only; tool calls are not replayed.
- Transcript files stay under workspace `state/sessions`, not the vault tree.