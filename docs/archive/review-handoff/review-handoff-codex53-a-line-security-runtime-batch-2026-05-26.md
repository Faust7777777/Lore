# Review Handoff: A-Line Security Runtime Batch

Date: 2026-05-26

## FINAL Findings Covered

This handoff is for submitting the current A-line batch as one review unit. It
exists because some shared files (`README.md`, `scripts/release-gate.ps1`, and
`internal/operatoragent/model*.go`) contain hunks from multiple A-line slices.
Do not use the parser-only handoff as the sole handoff if these mixed hunks are
submitted together.

Covered findings / runtime follow-ups:

- `S-1` / `Symlink path traversal` / `P0`
- `S-2` / `runtimeDocs prompt isolation` / `P1`
- `S-3` / `SessionLog lock-order comment` / `P1`
- A-line runtime follow-up: loop-parser fail-soft safety for non-envelope JSON
  answers, including MCP config JSON examples.

## Modified Files

S-1 vault symlink boundary:

- `internal/vault/query.go`
- `internal/vault/query_test.go`
- `internal/orchestrator/readapi.go`
- `internal/orchestrator/readapi_test.go`
- `internal/orchestrator/harness.go`
- `internal/orchestrator/harness_test.go`

S-2 runtime docs prompt isolation:

- `internal/operatoragent/model.go`
- `internal/operatoragent/model_test.go`

S-3 sessionlog lock-order comments:

- `internal/sessionlog/writer.go`
- `internal/sessionlog/index.go`

Parser fail-soft runtime safety:

- `internal/operatoragent/model.go`
- `internal/operatoragent/model_test.go`
- `internal/console/session_usage_e2e_test.go`
- `cmd/obsidian-harness/main_test.go`

Shared gate/docs files touched by multiple slices:

- `scripts/release-gate.ps1`
- `README.md`

Per-slice handoffs:

- `docs/archive/review-handoff/review-handoff-codex53-a-line-s1-vault-symlink-boundary-2026-05-25.md`
- `docs/archive/review-handoff/review-handoff-codex53-a-line-s2-runtime-doc-isolation-2026-05-25.md`
- `docs/archive/review-handoff/review-handoff-codex53-a-line-s3-sessionlog-lock-order-2026-05-25.md`
- `docs/archive/review-handoff/review-handoff-codex53-a-line-loop-parser-fail-soft-2026-05-25.md`

Batch handoff:

- `docs/archive/review-handoff/review-handoff-codex53-a-line-security-runtime-batch-2026-05-26.md`

## Implementation Notes

### S-1 Vault Symlink Boundary

- Added vault-relative safe resolvers for existing reads and new writes.
- Existing paths are lexically contained, then symlink-resolved and checked
  against the symlink-resolved vault root.
- New write targets validate the nearest existing parent, so non-existent final
  files do not use unsafe `EvalSymlinks(final)` behavior.
- Vault read/list/walk/search/backlink paths now go through the safe resolver.
- `ApplyDraft` and `WriteLowRiskNote` now write through vault-relative helpers.

### S-2 Runtime Docs Prompt Isolation

- Runtime docs are no longer raw appended text.
- Runtime docs render as XML-like bounded blocks with explicit trust labeling:
  vault/user-authored context, not runtime instructions.
- Runtime doc content is XML-escaped to prevent injected tags or delimiters from
  becoming prompt structure.
- Prompt doc excerpt truncation is UTF-8 safe.

### S-3 SessionLog Lock Order

- Added code comments documenting the lock order:
  `Recorder.mu -> indexLocks[root]`.
- No runtime logic changed in this slice.

### Parser Fail-Soft Safety

- `parseLoopResponse` still accepts legal loop envelopes and legacy action JSON.
- Non-envelope JSON such as `{"mcpServers":{...}}` becomes final user output.
- Markdown/prose containing JSON examples becomes final user output instead of
  extracting examples as protocol.
- Control-like JSON containing `tool`, `arguments`, or `action` without a legal
  loop `type` remains a hard parser error and never executes a tool.

## Load-Bearing Tests

S-1:

- `TestReadRelativeWithHashRejectsSymlinkFileOutsideRoot`
- `TestWalkListSearchAndBacklinksSkipFileSymlinkOutsideRoot`
- `TestWalkListSearchAndBacklinksSkipDirectorySymlinkOutsideRoot`
- `TestVaultReadRejectsSymlinkFileOutsideRoot`
- `TestVaultReadRejectsParentSymlinkOutsideRoot`
- `TestWriteLowRiskNoteRejectsParentSymlinkOutsideRoot`
- `TestApplyDraftRejectsParentSymlinkOutsideRoot`

S-2:

- `TestLoopSystemPromptIsolatesRuntimeDocsAsUntrustedContext`
- `TestPromptDocExcerptTruncatesUTF8Safely`
- Existing prompt version / runtime doc inclusion tests updated to assert the
  trust boundary.

S-3:

- No new test by design; existing sessionlog index tests remain load-bearing:
  `TestConcurrentRecordersPreserveIndexEntries`,
  `TestRecorderWritesIndexAndRestoresSnapshot`,
  `TestResumeRefreshesIndexTurnCount`.

Parser fail-soft:

- `TestModelAgentRespondTreatsNonEnvelopeJSONAsFinal`
- `TestModelAgentRespondTreatsMarkdownJSONExampleAsFinal`
- `TestModelAgentRespondRejectsControlLikeJSONWithoutType`
- `TestEndToEndUsagePipelineSuccessAndFailureTurns`
- `TestRunUsageCLIReadsFailedTurnUsageEndToEnd`

Gate coverage:

- `scripts/release-gate.ps1` now includes S-1 vault/orchestrator symlink gates,
  S-2 runtime-doc isolation gates, and parser fail-soft safety gates.

## Verification Already Run

- `go test ./internal/operatoragent -count=1`
- `go test ./internal/console ./cmd/obsidian-harness -count=1 -run "Test(EndToEndUsagePipelineSuccessAndFailureTurns|RunUsageCLIReadsFailedTurnUsageEndToEnd)$"`
- `go test ./internal/operatoragent ./internal/console ./cmd/obsidian-harness -count=1`
- `go test ./internal/vault ./internal/orchestrator ./internal/operatoragent ./internal/sessionlog -count=1`
- `powershell.exe -NoProfile -ExecutionPolicy Bypass -File .\scripts\release-gate.ps1 -SkipDiffCheck`
- `powershell.exe -NoProfile -ExecutionPolicy Bypass -File .\scripts\verify.ps1`
- `git diff --check`

Notes:

- File-symlink tests skip on the current Windows account because it lacks file
  symlink privilege.
- Directory junction tests run on Windows via `mklink /J` fallback and passed.
- One full `verify.ps1` run hit a transient smoke checkpoint materialization
  failure; a direct smoke rerun and the next full `verify.ps1` run both passed.

## Explicit Boundaries

This A-line batch did not change:

- MCP exposed write/apply/shell surface.
- Persona candidate state machine or persona draft governance path.
- TUI product behavior, presentation, approval pane semantics, or Cost/Usage UI.
- SDK behavior.
- Generic non-vault `WriteFileAtomic` semantics.

## Staging Guidance

Two valid submission options remain:

1. Submit this whole A-line batch using this batch handoff.
2. Split commits by slice. If doing that, stage shared file hunks carefully with
   `git add -p` so parser-only commits do not include S-1/S-2 release-gate or
   README hunks.

Do not submit parser-only using only the parser handoff while also staging S-1,
S-2, or S-3 hunks.
