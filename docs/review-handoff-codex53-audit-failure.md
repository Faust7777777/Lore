# 5.3 Codex Review Handoff: Audit Failure Visibility

## Scope

This change removes silent audit write drops in orchestrator code without making audit persistence a hard blocker for business operations.

Changed files:
- `internal/runtime/health.go`
- `internal/orchestrator/harness.go`
- `internal/orchestrator/readapi.go`
- `internal/orchestrator/harness_test.go`

## Contract

- Business operations should not fail solely because audit persistence failed.
- Audit failures must not be silent.
- Orchestrator audit writes now go through `Harness.recordAudit`.
- `recordAudit` calls `Auditor.Record`; on error it marks runtime health as `error` with `unexpected_failure` and message prefix `audit record failed:`.
- Read API audit writes use the same helper as write/process-sink/draft audit writes.

## Review Focus

- Confirm there are no remaining `_ = h.auditor.Record(...)` call sites in `internal/orchestrator`.
- Confirm audit failure does not block `WriteLowRiskNote`, but `StatusSnapshot()` exposes the degraded/error health state.
- Confirm `HealthService.MarkError` preserves existing dependency booleans while updating status/message/time.
- Confirm the Unicode escape cleanup in `harness.go` / `harness_test.go` is behavior-preserving and only avoids PowerShell mojibake.
- Confirm no TUI files are part of this change.

## Verification

```powershell
.\.tools\go\bin\go.exe test ./internal/orchestrator ./internal/app ./internal/runtime -count=1
```