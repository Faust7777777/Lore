# B-line review: config layer enforcement gaps (2026-06-04)

Audience: B-line owner + reviewer. Origin: framework-review tick of the
`/loop` after the Hermes full-project audit flagged "usage config not
taking effect". That turned out to be the visible tip of a systemic
issue.

## Finding

**9 `config.Config` fields are parsed, defaulted, and (some) validated,
but never read or enforced anywhere** — `grep` for each field name returns
only `internal/config/*` (struct def + `Default` + `Validate`) and config
tests; no consumer dereferences them. A workspace/user config that sets
any of these has zero effect.

| Field | Boundary | Intended effect (inferred) | Status |
|---|---|---|---|
| `usage.track_usage` | B-line | gate usage recording | **FIXED** this branch (`2e9479a`) |
| `usage.soft_warning_tokens` | B-line | soft daily budget warning | **FIXED** this branch (`db0b7a8`) |
| `process_sink.write_empty_slots` | B-line | write placeholder checkpoints for empty windows | **FIXED** this branch |
| `process_sink.retention_days` | B-line | prune process-sink docs older than N days | dead |
| `process_sink.daily_rollup_at` | B-line / daemon | schedule the daily rollup at HH:MM | dead |
| `runtime.inspect_at` | daemon (out-of-boundary) | scheduled inspect time | dead |
| `runtime.listen_address` | server (out-of-boundary) | server bind address | dead |
| `runtime.max_event_queue` | daemon (out-of-boundary) | event queue cap | dead |
| `runtime.max_draft_queue` | daemon (out-of-boundary) | draft queue cap | dead |
| `runtime.max_memory_mb` | daemon (out-of-boundary) | memory cap | dead |
| `runtime.proactive_mode` | daemon (out-of-boundary) | proactive behavior switch | dead |

(`process_sink.checkpoint_every` IS enforced — via `resolveCodexWindowSize`
— and the `paths.*` / `vault.*` / `llm.*` / `bootstrap.*` groups are
consumed. The dead set is the entire `runtime.*` block plus 3 of 4
`process_sink.*` fields.)

## Misleading comment

`internal/config/config.go:84-87` states that `Runtime.InspectAt` and
`ProcessSink.DailyRollupAt` are "**consumed by daemon code** that assumes
the strings can be split on ':'". Neither is consumed by any code. The
`hhmmPattern` validation is real; the consumption claim is not. The
comment should be corrected to "validated for a future daemon scheduler;
not yet wired" so it stops implying behavior that does not exist.

## Why this matters

`Validate` rejects malformed values for these fields (e.g. a bad
`daily_rollup_at` or negative `retention_days`), so an operator gets a
confident "config accepted" signal and reasonably assumes the setting
works — but nothing happens. That is worse than an unvalidated no-op: it
actively misleads. Same root cause the audit named for usage.

## Suggested priority (B-line slices)

1. `write_empty_slots` — **FIXED** (this branch). Gated at
   `importCodexWindowsContext` — the chokepoint every window-ingestion
   path (import / external / sync / appserver) funnels through: an
   empty-transcript window is skipped when the flag is false, before the
   model call. No `WriteCheckpoint` contract change was needed after all.
2. `retention_days` — **deferred (low priority).** Enforcing it needs a
   new `ProcessSinkStore` delete-older-than capability across all three
   backends (memory / json / sqlite) plus `.md` file cleanup, a service
   prune, and a trigger — a multi-slice feature. The value is modest
   (process-sink docs are small markdown files with low daily volume), so
   the multi-backend store change is not worth a rushed loop tick. Build
   deliberately if/when storage growth becomes a real concern.
3. `daily_rollup_at` — daemon scheduling; touches the daemon (boundary).

## Out of boundary (flag for the owning line, do not fix in B-line)

The entire `runtime.*` block (server/daemon config) is dead. Either wire
it in the daemon/server line or drop the fields + their validation so the
config surface stops advertising knobs that do nothing.

## Adjacent finding: write-then-audit consistency

A concrete instance of the audit's "state consistency via compensation":
`app/findings.go updateFindingState` changes a finding's state, then
appends an audit record; if the audit append fails it returns an **empty**
finding plus the error -- so the caller is told the operation failed even
though the state change already committed. The operator then sees a
confusing "resolved -> resolved invalid transition" on retry, and the
audit trail is missing the change.

This contradicts the codebase's own pattern: `orchestrator/harness.go
recordAudit` treats audit as best-effort (marks health-error, does not
fail the primary operation). Recommended fix: align `updateFindingState`
so a committed state change is not reported as total failure -- at minimum
return the updated finding (not a zero value) with a descriptive error.
Doing it cleanly wants an app-layer best-effort-audit seam (the Runtime
calls the store's Audit directly today), and a test needs a fake store
whose Audit fails while Findings succeeds -- a small but non-trivial
slice, flagged here rather than rushed.

