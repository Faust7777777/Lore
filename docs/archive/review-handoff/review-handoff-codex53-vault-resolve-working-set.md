# Review Handoff: vault_resolve Selected Path Working Set

## Change

`vault_resolve` unique results now flow into the session working set without relying on the model to repeat the selected path in its final answer.

Changed files:

- `internal/console/tool_runtime.go`
- `internal/console/session.go`
- `internal/console/tool_runtime_test.go`
- `internal/console/session_test.go`

## Reason

`vault_resolve` returns unique matches as `selected_path`, while the session working set previously recognized only `path` and `target_path`. If the final assistant message did not repeat the resolved path, follow-up turns could miss the resolved note context.

## Behavior

- When `vault_resolve` returns `status == "unique"` with a markdown `selected_path`, the console tool runtime enriches the trace arguments with `selected_path`.
- The session working-set extractor now recognizes `selected_path`.
- Ambiguous and not_found results do not add working-set paths through this path.
- Any model-supplied stale `selected_path` argument is cleared before executing `vault_resolve`.
- Assistant final JSON that looks like a `vault_resolve` result is treated by status: `unique` remembers only `selected_path`, while `ambiguous` and `not_found` do not recurse into candidate `matches`.

## Verification

```powershell
.\.tools\go\bin\go.exe test ./internal/console -run 'Test(ToolRuntimeVaultResolve|SessionHandle.*VaultResolve)' -count=1 -v
powershell.exe -NoProfile -ExecutionPolicy Bypass -Command ".\scripts\verify.ps1"
```

Both passed locally.

## Review Focus

- Confirm mutating the tool argument map for trace enrichment is acceptable here.
- Confirm only unique markdown `selected_path` is remembered.
- Confirm ambiguous/not_found cannot leak stale `selected_path` or candidate `matches` into WorkingSet.
- Confirm no SDK/TUI behavior changed.
