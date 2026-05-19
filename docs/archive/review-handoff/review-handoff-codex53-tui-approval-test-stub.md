# Review Handoff: TUI Approval Test Stub Compile Fix

## Scope

This A-line test-only fix updates the approval-state test stub so the release-gate TUI approval tests compile on a clean worktree.

Changed files:

- `internal/tui/approval_state_test.go`

No TUI implementation, rendering, approval behavior, runtime behavior, operator-agent behavior, or MCP behavior changed.

## Why

A clean worktree run of:

```powershell
powershell.exe -NoProfile -ExecutionPolicy Bypass -File .\scripts\release-gate.ps1 -AssertClean
```

failed at the TUI approval state gate because `approvalDriverStub` no longer implemented `InteractiveWorkbenchDriver`. The interface now requires `ExecuteFindingAction`, but this test stub only implemented `Execute` and `ExecuteApprovalAction`.

## Fix

Added a minimal `ExecuteFindingAction` method to `approvalDriverStub`:

- reloads the current view model through `Load("")`;
- returns a simple `LastOutput` string;
- does not mutate approval action state;
- does not touch production code.

## Boundaries

- Test-only change.
- Does not alter approval pane behavior.
- Does not implement finding UI behavior.
- Keeps A-line ownership limited to release-gate/TUI approval-state test health.

## Review Focus

- Confirm this is only a stub completeness fix.
- Confirm approval action assertions remain unchanged.
- Confirm no TUI presentation or workbench implementation files changed.

## Verification

```powershell
go test ./internal/tui -run TestApprovalFlow_ -count=1 -v
```
