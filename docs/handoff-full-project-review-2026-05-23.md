# Full Project Review Handoff - 2026-05-23

本页是 2026-05-23 对 `obsidian-harness` 的接续开发交接文档。它综合了本地代码审阅、4 个并行子代理审阅、现有 repo 文档、以及 `C:\Users\15892\Desktop\docs\hermes-workspace` 下架构师审阅文档。

## 0. Review Baseline

| Item | State |
| --- | --- |
| Repo | `C:\Users\15892\Desktop\obsidian-harness` |
| Branch | `main...origin/main [ahead 24]` |
| Reviewed HEAD | `790baac docs: thread show --json into pipeline overview` |
| Worktree before this doc | clean |
| External architect docs | `C:\Users\15892\Desktop\docs\hermes-workspace` |
| Subagent reviews used | core architecture/store, persona/CLI, console/operatoragent/TUI/SDK, external docs integration |

Verification run during this review:

```powershell
go test ./... -count=1
Push-Location .\sdk\go\lore; go test ./... -count=1; Pop-Location
.\scripts\release-gate.ps1 -SkipDiffCheck
.\scripts\release-gate.ps1 -PersonaAcceptance -SkipDiffCheck
```

All passed. `TestSDKEndToEndWithLoreMCP` remains intentionally skipped unless `LORE_SDK_E2E=1`.

## 1. Executive Summary

The project is no longer a thin prototype. The current codebase has a working governed knowledge-ops harness with:

- Local runtime and daemon smoke paths.
- SQLite-backed state store with JSON migration.
- Governed draft review/apply flow for managed vault writes.
- Read/proposal-only MCP boundary.
- CLI and TUI shells.
- Operator-agent tool loop with visible task-turn steps.
- Sessionlog persistence for task-turn events.
- Go SDK read-only contract.
- LLM persona memory candidate pipeline from extraction to CLI review to governed `persona_update` draft.

The highest-value next work is not more persona feature growth. It is hardening state-machine consistency, CLI contract correctness, turn cancellation, and operator-facing UX around pending work.

Immediate issue before more user-facing demos: persona CLI docs advertise ID-first flag placement such as `lore persona candidates show <id> --json`, but implementation uses Go `flag.FlagSet.Parse`, where flags after the first positional arg are not parsed. The working shape is currently `lore persona candidates show --json <id>`. Fix the parser or update docs/tests before asking humans to follow the docs.

## 2. Current Architecture Map

| Layer | Main locations | Current role | Status |
| --- | --- | --- | --- |
| CLI entrypoints | `cmd/lore`, `cmd/obsidian-harness`, `internal/cli` | User-facing command surface, smoke entrypoint, TUI launch, persona commands | Broad but too large; behavior well-tested |
| App facade | `internal/app` | Opens runtime, wires config/store/harness/summarizers/persona extractor, owns higher-level workflows | Stable but several files are oversized |
| Governance core | `internal/orchestrator` | Draft proposal/review/apply, vault writes, audit, read API | Core contract works; apply consistency needs hardening |
| Store | `internal/store`, `internal/store/sqlitestore`, `jsonstore`, `memory` | State persistence and backend parity | Persona CAS strong; Finding transitions weak |
| Persona mining | `internal/persona`, app persona candidate helpers, CLI persona commands | LLM extraction, candidate lifecycle, CLI review/recover, summary/errors JSON | Functionally complete; flag-order contract mismatch |
| Console shell | `internal/console` | Console turn handling, tool runtime, session logging, persona async extraction | Stable; cancellation/tool-result budget missing |
| Operator agent | `internal/operatoragent` | Model/tool loop, TurnStep trace, LLM env config | Tested; hardcoded prompt and `context.Background()` remain |
| TUI | `internal/tui`, `internal/cli/tui_workbench.go` | Interactive shell and presentation | Task-step visibility works; Bubble Tea `View()` side effects remain |
| Sessionlog | `internal/sessionlog` | JSONL session events, task-turn records, history | Improved guardrails; index/write scalability still a concern |
| MCP/tools | `internal/mcp`, `internal/tools` | External read/proposal-only tool surface | Contract snapshots pass |
| SDK | `sdk/go/lore` | Go client for MCP stdio transport | Contract tests pass; frame-size limit missing |
| Infra | `internal/config`, `internal/vault`, `internal/runtime` | Config loading, vault FS operations, event broker/auditor | Thin and usable; validation/durability gaps remain |

