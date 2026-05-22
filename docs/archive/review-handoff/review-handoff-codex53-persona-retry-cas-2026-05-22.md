# Review Handoff: SQLite Persona Retry CAS Hardening

Date: 2026-05-22

Owner line: backend governance / persona candidate lifecycle

## Scope

This is a small follow-up on top of `a3eebab fix(persona,store,app): close retry duplicate-draft race via ClaimCandidateForRetry CAS`.

Changed files:

- `internal/store/sqlitestore/store.go`
- `docs/archive/review-handoff/review-handoff-codex53-persona-retry-cas-2026-05-22.md`

## Why

`ClaimCandidateForRetry` stores `DraftID` inside the JSON payload. The original SQLite implementation verified `DraftID` in Go after `SELECT`, then updated by `id` and `state='drafted'`.

That passed the existing concurrency tests, but the SQL CAS was weaker than memory/jsonstore semantics: a stale reader could theoretically update after a peer changed the payload while leaving `state='drafted'`.

## Fix

The SQLite `UPDATE` now includes the exact `payload` observed by the `SELECT`:

```sql
WHERE id = ? AND state = ? AND payload = ?
```

This makes the SQLite backend reject stale payload writes with `store.ErrConflict`, matching the intended compare-and-set contract.

## Validation

```powershell
go test ./internal/store -run "TestClaimCandidateForRetry" -count=1 -v
go test ./internal/app -run "TestRetryRejectedPersonaDraft(ConcurrentCallsOnlyOneSucceeds|LinkFailureDoesNotDuplicate|Success|Rejects)" -count=1 -v
go test ./internal/app ./internal/store/... ./internal/console -count=1
git diff --check -- internal/store/sqlitestore/store.go docs/archive/review-handoff/review-handoff-codex53-persona-retry-cas-2026-05-22.md
```

## Review Focus

- Confirm SQLite now uses payload equality as the CAS guard.
- Confirm no app, CLI, MCP, or TUI behavior changed in this follow-up.
- Confirm existing persona retry and store tests remain green.
