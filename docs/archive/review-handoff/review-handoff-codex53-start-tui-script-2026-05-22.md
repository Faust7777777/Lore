# Review Handoff: start-tui Helper Script

Date: 2026-05-22

## Scope

- `scripts/start-tui.ps1`

## Summary

Adds a local helper script for preparing and launching the Lore TUI demo workflow.

The script builds `bin\lore-tui.exe`, bootstraps a workdir, optionally seeds daemon findings, and can stop before launch with `-NoLaunch`. It is intended for local demo/review use, not CI gating.

## Safety Boundary

- Generated binary path is under ignored `bin/`.
- Default demo workdir is under ignored `.tmp/`.
- `-Reset` refuses to delete the repository root.
- `-Reset` only removes workdirs under the repository root.
- `-SeedProcessSink` requires explicit LLM environment variables and fails fast if missing.

## Review Focus

- Confirm the reset guard cannot delete arbitrary paths outside the repo.
- Confirm generated demo artifacts stay in ignored paths.
- Confirm this does not change TUI implementation or product behavior.

## Validation

```powershell
powershell.exe -NoProfile -ExecutionPolicy Bypass -File .\scripts\start-tui.ps1 -WorkDir .tmp\tui-demo-review -Reset -SeedFindings -NoLaunch
git status --short
```

Observed result:

- Script built `bin\lore-tui.exe`.
- Script bootstrapped `.tmp\tui-demo-review`.
- Script seeded one out-of-band note finding through daemon scans.
- `git status --short` showed only pre-existing tracked staged files and `?? --workdir/`; no `.tmp/` or `bin/` pollution.