## 3. Completed Capability Inventory

### A / Gate / TUI Visibility

The visible task-turn path is gated and passing:

- `TestRunTUIOnceShowsResolveReadFinalTaskVisibility` is active.
- `release-gate.ps1 -SkipDiffCheck` passes.
- `release-gate.ps1 -PersonaAcceptance -SkipDiffCheck` passes.
- Operator-agent turn steps are persisted through console/sessionlog and rendered through TUI tests.

Current TUI state:

- Approval state rendering and task-step rendering are covered.
- Observation excerpts are rune-safe for CJK/emoji.
- Non-interactive and interactive paths share truncation behavior.
- TUI still has no first-class persona candidate review panel. Persona review is CLI-first.

### B / Persona Memory Candidate Pipeline

Persona memory is functionally complete through the current B-line scope.

Implemented flow:

1. Console user turn completes normally.
2. Async persona extractor runs fire-and-forget using `LORE_LLM_*`.
3. Extractor emits only candidates with validated `evidence_quote` substring.
4. Candidate store persists/dedups across SQLite, JSON, and memory backends.
5. Operator reviews candidates with `lore persona candidates list/show`.
6. Operator promotes via `lore persona candidates draft`.
7. Promotion reuses `Harness.ProposePersonaUpdate`.
8. Result is a governed pending-review `persona_update` draft.
9. No auto-approve and no auto-apply.
10. Partial/orphan lifecycle can be recovered with `recover`.
11. Observability is exposed through `lore persona errors`, `lore persona summary`, and `lore usage`.

Important invariants:

- The LLM never writes persona docs directly.
- MCP does not expose direct persona writes.
- Candidate `DedupKey` is normalized `field|proposed_value|evidence_quote`.
- `State=drafted` with empty `DraftID` is a partial-orphan scar, not a normal completed state.
- Store-level CAS protects first promotion and retry promotion from duplicate drafts.
- Retry is allowed only for rejected/expired/superseded linked drafts.
- Parser warnings and extraction failures are observable in workdir logs.
- `UsageRecord.Purpose` separates `chat`, `process_sink`, and `persona_extract`.

Current CLI surfaces:

```powershell
lore persona candidates list --workdir <p> [--state open|drafted|dismissed] [--limit n] [--json]
lore persona candidates show --workdir <p> [--json] <id>
lore persona candidates dismiss --workdir <p> <id>
lore persona candidates draft --workdir <p> [--retry-rejected] <id>
lore persona candidates recover --workdir <p> --link <draft-id> <id>
lore persona candidates recover --workdir <p> --force-dismiss <id>
lore persona errors --workdir <p> [--json]
lore persona summary --workdir <p> [--json] [--fail-on-orphan]
lore usage --workdir <p>
```

Use the flag-before-ID form above until parser normalization is fixed.

### MCP / SDK Boundary

Current contract:

- MCP v1 exposes read and proposal tools only.
- External MCP does not expose direct writes, apply, shell, or persona candidate mutation.
- SDK contract and README argument table are gated.
- SDK E2E remains opt-in via `LORE_SDK_E2E=1`.

This is aligned with `AGENTS.md`: governance and approval are enforced in runtime code, not by prompt convention.

## 4. Validated Findings and Risks

### P0 - Finding State Transitions Are Not Enforced

`Runtime.ResolveFinding` and `Runtime.IgnoreFinding` delegate to `UpdateFindingState`, and all backends overwrite the state without validating a legal transition. This matches the external `lore-state-machine-audit.md` finding.

