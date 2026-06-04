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
| `process_sink.write_empty_slots` | B-line | write placeholder checkpoints for empty windows | dead |
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

1. `write_empty_slots` — most contained. The single placeholder-writing
   chokepoint is `processsink.Service.WriteCheckpoint` (constructed once at
   `orchestrator/harness.go:63`). Gate it there; needs a decision on how
   `WriteCheckpoint`/`IngestSessionWindow` represent a skipped slot (the
   reason it was not done in this review tick — it is a contract change,
   not a drop-in).
2. `retention_days` — a new prune pass over process-sink checkpoints /
   daily reports (store-level delete-older-than). Feature-sized.
3. `daily_rollup_at` — daemon scheduling; touches the daemon (boundary).

## Out of boundary (flag for the owning line, do not fix in B-line)

The entire `runtime.*` block (server/daemon config) is dead. Either wire
it in the daemon/server line or drop the fields + their validation so the
config surface stops advertising knobs that do nothing.
