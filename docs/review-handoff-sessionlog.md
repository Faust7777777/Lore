# Session Transcript/Resume Review Handoff

## Scope

This change adds workspace-local session transcripts and explicit resume support for Lore console/TUI sessions.

Changed areas:
- `internal/sessionlog/`: append-only JSONL recorder, index, resume loader, tests.
- `internal/console/session.go`: optional `TranscriptRecorder` hook; records user/assistant/tool/working-set/error/local-command events.
- `internal/cli/cli.go`: `--resume` and `--resume-id` wiring for `console` and `tui`; new sessions always get a transcript.
- `internal/tui/*`: snapshot carries session id/path; status panel shows the current transcript location.

## Frozen Semantics To Verify

- Default cross-session memory is off. New `console`/`tui` runs create a transcript, but do not load old transcripts unless `--resume` or `--resume-id` is passed.
- `--resume-id <id>` loads that workspace session from `<workdir>/state/sessions/<id>.jsonl`.
- `--resume` lists the recent 20 workspace sessions and resumes the selected id.
- Resume restores only conversation history and the latest working set snapshot. It does not replay tool calls or re-run side effects.
- Transcript storage is workspace-local under `<workdir>/state/sessions/`, not inside the vault content tree.
- Corrupted JSONL lines are skipped during load and reported as snapshot warnings.
- Tool call arguments and errors are truncated inside `sessionlog`, so callers do not need to sanitize large fields.

## Review Focus

- Confirm `configureSessionRecorder` is called after runtime/session creation in both `RunConsoleCommand` and `RunTUICommand`.
- Confirm `sessionlog.Start` is used for fresh sessions and `sessionlog.Resume` is used only when resume input is explicit.
- Confirm `RecordToolTrace` writes a stable truncated JSON envelope for oversized arguments: `{ "truncated": true, "preview": ... }`.
- Confirm `Load` tolerates bad JSONL lines and still restores valid user/assistant turns and the last working set.
- Confirm TUI only displays transcript metadata and does not own transcript state or persistence.

## Known Non-P0 Limits

- `sessionlog.Search` currently searches session id/title only. Full transcript search is a future extension.
- `appendAndIndex` reloads the transcript to refresh the index after each event. This is simple and acceptable for P0, but can be optimized later.
- `--resume` selection is CLI-level stdin/stdout interaction, not a Bubble Tea picker yet.

## Verification Run

Focused packages:

```powershell
.\.tools\go\bin\go.exe test ./internal/sessionlog ./internal/tui ./internal/cli ./cmd/obsidian-harness ./internal/console -count=1
```

Full package test should exclude the known ignored probe package `obsidian-harness/tmp`.