Impact:

- A resolved or ignored finding can be overwritten back to another state.
- Audit trails can imply a clean state machine while the store permits invalid histories.
- This is inconsistent with the stronger Draft and Persona Candidate contracts.

Suggested fix:

- Add `model.ValidateFindingTransition(from, to)`.
- Enforce in memory/json/sqlite backends or in app plus backend tests.
- Add table-driven tests across all three backends.
- Prefer CAS-style update for SQLite: `UPDATE ... WHERE id=? AND state=?`.

### P0 - `ApplyDraft` Has a Vault/State Consistency Window

`Harness.ApplyDraft` writes the vault file with `vault.WriteFileAtomic` before persisting the draft as `applied`. If the store transition fails after the file write, the vault has changed while the draft can remain `approved`.

Impact:

- Operator may retry apply and hit conflict or duplicate semantics.
- Audit can under-report a real file mutation.
- This is rare but central to governance correctness.

Suggested fix options:

- Add a recoverable apply state such as `apply_write_succeeded` or an audit/finding path when state persistence fails after write.
- Or flip to a prepare/commit protocol where the draft is marked in-progress before write, then resolved after write.
- Add failure-injection tests around write-success/state-fail.

### P1 - Persona CLI Flag Ordering Mismatch

Docs currently show ID-first forms in some places:

```powershell
lore persona candidates show <id> [--json]
lore persona candidates draft <id> [--retry-rejected]
lore persona candidates recover <id> --link <draft-id>
```

The parser uses Go `flag.FlagSet.Parse(args)`, which stops parsing flags after the first positional argument. Therefore:

- `show pc-1 --json` returns human output instead of JSON.
- `draft pc-1 --retry-rejected` does not take the retry path.
- `recover pc-1 --link draft-1` reports missing recover mode.

Suggested fix:

- Normalize positional ID and flags before `FlagSet.Parse`, or manually parse these three commands.
- Add tests for both `show --json <id>` and `show <id> --json`.
- Add tests for `draft <id> --retry-rejected`.
- Add tests for `recover <id> --link <draft-id>` and `recover <id> --force-dismiss`.
- Keep docs consistent after the parser change.

### P1 - Operator-Agent Turn Cancellation Is Not Threaded

Operator model calls use `context.Background()` inside the loop. TUI/console cannot cancel an in-flight model call or a long tool call at the turn boundary.

Impact:

- Slow provider calls can outlive the user interaction.
- TUI cancellation cannot reliably stop the backend turn.
- Resource cleanup remains best-effort.

Suggested fix:

- Thread `context.Context` through console/TUI `Session.Handle`, `LoopAgent.Respond`, model client calls, and tool runtime calls.
- Add turn-level timeout config.
- Add tests for canceled context before model call, during model call, and during tool call.

### P1 - Tool Results Are Re-Injected Into Model Context Unbounded

UI/session excerpts are bounded, but the full tool result content can be appended back into model messages.

Impact:

- Large vault reads/searches can bloat context.
- Model cost and failure rate can spike.
- A malicious or accidental large tool response can degrade the loop.

Suggested fix:

- Add a model-context-specific truncation/summarization boundary for tool results.
- Make binary/redaction logic shared with TurnStep excerpts where possible.
- Add tests that a huge tool result is bounded before re-injection.

### P1 - TUI `View()` Still Has Side Effects

The TUI currently refreshes model-derived content in `View()`. Bubble Tea convention expects `View()` to be pure rendering.

Impact:

- Harder to reason about render/update ordering.
- Future async work can create subtle UI state bugs.
- Tests may pass while user interaction sees timing issues.

Suggested fix:

- Move content refresh into `Update()` or explicit messages.
- Keep `View()` as pure string rendering.
- Add a test that repeated `View()` calls do not mutate state.

### P1 - SDK Stdio Frame Allocation Has No Max Size

