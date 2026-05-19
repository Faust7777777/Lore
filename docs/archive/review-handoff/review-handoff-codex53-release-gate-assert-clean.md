# Review Handoff: Release Gate Cleanliness Assertion

## Scope

This A-line change adds an optional repository cleanliness assertion to `scripts/release-gate.ps1` and makes GitHub Actions use it.

Changed files:

- `scripts/release-gate.ps1`
- `.github/workflows/release-gate.yml`
- `README.md`

No runtime, TUI, operator-agent, MCP, or SDK implementation changed.

## Why

The GitHub workflow already had a final cleanliness assertion, but the release-gate script itself had no reusable clean-check mode. Keeping the assertion only in workflow YAML means local/other callers can run a supposedly release-grade gate without checking for repo pollution.

## Behavior

`release-gate.ps1` now accepts `-AssertClean`.

When enabled, it checks in `finally`:

- repo-local `.smoke-workdir` was not created;
- `git status --short` is empty.

GitHub Actions now passes `-AssertClean` for both PR and full gates. The existing final workflow cleanup assertion remains as a defensive fallback with `if: always()`.

Local developers can still run `release-gate.ps1 -SkipDiffCheck` on a dirty collaborative worktree without triggering the clean assertion.

## Review Focus

- Confirm `-AssertClean` is opt-in and does not break dirty local collaboration.
- Confirm CI uses `-AssertClean` for both PR and full paths.
- Confirm the final workflow cleanliness step still runs on failures.
- Confirm the assertion executes after `Pop-Location` is not required; it runs while still at repo root.

## Verification

```powershell
powershell.exe -NoProfile -ExecutionPolicy Bypass -File .\scripts\release-gate.ps1 -SkipDiffCheck
```

A full `-AssertClean` run requires a clean worktree, so it should be validated in CI or a clean temporary worktree.
