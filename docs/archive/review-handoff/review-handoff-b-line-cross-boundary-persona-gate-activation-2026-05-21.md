# Review Handoff: B-line Cross-Boundary Activation of Persona Memory Acceptance Gate

Date: 2026-05-21

Owner line touched: A-line governance / release gates (normally codex53 / 江物外)

Author: B-line developer (main developer B, persona memory candidate pipeline)

## Why this is cross-boundary

The persona memory candidate acceptance gate is A-line scope:

- `cmd/obsidian-harness/main_test.go` end-to-end acceptance scaffold
- `cmd/obsidian-harness/main_test.go` fake LLM server (`newOperatorAgentTestServer`)
- `scripts/release-gate.ps1` opt-in `-PersonaAcceptance` switch
- the acceptance plan at `docs/plans/2026-05-20-persona-memory-acceptance.md`

A-line was originally going to activate this gate after B finished the
backend slices (B-P1 through B-P9). With B-P1 ~ B-P9 landed
(`83192c1`..`4f3aaac`) and A-line owner temporarily unavailable, the
B-line developer was explicitly authorised to cross the boundary just
to activate the gate -- the boundary contract otherwise remains the
same and B-line will not touch A-line surfaces outside this slice.

## Scope of cross-boundary edits

Three files, all on the A boundary:

- `cmd/obsidian-harness/main_test.go`
  - `newOperatorAgentTestServer` now detects the persona extractor
    system prompt (`"extract candidate persona facts"`) on both
    `/chat/completions` and `/responses` and returns a one-candidate
    JSON payload. Both transports are covered because
    `configureLLMTestEnv` sets `LORE_LLM_MODEL=gpt-5.4`, which routes
    through `/responses`, while other tests (and the future Sonnet /
    other-provider paths) may still hit `/chat/completions`.
  - new helper `personaExtractorAcceptancePayload(t)` returns the
    scripted JSON body. The `evidence_quote` is a verbatim substring
    of the acceptance scaffold's user utterance
    ("我每周三晚上都会复盘英语听力错题，这件事对我的学习计划很重要。")
    so the parser's NFKC substring validation does not discard the
    candidate.
  - `TestRunPersonaMemoryCandidateAcceptanceScaffold` no longer calls
    `t.Skip(...)`. The scaffold's existing flow is preserved:
    1. console one-shot user turn lands;
    2. async extractor produces a candidate (now mocked through the
       fake LLM);
    3. `lore persona candidates list` shows the candidate row;
    4. `lore persona candidates show <id>` renders all detail labels
       (Field / Proposed value / Evidence / Reason / Confidence /
       Source / Observed at);
    5. no persona_update draft is auto-created;
    6. the persona file at `vault/人物画像.md` is not auto-mutated.

- `scripts/release-gate.ps1`
  - the previous `throw` guarding `-PersonaAcceptance` is replaced by
    an `Invoke-GoGate` block targeting
    `./cmd/obsidian-harness -Run TestRunPersonaMemoryCandidateAcceptanceScaffold$`.
  - the default release-gate path (no `-PersonaAcceptance` flag) is
    unchanged, so PR runs do NOT include this gate. The persona
    acceptance gate stays opt-in until A-line confirms the gate
    cadence (planned: include in nightly / pre-release, not PR).

- `docs/archive/review-handoff/review-handoff-b-line-cross-boundary-persona-gate-activation-2026-05-21.md`
  - this handoff document.

## What was NOT touched

Confirming the cross-boundary edit is genuinely minimal:

- No `internal/persona/*` changes (extractor / model / prompt / parser).
- No `internal/store/*` changes.
- No `internal/app/persona_candidate.go` or `internal/app/runtime.go` changes.
- No `internal/cli/cli.go` or `internal/cli/persona_command_test.go` changes.
- No MCP surface change.
- No TUI rendering change.
- No CI definition change beyond release-gate.ps1.
- No commit to main happened on shared CI / smoke pipelines outside
  the new persona acceptance Invoke-GoGate block.

## Verification performed

```powershell
go test ./cmd/obsidian-harness -count=1 -run TestRunPersonaMemoryCandidateAcceptanceScaffold -v
# PASS in 0.10s

powershell.exe -NoProfile -ExecutionPolicy Bypass -File .\scripts\release-gate.ps1 -PersonaAcceptance -SkipDiffCheck
# [gate] persona memory candidate acceptance
# === RUN   TestRunPersonaMemoryCandidateAcceptanceScaffold
# --- PASS: TestRunPersonaMemoryCandidateAcceptanceScaffold (0.09s)
# ...
# [gate] release gate passed
```

The full `-PersonaAcceptance` smoke (operator agent + persona extractor +
acceptance + every other gate the release script runs) is green.

## Recommended A-line review focus

- Confirm the cross-boundary edit set is exactly the three files above
  and contains no shadow business-line changes.
- Confirm `personaExtractorAcceptancePayload`'s scripted candidate is
  acceptable as the acceptance gate's canonical fake (Field, evidence
  shape, reason text, confidence). The current values are PM-friendly
  Chinese strings matching the scaffold's user utterance.
- Confirm `-PersonaAcceptance` should remain opt-in (not added to the
  default release-gate path) until A-line decides the cadence.
- Confirm the handoff doc lives at the expected path. A-line owner can
  rename it without rewriting -- it is historical.

## Follow-ups for A-line (NOT done in this slice)

- Decide PR-vs-nightly cadence for `-PersonaAcceptance`.
- Decide whether the acceptance gate should pin a stable model name
  (currently `gpt-5.4` via `configureLLMTestEnv`) or allow override
  via env when running the gate against a live provider.
- Consider adding a parallel acceptance scaffold for
  `lore persona candidates draft <id>` (today the gate stops at
  list+show + asserts no auto-draft; it does not exercise the
  manual promote / retry-rejected paths that B-P8 added).
