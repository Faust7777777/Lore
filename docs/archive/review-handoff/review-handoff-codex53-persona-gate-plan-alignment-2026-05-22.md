# Review Handoff: Persona Gate Plan Alignment

Date: 2026-05-22

Owner line: A-line governance / release gates

## Scope

Docs-only correction after reviewing `d4625f6`.

Changed file:

- `docs/plans/2026-05-20-persona-memory-acceptance.md`

## Why

`d4625f6` correctly activated the persona memory candidate acceptance gate:

- `TestRunPersonaMemoryCandidateAcceptanceScaffold` no longer calls `t.Skip`.
- `scripts/release-gate.ps1 -PersonaAcceptance` now runs the deterministic fake
  model acceptance test.

The active plan still described the old state:

- scaffold skipped;
- `-PersonaAcceptance` fail-fast;
- activation checklist as future work.

That was documentation drift, not a product/runtime blocker.

## Updated Wording

- Gate shape now says the acceptance test is active.
- `-PersonaAcceptance` is described as an opt-in running gate, not fail-fast.
- Activation checklist is converted into completed prerequisites.
- Remaining decision is cadence: opt-in, nightly/pre-release, or PR.

## Verification

Suggested checks:

```powershell
git diff --check -- docs/plans/2026-05-20-persona-memory-acceptance.md docs/archive/review-handoff/review-handoff-codex53-persona-gate-plan-alignment-2026-05-22.md
go test ./cmd/obsidian-harness -run TestRunPersonaMemoryCandidateAcceptanceScaffold -count=1 -v -timeout 60s
powershell.exe -NoProfile -ExecutionPolicy Bypass -File .\scripts\release-gate.ps1 -PersonaAcceptance -SkipDiffCheck
```

## Review Focus

- Confirm this does not change code or gate behavior.
- Confirm the plan now matches `d4625f6`.
- Confirm PR-vs-nightly cadence remains an explicit open A-line decision.