`sdk/go/lore` reads `Content-Length` and allocates that exact payload size.

Impact:

- A bad server or corrupt stream can cause large allocation/OOM.
- This is a client hardening gap even if local server is trusted.

Suggested fix:

- Add `MaxFrameBytes` with a conservative default.
- Return a typed transport error when exceeded.
- Add tests for over-limit frames.

### P2 - Config and Vault Infra Need Hardening

Validated issues from external infra review:

- Config durations are not consistently human-friendly.
- Config validation is thin.
- Env override story is partial and feature-specific.
- Vault atomic writes lack fsync and cross-device rename fallback.
- Vault search returns first match per file.
- Backlinks only cover wiki-link patterns.

Suggested fix:

- Add explicit config validation on runtime open.
- Add human-readable duration parsing consistently.
- Add optional env override layer for operational knobs.
- Add vault write durability options and failure tests.

### P2 - Sessionlog Scaling and Index Durability

Sessionlog is now safer than the external review snapshot in several areas: truncation exists, scanner buffer is larger, corrupt lines are tolerated, and task-turn tests pass. Still, index/write behavior is not designed for large multi-month logs.

Suggested fix:

- Make index writes atomic.
- Add recovery/rebuild behavior for corrupt indexes.
- Avoid O(n) full-file work on every append if logs grow.
- Add tests with large session files.

### P2 - CLI Monolith and Flag Parser Consistency

`internal/cli/cli.go` has accumulated many command families and bespoke parser helpers.

Impact:

- New feature slices are likely to introduce inconsistent flag semantics.
- Reviewer cost is high because behavior is distributed in a single large file.

Suggested fix:

- Split persona commands into `internal/cli/persona_commands.go`.
- Add a small shared parse helper for "flags anywhere around one positional ID".
- Keep `cmd/*` wrappers thin.

## 5. External Architect Docs Integration

The truly new external material is the extracted Hermes workspace:

```text
C:\Users\15892\Desktop\docs\hermes-workspace
```

Files:

- `adapter-handoff.md`
- `app-module-handoff.md`
- `lore-console-llm-handover.md`
- `lore-infra-handover.md`
- `lore-model-domain-handover.md`
- `lore-orchestrator-operatoragent-handover.md`
- `lore-sdk-handover.md`
- `lore-state-machine-audit.md`
- `store-module-handover.md`
- `tui-module-handoff.md`

Do not commit the whole `C:\Users\15892\Desktop\docs` root. It contains many copied repo docs and drifted snapshots. If these architect docs should be archived in the repo, commit only the 10 Hermes markdown files under a scoped archive path:

```text
docs/archive/architect-review/2026-05-22/hermes-workspace/
```

Also add one index:

```text
docs/archive/architect-review/2026-05-22/index.md
```

Do not commit `hermes-workspace.tar.gz` if the extracted docs are committed.

## 6. Recommended Roadmap

### R0 - Before the Next Demo or Human Test Batch

1. Fix persona CLI flag ordering or update every doc to flag-before-ID only.
2. Re-run `.\scripts\release-gate.ps1 -PersonaAcceptance -SkipDiffCheck`.
3. Run one real LLM persona memory manual test from `docs/persona-memory-manual-test.md`.
4. Push the 24 ahead commits only after confirming no accidental local doc snapshot is staged.

Suggested owner: B/CLI reviewer.

### R1 - State Machine Hardening

1. Add Finding transition validation and backend tests.
2. Add `ApplyDraft` write/state failure recovery.
3. Normalize SQLite state update strategy around CAS/transactions.
4. Clarify draft `created` vs `pending_review` semantics in domain/runtime.

Suggested owner: runtime/store/orchestrator.

### R2 - Operator Loop and TUI Hardening

1. Thread turn context/cancellation through console/TUI/operatoragent/tool runtime.
2. Bound tool-result re-injection into model context.
3. Move TUI refresh side effects out of `View()`.
4. Add explicit viewport height ownership per panel.

