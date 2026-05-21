# Persona Memory Acceptance Gate

Date: 2026-05-20

Owner line: A-line governance / release gates

Task name: A-Persona-Gate: fake-model persona candidate acceptance

## Purpose

This plan defines the acceptance gate for the persona memory candidate pipeline.
It intentionally does not implement persona extraction, storage, CLI behavior, or
TUI rendering.

The business goal is to make the future persona memory pipeline testable before
it becomes part of the release gate:

1. a user says a stable persona fact during a normal Lore turn;
2. an asynchronous LLM-backed extractor emits a candidate;
3. local CLI can list and show the candidate;
4. creating a governed persona draft remains a manual action;
5. no persona write or draft apply happens automatically.

## Non-Goals

- Do not implement `internal/persona`.
- Do not change store implementations.
- Do not change TUI rendering.
- Do not add a real-model dependency to the PR gate.
- Do not create a parallel persona governance path.
- Do not auto-create persona update drafts from candidates in the acceptance
  scenario.

## Acceptance Scenario

Input:

```text
我每周三晚上都会复盘英语听力错题，这件事对我的学习计划很重要。
```

Expected fake-model sequence after B-line implementation:

1. The normal user turn completes without waiting for persona extraction.
2. The persona extractor runs asynchronously.
3. The extractor returns one candidate:
   - fact: the user reviews English listening mistakes every Wednesday night;
   - evidence quote: a normalized substring of the source user message;
   - confidence: deterministic fake value;
   - source turn/session identifiers.
4. `lore persona candidates list --workdir <dir>` shows a candidate overview:
   - parseable candidate ID;
   - coarse proposed value or field, such as `英语听力错题`;
   - next-action guidance such as `lore persona candidates draft <id>`.
5. `lore persona candidates show --workdir <dir> <id>` shows:
   - fact;
   - evidence quote;
   - detailed evidence text such as `我每周三晚上都会复盘英语听力错题`;
   - source metadata;
   - reason, confidence, and observed time;
   - status.
6. The output must state that a persona draft still requires manual creation.
7. No `persona_update` draft is created automatically by this scenario.
8. No persona document is modified automatically by this scenario.

## Gate Shape

The scaffold test lives in `cmd/obsidian-harness/main_test.go`:

- test name: `TestRunPersonaMemoryCandidateAcceptanceScaffold`;
- current state:
  `t.Skip("activate after B candidate storage/console async/CLI review")`;
- model dependency: fake model only;
- PR release gate: not active until B-line implementation lands.

`scripts/release-gate.ps1` reserves an opt-in persona acceptance switch behind
`-PersonaAcceptance`. While the scaffold is skipped, this switch fails fast
instead of running a skipped test as a false pass. The default PR gate does not
run this switch.

## Activation Checklist

Do not remove the skip until all items are true:

- B extractor slices have landed: extractor contract, prompt, result model, and
  NFKC evidence validation are implemented.
- B usage slice has landed: usage records distinguish at least `chat` and
  `persona_extract`.
- B P3 has landed: candidate persistence exists.
- B P4 has landed: persona extraction can be triggered from a normal
  console/CLI turn without blocking the user response.
- B P6 has landed: local CLI can list/show candidates and create a manual draft
  action.
- Manual candidate-to-draft promotion reuses `harness.ProposePersonaUpdate`; no
  new draft governance path exists.
- Candidate CLI list output exposes only overview-level data: a stable
  parseable ID, coarse field/value, and next-action guidance.
- Candidate CLI show output exposes stable record detail fields, including
  evidence, reason, source, confidence, and observed time.
- `evidence_quote` validation uses NFKC + trim substring checking and drops
  invalid candidates with warning/audit.
- The acceptance test uses only fake-model behavior and temporary workdirs.
- The test asserts no automatic draft creation and no automatic persona file
  modification.
- The release gate switch is converted from fail-fast guard to `Invoke-GoGate`
  only after the skipped scaffold becomes a deterministic passing test.

## Future Gate Integration

After activation:

1. Add the persona acceptance group to the PR release gate.
2. Keep any real-model persona smoke in `release-gate.ps1 -Full` or an explicit
   manual workflow only.
3. Make failure output identify which stage failed:
   - source turn completion;
   - extractor candidate creation;
   - candidate list;
   - candidate show;
   - accidental auto-draft;
   - accidental persona write.

## Review Focus

- A-line changes should remain limited to plans, tests, and gate wiring.
- The scaffold must not imply the persona memory pipeline is already
  implemented.
- The PR gate must not depend on a real model or unimplemented persona commands.
