# B-line review handoff: Hermes audit follow-ups (2026-06-04)

Branch: `b-line/audit-followups-2026-06-04` — 14 commits ahead of
`origin/main`, **not pushed** (per the review-then-push gate). Origin: the
Hermes full-project audit flagged five B-line-relevant pain points; this
branch closes the in-boundary ones. Reviewer: DeepSeek. This doc is the
map — what each commit does, which audit finding it answers, and where to
focus.

## Verification status (whole module, not just `./internal/...`)

- `go build ./...` — clean
- `go vet ./...` — clean
- `go test ./...` — green incl. `cmd/` and `cmd/obsidian-harness`

The whole-module sweep caught one latent push-blocker the per-package runs
had hidden — see commit `c482020` below. **`go build` alone is not
sufficient: it skips `_test.go`**, so an interface rename can leave a test
fake uncompilable while build stays green.

## Audit finding → commits → review focus

### 1. "usage 配置未真正生效" (usage config had no effect)

| Commit | What |
|---|---|
| `2e9479a` | `RecordUsage` now gates on `Config.Usage.TrackUsage` — a disabled config actually stops recording. |
| `db0b7a8` | `usage.soft_warning_tokens` surfaced as a soft-budget nudge in `lore status`. |

This was the visible tip of a systemic issue: **9 config fields parsed +
defaulted + validated but never read**. Full inventory + which are
in/out-of-boundary in `docs/review-b-line-config-enforcement-2026-06-04.md`
(commit `d0ade8c`, updated `d158cf6`). Review focus: confirm the two
enforced fields read from the resolved (layered) config, not a default.

### 2. "process-sink 无业务取消" (no business cancellation)

Threaded a cancellable `context.Context` through the process-sink paths,
using the codebase's **additive `…Context` pattern** (keep the non-ctx
method as a delegator calling the ctx variant with `context.Background()`),
so no existing caller/test signature changed.

| Commit | Slice |
|---|---|
| `2755f0e` | 1/3 — summarizer made cancellation-capable. |
| `489bc54` | 2/3 — import path (`importCodexWindowsContext` + helper). |
| `a17f69e` | 3/3 — sync/attach path (`SyncCodexJSONLContext`, `CodexJSONLSyncer` interface, `syncCodexAttachOnce`). |

Review focus: CLI commands wrap `signal.NotifyContext(…, os.Interrupt)` so
Ctrl-C cancels mid-run; the additive delegators keep `context.Background()`
behaviour identical for non-cancellable callers.

### 3. "状态一致性依赖补偿" (consistency relies on compensation)

| Commit | What |
|---|---|
| `b1cd066` | `updateFindingState` returns the **committed** finding + a wrapped error on audit-append failure (was returning `Finding{}` — pretending the state change didn't happen). |

The broader review of failure-handling across draft/finding/apply/status
paths is in `docs/review-b-line-failure-semantics-2026-06-04.md` (commit
`31c7744`): the framework follows one invariant — **reads are best-effort;
mutations fail hard and emit a governance Finding on a partial vault side
effect (`ApplyDraft`); audits are best-effort but degrade `lore status`
health visibly.** The audited-sound core paths are listed there so they need
no re-derivation. Review focus: confirm the invariant statement matches the
code; this is the doc to push back on if the model of the system is wrong.

### 4. "入口分散" (scattered operator entry points)

| Commit | What |
|---|---|
| `db9cf68` | `OperatorQueue` DTO + `lore inbox` — one read-only view of pending drafts / open findings / open persona candidates + today's usage glance. |
| `3f1fc62` | `lore inbox --json` + an action-item nudge in `lore status`. |
| `e948f90` | Queue tolerates a usage-summary failure (best-effort glance; never hides action items). |

Review focus: the queue reuses the *same* app methods the dedicated commands
use (`ListDrafts` / `ListFindings` / `ListPersonaCandidates` /
`SummarizeUsage`), so it cannot disagree with them. It is read-only — the
operator still acts through the existing commands.

### 5. `process_sink.write_empty_slots` enforcement

| Commit | What |
|---|---|
| `1746d51` | The gate is now honoured in `importCodexWindowsContext` (placeholder checkpoints for empty windows are only written when configured). |

### Housekeeping

| Commit | What |
|---|---|
| `c482020` | Sync the `countingCodexSyncer` test fake in `cmd/obsidian-harness` to the renamed `SyncCodexJSONLContext` interface (regression from `a17f69e`; build-green but vet/test-red until fixed). |

## Out of boundary / deferred (NOT in this branch)

- **"tool-schema 有多处来源"** — MCP/tool-schema unification is A/MCP-line.
- **`runtime.*` config block** (6 fields: `inspect_at`, `listen_address`,
  `max_event_queue`, `max_draft_queue`, `max_memory_mb`, `proactive_mode`)
  — daemon/server line. Documented as dead in the config-enforcement note,
  not actioned here.
- **`process_sink.retention_days` / `daily_rollup_at`** — deferred
  (low-value / multi-backend pruning + daemon-scheduled rollup).
- **Watchers** (`vault_watcher.go`, `single_file_watcher.go`) — daemon
  infra; reviewed read-only this session, found sound, left untouched.

## Boundary assertion

Only B-line files touched: `internal/app/*`, `internal/cli/*`, one
`cmd/obsidian-harness/*_test.go`, and `docs/*`. The 13 A/TUI WIP files in the
working tree were left unstaged and unmodified throughout.