Suggested owner: console/operatoragent/TUI shell.

### R3 - Persona UX Expansion

1. Add TUI persona candidate review panel if CLI workflow proves useful.
2. Add JSON contract tests for all persona read surfaces.
3. Consider MCP read-only exposure for candidates only if external agents need review context.
4. Keep mutation commands local CLI/TUI only unless governance requirements change.

Suggested owner: B persona + A TUI, with runtime boundary review.

### R4 - Infra and Scale

1. Config validation and consistent duration/env handling.
2. Vault atomic write durability improvements.
3. Sessionlog index atomicity and large-log performance.
4. SDK max frame size.
5. Long-running app-server attach loop if Codex App Server becomes a primary integration.

Suggested owner: infra/adapter/SDK.

## 7. Human Testing Plan

Use a fresh workdir for manual tests:

```powershell
$work = Join-Path $env:TEMP ("lore-human-" + [guid]::NewGuid().ToString("N"))
New-Item -ItemType Directory -Force $work | Out-Null
```

Configure model:

```powershell
$env:LORE_LLM_BASE_URL = "<endpoint>"
$env:LORE_LLM_API_KEY = "<key>"
$env:LORE_LLM_MODEL = "<model>"
$env:LORE_LLM_PERSONA_EXTRACT_TIMEOUT = "30s"
```

Run a persona extraction turn:

```powershell
lore console --workdir $work --once "我每周三晚上都会复盘英语听力错题，这件事对我的学习计划很重要。"
lore persona candidates list --workdir $work
lore persona candidates show --workdir $work <candidate-id>
lore persona candidates draft --workdir $work <candidate-id>
lore draft list --workdir $work
```

Check observability:

```powershell
lore persona errors --workdir $work
lore persona errors --workdir $work --json
lore persona summary --workdir $work
lore persona summary --workdir $work --json
lore usage --workdir $work
```

Until the flag-order issue is fixed, keep flags before IDs:

```powershell
lore persona candidates show --workdir $work --json <candidate-id>
lore persona candidates draft --workdir $work --retry-rejected <candidate-id>
lore persona candidates recover --workdir $work --link <draft-id> <candidate-id>
```

Expected manual result:

- Console turn should return normally even if extraction is async.
- `list` should show overview only.
- `show` should show evidence, reason, source, observed time, and linked draft ID after promotion.
- `draft` should create a pending-review `persona_update` draft.
- No persona doc should change until normal draft review/apply.
- `summary --fail-on-orphan` should fail only when a partial orphan exists.

## 8. A/B Development Lines

从这里开始按两条线并行推进。每条线都必须先读本页作为路线图，再回到原始文档确认上下文；不要只按本页摘要开发。

### Shared Reading Gate

Both lines must read these first:

- `AGENTS.md` - repository collaboration boundaries and frozen contracts.
- `docs/handoff-full-project-review-2026-05-23.md` - this consolidated handoff.
- `docs/review-handoff-index.md` - repo-local handoff index.
- `C:\Users\15892\Desktop\docs\hermes-workspace\*.md` - original architect review set. Read the relevant module docs, not only this summary.

Both lines must not commit the whole `C:\Users\15892\Desktop\docs` copied root. If archiving external review docs, copy only `C:\Users\15892\Desktop\docs\hermes-workspace` into a scoped repo archive.

### A Line - Shell, TUI, Operator Loop, SDK, Gates

Mission:

- Make the interactive/user-visible shell robust.
- Keep TUI as a client shell, not a governance owner.
- Preserve MCP read/proposal-only boundaries.
- Keep release gates deterministic and model-free unless explicitly opted in.

Primary code scope:

- `internal/console`
- `internal/operatoragent`
- `internal/tui`
- `internal/sessionlog`
- `internal/cli/tui_workbench.go`
- `cmd/obsidian-harness`
- `scripts/release-gate.ps1`
- `scripts/verify.ps1`
- `sdk/go/lore`

