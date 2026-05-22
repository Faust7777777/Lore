# Review Handoff: CloneTurnSteps Contract Clarification

Date: 2026-05-22

## Scope

- `internal/operatoragent/model.go`

## Summary

Clarified the `CloneTurnSteps` documentation so it matches the current implementation exactly.

The helper copies each `TurnStep` value and rebuilds the top-level `Arguments` map. It does not recursively clone nested maps or slices inside `Arguments`. That is acceptable for the current Lore tool schemas because tool arguments are flat scalar/string primitives by convention.

## Boundary

- No behavior change.
- No MCP surface change.
- No TUI rendering change.
- No frozen contract shape change.

## Review Focus

- Confirm the comment no longer overclaims recursive/deep-copy semantics.
- Confirm the future-schema caveat is explicit: if nested tool arguments are introduced later, `CloneTurnSteps` must be upgraded to recursive cloning in the same change.

## Validation

```powershell
go test ./internal/operatoragent -run "TestCloneTurnStepsDeepCopiesArguments|TestModelAgentRespondStepsAreIsolatedFromTraceAndOtherSnapshots" -count=1 -v
git diff --cached --check -- internal\operatoragent\model.go
```
