# Obsidian Harness Opus Review Packet

## Review Target

- Repo: `C:\Users\15892\Desktop\obsidian-harness`
- Branch: `main`
- Commit to review: `eb6f7ab feat: stabilize codex ingest and gpt5 transport`

Please review this commit as a code review, not as a feature brainstorm.
Primary output wanted:

1. Findings first, ordered by severity.
2. Each finding should include file/line references.
3. Focus on bugs, regressions, protocol mismatches, bad assumptions, and missing tests.
4. Keep summary short unless needed.

## Product Context

This project is a Go-based Obsidian harness. Two P0 chains matter here:

1. Managed-doc chain:
   `document change -> draft -> review -> apply`
2. External-agent process-sink chain:
   `Codex JSONL -> checkpoint summaries -> daily report`

The commit under review mainly hardens the second chain and the model transport used by both the operator agent and process-sink summarizer.

## Why This Commit Exists

There were two concrete failures before this change:

1. `gpt-5.4` against the configured OpenAI-compatible endpoint was not reliable through `chat/completions`.
2. Codex JSONL ingest had identity and placeholder-window edge cases:
   - transcript `session_meta.id` was not reliably winning over filename-derived session IDs
   - initial sync could incorrectly generate historical idle placeholders
   - daemon loop could watch vault changes but not also sync a Codex JSONL each cycle

## High-Level Changes

### 1. GPT-5 transport switched to Responses API

Relevant files:

- `internal/llm/openai/client.go`
- `internal/llm/openai/client_test.go`

Key functions:

- `ChatCompletion` at `internal/llm/openai/client.go:146`
- `responsesOnce` at `internal/llm/openai/client.go:242`
- `usesResponsesAPI` at `internal/llm/openai/client.go:439`
- `buildResponsesPayload` at `internal/llm/openai/client.go:444`
- `extractResponsesText` at `internal/llm/openai/client.go:485`

What changed:

- `gpt-5*` models now go to `/responses` instead of `/chat/completions`.
- request mapping:
  - `system` / `developer` messages -> `instructions`
  - `user` / `assistant` messages -> `input` message items
- response parsing now reads `output[].content[].type == "output_text"` and usage from `input_tokens` / `output_tokens`
- existing non-GPT-5 path still uses `chat/completions`
- retry behavior currently covers:
  - HTTP `5xx`
  - `io.EOF`
  - `net.Error`

Important implementation choice:

- This transport switch was intentionally aligned to local open-source reference behavior, especially `openclaw`, where `gpt-5.4` is modeled as `openai-responses`, not `openai-completions`.

Reference files consulted while implementing:

- `C:\Users\15892\openclaw\src\agents\model-compat.test.ts`
- `C:\Users\15892\openclaw\src\agents\model-forward-compat.ts`
- `C:\Users\15892\openclaw\src\infra\retry.ts`

### 2. Codex JSONL identity binding fixed

Relevant files:

- `internal/adapter/codexjsonl/parser.go`
- `internal/adapter/codexjsonl/parser_test.go`
- `internal/app/import_codex.go`

Key functions:

- `applyCodexJSONLIdentity` at `internal/app/import_codex.go:52`
- `shouldBindSessionMeta` at `internal/adapter/codexjsonl/parser.go:402`
- `shouldBindAgentMeta` at `internal/adapter/codexjsonl/parser.go:414`

What changed:

- parser now allows the first meaningful `session_meta` to override filename-derived session IDs
- but it does not allow later `session_meta` records to keep flipping the active session
- intended result:
  - `session_meta.id` should win over `probe.jsonl -> probe`
  - later parent-thread style metadata should not overwrite the active rollout session

### 3. Initial sync no longer backfills historical idle windows

Relevant files:

- `internal/app/sync_codex.go`
- `internal/app/import_codex_test.go`

Key functions:

- `SyncCodexJSONL` at `internal/app/sync_codex.go:24`
- `codexJSONLTrailingPlaceholders` at `internal/app/sync_codex.go:225`

What changed:

- trailing placeholder generation now only happens when a saved cursor already exists
- first attach of an old transcript should ingest only material windows actually present in the file

### 4. Daemon can sync Codex JSONL during each cycle

Relevant files:

- `cmd/obsidian-harness/main.go`
- `cmd/obsidian-harness/main_test.go`
- `internal/app/daemon.go`

Key functions:

- `RunVaultDaemon` at `internal/app/daemon.go:99`
- `writeDaemonCodexSummary` at `internal/app/daemon.go:164`

What changed:

- `daemon run` accepts:
  - `--codex-jsonl`
  - `--agent`
  - `--session`
  - `--window`
  - `--skip-rollup`
- each daemon cycle can now:
  - scan vault changes
  - then sync Codex JSONL
  - then print sync summary

## Files Worth Reviewing Closely

Most important:

- `internal/llm/openai/client.go`
- `internal/adapter/codexjsonl/parser.go`
- `internal/app/sync_codex.go`
- `internal/app/daemon.go`

Supporting tests:

- `internal/llm/openai/client_test.go`
- `internal/adapter/codexjsonl/parser_test.go`
- `internal/app/import_codex_test.go`
- `cmd/obsidian-harness/main_test.go`

## What Was Actually Verified

### Local automated verification

Ran successfully:

- `go test ./...`
- `go build -o bin/obsidian-harness.exe ./cmd/obsidian-harness`

### Real provider verification

Verified against the locally configured OpenAI-compatible endpoint using `gpt-5.4`.

Observed facts:

1. `/responses` works for this provider and returns OpenAI-style response objects with:
   - `output[].content[].type = output_text`
   - usage fields `input_tokens` / `output_tokens`
2. `import-codex-jsonl` now succeeds end-to-end.
3. `sync-codex-jsonl` now succeeds end-to-end.
4. `daemon run --once --codex-jsonl ...` now succeeds end-to-end.
5. resulting checkpoints are `materialized`, not mistakenly overwritten to `placeholder`.
6. resulting session ID is `session-daemon` from transcript meta, not filename-derived fallback.

## Suggested Review Questions

Please scrutinize these areas especially:

1. Is `usesResponsesAPI(model)` being `strings.HasPrefix(model, "gpt-5")` too broad or too narrow?
2. Is the responses payload mapping correct for this project’s actual usage of system/developer/user/assistant history?
3. Could the retry behavior produce duplicate side effects or mask a bad request as a transient failure?
4. Is `session_meta` binding now correct for:
   - fresh imports
   - resumed tail syncs
   - files with multiple `session_meta` records
   - filename/session mismatches
5. Does gating placeholder generation on `savedCursorRaw != ""` fully prevent false backfill without breaking valid idle-slot generation on resumed sessions?
6. Does daemon integration handle partial failure boundaries correctly?
   - vault scan succeeds
   - Codex sync fails
   - one cycle partially completes
7. Any regression risk for non-GPT-5 models that still use `/chat/completions`?
8. Any missing tests around:
   - `/responses` payload edge cases
   - assistant history in responses input
   - empty or malformed output arrays
   - retries on transport interruption

## What I Want Back From Review

Please answer in this shape:

1. Findings
2. Open questions / assumptions
3. Short change-summary only if needed

If there are no findings, please say that explicitly and mention any residual risk or test gap.