Must read repo docs:

- `docs/handoff-full-project-review-2026-05-23.md`
- `docs/review-handoff-index.md`
- `docs/archive/OPUS_COLLAB_GUARDRAILS.md`
- `docs/archive/review-handoff/review-handoff-codex53-a-line-manual-e2e-release-gate-2026-05-19.md`
- `docs/archive/review-handoff/review-handoff-codex53-a-line-release-gate-hardening-2026-05-19.md`
- `docs/archive/review-handoff/review-handoff-codex53-sdk-release-gate.md`
- `docs/archive/review-handoff/review-handoff-codex53-verify-e2e-switch.md`

Must read original architect docs:

- `C:\Users\15892\Desktop\docs\hermes-workspace\tui-module-handoff.md`
- `C:\Users\15892\Desktop\docs\hermes-workspace\lore-console-llm-handover.md`
- `C:\Users\15892\Desktop\docs\hermes-workspace\lore-orchestrator-operatoragent-handover.md`
- `C:\Users\15892\Desktop\docs\hermes-workspace\lore-sdk-handover.md`
- `C:\Users\15892\Desktop\docs\hermes-workspace\lore-infra-handover.md`

First tasks, in order:

1. Thread turn-level `context.Context` through console/TUI -> `LoopAgent.Respond` -> model calls -> tool calls.
2. Bound tool-result re-injection into model messages, not just UI/session excerpts.
3. Move TUI model/content refresh out of `View()` and add a test that repeated `View()` calls do not mutate state.
4. Make panel viewport height ownership explicit; do not reuse approval viewport height as an implicit clamp for unrelated panels.
5. Add SDK stdio max-frame-size guard and typed over-limit error.
6. Keep `release-gate.ps1 -PersonaAcceptance` passing after B-line changes, but do not own persona governance implementation.

Suggested validation:

```powershell
go test ./internal/console ./internal/operatoragent ./internal/tui ./internal/sessionlog ./internal/cli ./cmd/obsidian-harness -count=1
Push-Location .\sdk\go\lore; go test ./... -count=1; Pop-Location
.\scripts\release-gate.ps1 -SkipDiffCheck
```

Boundary rules:

- Do not change persona candidate state machine semantics from A line.
- Do not add direct writes/apply/shell to MCP.
- Do not put policy or adapter logic into TUI render files.
- Do not make SDK E2E mandatory in normal gates.

### B Line - Governance, Store, Persona, CLI Contracts

Mission:

- Tighten runtime governance and state-machine correctness.
- Keep persona memory reviewable, recoverable, and non-autonomous.
- Make CLI behavior match docs and operator intuition.
- Keep backend parity across SQLite, JSON, and memory stores.

Primary code scope:

- `internal/model`
- `internal/app`
- `internal/orchestrator`
- `internal/store`
- `internal/persona`
- `internal/cli/cli.go`
- `internal/cli/*persona*_test.go`
- `internal/config`
- `internal/vault`
- persona docs under `docs/`

Must read repo docs:

- `docs/handoff-full-project-review-2026-05-23.md`
- `docs/handoff-b-line-completion-2026-05-23.md`
- `docs/persona-memory-pipeline.md`
- `docs/persona-memory-manual-test.md`
- `docs/review-handoff-index.md`
- `docs/archive/review-handoff/review-handoff-b-line-p8-candidate-lifecycle-recovery-2026-05-22.md`
- `docs/archive/review-handoff/review-handoff-b-line-p9-extraction-observability-2026-05-22.md`
- `docs/archive/review-handoff/review-handoff-b-line-p10-list-draft-column-2026-05-22.md`
- `docs/archive/review-handoff/review-handoff-b-line-p11a-usage-purpose-breakdown-2026-05-22.md`
- `docs/archive/review-handoff/review-handoff-b-line-cross-boundary-persona-gate-activation-2026-05-21.md`

Must read original architect docs:

