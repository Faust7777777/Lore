# Review Handoff: Manual E2E Release Gate Input

## Scope

This A-line CI/docs change lets maintainers run the opt-in E2E gate from GitHub Actions manually.

Changed files:

- `.github/workflows/release-gate.yml`
- `README.md`

No runtime, TUI, operator-agent, MCP contract, or release script behavior is changed.

## Why

`release-gate.ps1 -E2E` already exists locally, but the GitHub workflow had no manual input to append those checks. This left SDK/MCP E2E validation as a local-only command even though `workflow_dispatch` existed.

## Behavior

- Pull requests still run `.\scripts\release-gate.ps1` only.
- Pushes to `main` still run `.\scripts\release-gate.ps1 -Full` only.
- Manual `workflow_dispatch` runs still run `-Full` by default.
- Manual `workflow_dispatch` runs with `e2e=true` run `.\scripts\release-gate.ps1 -Full -E2E`.

The full/manual path still requires `LORE_LLM_BASE_URL`, `LORE_LLM_API_KEY`, and `LORE_LLM_MODEL`.

## Review Focus

- Confirm PRs still avoid real-model and E2E requirements.
- Confirm `push main` does not unexpectedly run E2E.
- Confirm only manual runs can opt into E2E.
- Confirm the workflow keeps the existing final cleanliness assertion.

## Verification

```powershell
git diff --check -- .github/workflows/release-gate.yml README.md docs/archive/review-handoff/review-handoff-codex53-manual-e2e-release-gate.md
```
