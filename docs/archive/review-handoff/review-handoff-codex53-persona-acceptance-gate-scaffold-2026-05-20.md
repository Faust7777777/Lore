# Review Handoff: Persona Acceptance Gate Scaffold

Date: 2026-05-20

Owner line: A-line governance / release gates

## Scope

This slice adds a skipped acceptance scaffold for the future persona memory
candidate pipeline. It does not implement the pipeline.

Changed files:

- `docs/plans/2026-05-20-persona-memory-acceptance.md`
- `cmd/obsidian-harness/main_test.go`
- `scripts/release-gate.ps1`
- `docs/archive/review-handoff/review-handoff-codex53-persona-acceptance-gate-scaffold-2026-05-20.md`

## Intended Boundary

- No `internal/persona` changes.
- No store changes.
- No TUI rendering changes.
- No real-model dependency added to PR gate.
- No MCP surface change.

## Acceptance Scaffold

The new test is:

- `TestRunPersonaMemoryCandidateAcceptanceScaffold`

It is intentionally skipped with:

```go
t.Skip("activate after B candidate storage/console async/CLI review")
```

The scenario documents the future deterministic flow:

1. user states a stable persona fact;
2. async extractor creates a candidate;
3. CLI list shows a candidate overview and parseable `pc-` ID;
4. CLI show displays detailed evidence and metadata;
5. manual draft creation is still required;
6. no automatic persona draft or persona file write occurs.

## Release Gate Wiring

`scripts/release-gate.ps1` has an opt-in reserved switch:

```powershell
.\scripts\release-gate.ps1 -PersonaAcceptance
```

The default release gate and PR path do not run this switch. If invoked now, it
fails fast with an activation message instead of treating the skipped scaffold
as a passing product gate.

## Review Focus

- Confirm the test remains skipped and is not included in the default gate.
- Confirm `-PersonaAcceptance` is opt-in only and currently fails fast.
- Confirm the plan activation checklist requires B extractor, usage, candidate
  store, console async, and CLI review slices before unskipping.
- Confirm list assertions stay overview-level; detailed evidence belongs to
  `persona candidates show`.
- Confirm this slice does not touch business implementation files.

## Suggested Validation

```powershell
go test ./cmd/obsidian-harness -run TestRunPersonaMemoryCandidateAcceptanceScaffold -count=1 -v
powershell.exe -NoProfile -ExecutionPolicy Bypass -File .\scripts\release-gate.ps1 -SkipDiffCheck
powershell.exe -NoProfile -ExecutionPolicy Bypass -File .\scripts\release-gate.ps1 -PersonaAcceptance -SkipDiffCheck
```