- `C:\Users\15892\Desktop\docs\hermes-workspace\lore-state-machine-audit.md`
- `C:\Users\15892\Desktop\docs\hermes-workspace\store-module-handover.md`
- `C:\Users\15892\Desktop\docs\hermes-workspace\app-module-handoff.md`
- `C:\Users\15892\Desktop\docs\hermes-workspace\lore-model-domain-handover.md`
- `C:\Users\15892\Desktop\docs\hermes-workspace\lore-infra-handover.md`
- `C:\Users\15892\Desktop\docs\hermes-workspace\adapter-handoff.md`

First tasks, in order:

1. Fix persona CLI flag ordering so both `show --json <id>` and `show <id> --json` work. Cover `show`, `draft --retry-rejected`, and `recover --link/--force-dismiss`.
2. Update persona docs only after the parser fix, not before.
3. Add Finding transition validation and backend parity tests across SQLite, JSON, and memory.
4. Harden `ApplyDraft` against the write-success/state-fail window with an explicit recovery/audit path and failure-injection tests.
5. Normalize SQLite state mutations around CAS/transactions where state transitions matter.
6. Add config validation and consistent human-readable duration parsing for operational knobs.
7. Archive the 10 Hermes docs only if requested, under `docs/archive/architect-review/2026-05-22/`, with an index. Do not archive the external root snapshot.

Suggested validation:

```powershell
go test ./internal/persona ./internal/app ./internal/cli ./internal/store ./internal/store/sqlitestore ./internal/orchestrator -count=1
go test ./cmd/obsidian-harness -count=1 -run "PersonaMemoryCandidate|Smoke"
.\scripts\release-gate.ps1 -PersonaAcceptance -SkipDiffCheck
git diff --check
```

Boundary rules:

- Do not auto-approve or auto-apply persona drafts.
- Do not bypass `Harness.ProposePersonaUpdate` for persona draft creation.
- Do not expose persona candidate mutation through MCP unless a separate governance review explicitly approves it.
- Do not modify TUI presentation files for B-line CLI/runtime fixes.

## 9. Review Checklist for Future Slices

Every future slice should answer these before merge:

- Does it preserve MCP read/proposal-only boundaries?
- Does it preserve persona no-auto-apply?
- Does it introduce a state transition? If yes, is it validated and tested across backends?
- Does it write the vault? If yes, what happens if store/audit fails afterward?
- Does it add CLI flags? If yes, are flag positions tested around positional args?
- Does it add model/tool output to context? If yes, is it bounded?
- Does it touch TUI `View()`? If yes, verify no state mutation happens inside render.
- Does it affect SDK transport? If yes, verify frame size/cancellation behavior.
- Does it add docs from outside the repo? If yes, confirm there is no stale duplicate source of truth.

## 10. Suggested Next Commands

For a clean review baseline:

```powershell
git status --short --branch
go test ./... -count=1
Push-Location .\sdk\go\lore; go test ./... -count=1; Pop-Location
.\scripts\release-gate.ps1 -PersonaAcceptance -SkipDiffCheck
```

For the immediate CLI parser fix:

```powershell
go test ./internal/cli -count=1 -run "RunPersona|Persona"
go test ./cmd/obsidian-harness -count=1 -run "PersonaMemoryCandidate"
git diff --check
```

For runtime/store hardening:

```powershell
go test ./internal/store ./internal/app ./internal/orchestrator -count=1 -run "Finding|Draft|Apply|Transition"
go test ./... -count=1
```

## 11. Final Guidance

The project is in a strong enough state for real manual testing, but not for broad feature expansion without tightening state-machine safety. The next best move is:

1. Fix persona CLI flag ordering.
2. Run the manual persona memory cookbook against a real model.
3. Address Finding transitions and ApplyDraft recovery.
4. Then add TUI persona review UX if the CLI workflow proves valuable.

Avoid adding new agent surfaces before the runtime invariants above are tightened. The current value is governance correctness; keep that as the organizing constraint.
