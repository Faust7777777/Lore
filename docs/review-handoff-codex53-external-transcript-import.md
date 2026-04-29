# Review Handoff: Lore External Transcript Import

## Scope

This adds a one-shot Lore external transcript JSONL import path for process-sink ingestion.

It is intentionally not MCP, not note-writing, and not a governed draft/apply path. It only turns external chat transcripts into process-sink checkpoints and daily reports.

## Files Changed

- `internal/adapter/externaljsonl/parser.go`
- `internal/adapter/externaljsonl/parser_test.go`
- `internal/app/import_external.go`
- `internal/app/import_external_test.go`
- `internal/cli/cli.go`
- `cmd/obsidian-harness/main_test.go`
- `docs/adr-lore-v1-architecture.md`
- `docs/compact-handoff-lore-v1.md`
- `docs/plans/2026-04-28-external-agent-governed-intake.md`

## Input Format

The importer accepts NDJSON in Lore's external transcript JSONL schema. It is not an automatic adapter for arbitrary third-party JSONL formats.

Session metadata:

```json
{"type":"session_meta","agent_id":"Claude Code","session_id":"class-1"}
```

Message events:

```json
{"timestamp":"2026-04-22T09:05:00+08:00","role":"user","text":"summarize the class"}
{"timestamp":"2026-04-22T09:35:00+08:00","role":"assistant","phase":"final","text":"class summary ready"}
```

Allowed roles:

- `user`
- `assistant`
- `agent` and `model` are normalized to `assistant`

Unsupported roles return an error instead of being silently ingested.

Identity binding:

- The first non-empty `agent_id` wins.
- The first non-empty `session_id` wins.
- Later `session_meta` or event-level metadata cannot flip an already-bound transcript identity.
- CLI/app overrides through `--agent` and `--session` still take precedence after parsing.

## CLI

```powershell
lore import-external-jsonl --workdir <workdir> --input <path> [--agent <id>] [--session <id>] [--window 30m] [--skip-rollup]
```

The command prints:

```text
External transcript JSONL imported
- agent: ...
- session: ...
- checkpoints: ...
- daily reports: ...
```

## Semantics

- Uses the existing 30-minute process-sink window pipeline.
- Reuses existing checkpoint summarizer and daily rollup behavior.
- Does not create formal notes.
- Does not mutate persona/system/progress.
- Does not create markdown-note proposals.
- Does not expose any new MCP tools.

## Non-Goals

- No external transcript attach/sync mode yet.
- No tail cursor for Lore external transcript JSONL yet.
- No automatic persona/profile update extraction.
- No automatic governed note proposal generation.

## Tests Added

- `TestLoadFileParsesGenericExternalTranscript`
- `TestLoadFileRejectsUnsupportedRole`
- `TestLoadFileKeepsFirstSessionMetaIdentity`
- `TestRuntimeImportExternalTranscriptJSONLWritesCheckpointsAndRollup`
- `TestRuntimeImportExternalTranscriptJSONLRequiresInput`
- `TestRunImportExternalJSONL`
- `TestRunImportExternalJSONLMissingInput`

## Verification

Run:

```powershell
.\.tools\go\bin\go.exe test ./internal/adapter/externaljsonl -count=1 -v
.\.tools\go\bin\go.exe test ./internal/app -run TestRuntimeImportExternalTranscriptJSONL -count=1 -v
.\.tools\go\bin\go.exe test ./cmd/obsidian-harness -run TestRunImportExternalJSONL -count=1 -v
```

Expected: all pass.

## Review Focus

- Confirm this remains process-sink only and does not become a write/proposal/apply path.
- Confirm unsupported roles are rejected rather than silently ingested.
- Confirm empty app-level input returns `empty input path` instead of opening the current directory.
- Confirm agent/session metadata is preserved and sanitized through the existing transcript pipeline.
- Confirm later metadata cannot flip a transcript identity.
- Confirm no MCP boundary drift.
