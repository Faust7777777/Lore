# Handoff: B-line FINAL Q-12 — 2026-05-25

Audience: next reviewer / next B-line contributor.

Source: REVIEW-2026-05-25-FINAL.txt section 3 "代码质量缺陷",
item Q-12 (P3): "`reorderFlagsBeforePositionals` 缺单元测试".

## TL;DR

| FINAL # | Headline | Commit |
|---|---|---|
| Q-12 | `internal/cli/flag_utils_test.go` adds 14-case table-driven coverage for `reorderFlagsBeforePositionals` plus a flag.Parse-after-reorder smoke test and a `TestIsBoolFlag` companion case | `e5f7c3e` |

This is a pure additive test slice — the helper itself is
unchanged.

## What Q-12 changed

`reorderFlagsBeforePositionals` shipped to `main` in commit
`9c828eb` (FINAL section 1.G "Flag 位置灵活化"). It is what
allows operators to write the modern CLI shape

```
lore persona candidates show pc-1 --json
```

instead of the stdlib-flag-required

```
lore persona candidates show --json pc-1
```

The helper was relocated into its own `flag_utils.go` during the
Q-1 cli.go split (commit `bd19935`). At that point it had no
direct unit test — only indirect coverage via persona command
end-to-end tests, which exercise the happy paths but do not
probe edge cases like the `--` terminator, the `--name=value`
form, lone `"-"` as a positional, or value flags at the end of
args without a value.

The new test file adds:

1. **`TestReorderFlagsBeforePositionals`** — 14-case table:
   - empty input
   - all positionals
   - all flags
   - bool flag after positional
   - value flag after positional pulls its value
   - interleaved flags and positionals
   - `--name=value` form does not consume next arg
   - `--` terminator freezes following args as positionals
   - single dash is a positional, not a flag
   - unknown flag passes through to the flag side
   - value flag at end of args without a value does not crash
   - short single-dash bool flag (`-json`)
   - multiple bool flags after positional
   - int value flag pulls its value

2. **`TestReorderFlagsBeforePositionalsParsesAfterReorder`** —
   end-to-end smoke: feeds `["pc-1", "--json", "--workdir",
   "/tmp/wd"]` through reorder, then through `fs.Parse`, and
   asserts both flag values and the trailing positional. This is
   the real contract — the helper exists to unblock `flag.Parse`,
   so testing reorder + parse together catches any regression
   that would slip past the unit table.

3. **`TestIsBoolFlag`** — minimal sanity for the small companion
   helper that distinguishes "consumes next arg" from "doesn't".

## Files changed (e5f7c3e)

- `internal/cli/flag_utils_test.go` (new, 165 lines, additive)

No production code touched; no behavior change.

## Load-bearing test rationale

This *is* the test rationale — the slice exists to add tests.

What "load-bearing" means here:
- The 14 sub-cases are the contract surface I want CI to defend.
  Future edits to the reorder helper now have a clear regression
  net for every documented edge case.
- The reorder+parse smoke is the *behavioral* assertion: even if
  the table grows or shrinks, the integration test pins the
  feature's user-visible contract ("flag-after-positional must
  work").
- Verified clean: `go test ./internal/cli/... -count=1` →
  `ok obsidian-harness/internal/cli 6.572s` after the new file
  lands. All pre-existing tests still pass.

## Boundary statement (B-line scope only)

This slice did NOT touch:

- A-line vault symlink S-1.
- A-line runtimeDocs S-2 isolation.
- A-line sessionlog S-3 lock-order documentation.
- TUI Q-8 context-background audit.
- SDK frame-size limits.

This slice also did NOT touch any production code — it is a
pure test addition under `internal/cli/`.

## What is intentionally NOT done in this slice

- **Q-7..Q-11, Q-13..Q-16** (the rest of the FINAL P2/P3 list).
  Each of those touches a different surface (Q-7 prompt
  truncation, Q-8 TUI context, Q-9 vault writeFileSync, Q-10
  finding sentinel, Q-11 finding CAS hardcoding, Q-13 sessionlog
  normalize, Q-14/Q-15 vault link parsing, Q-16 sessionlog
  upsert lock duration). Out of scope for B-line CLI work.
- **Behavioral changes to `reorderFlagsBeforePositionals`.** The
  helper handles every edge case the test asserts; nothing in the
  table revealed a bug. If a future case (e.g. negative numbers
  starting with `-` that should be positional values, like
  `--limit -5`) needs to be handled, it should be a separate
  decided slice with the test case landing alongside the
  production change.

## Push readiness

One B-line commit added on `main`:

```
e5f7c3e test(cli): cover reorderFlagsBeforePositionals with table-driven cases
```

Working tree still carries A-line edits (S-1/S-2/S-3 plus
README/release-gate touches and four codex53 a-line handoff
docs) — those are NOT part of this commit. The push decision
belongs to the human operator.

## Reading order for a new contributor

1. This document.
2. `internal/cli/flag_utils.go` — the helper under test.
3. `internal/cli/flag_utils_test.go` — the table.
4. `REVIEW-2026-05-25-FINAL.txt` section 3 item Q-12.
5. The Q-1 sibling handoff
   `review-handoff-b-line-final-q1-q4-2026-05-25.md` for context
   on why this helper lives in its own file.

## FINAL Q-1..Q-12 closure (B-line surface)

| # | Status | Commit |
|---|---|---|
| Q-1 | landed | `bd19935` |
| Q-2 | landed | `5609a80` |
| Q-3 | landed | `5609a80` |
| Q-4 | landed | `5609a80` |
| Q-5 | landed | `f7aaeb3` |
| Q-6 | landed | `f7aaeb3` |
| Q-7 | deferred (prompt surface, not B-line CLI) | — |
| Q-8 | deferred (A-line TUI per architect) | — |
| Q-9 | deferred (vault, not B-line) | — |
| Q-10 | deferred (model.finding sentinel) | — |
| Q-11 | deferred (sqlite finding CAS) | — |
| Q-12 | landed | `e5f7c3e` |

Q-12 is the only Q-7..Q-16 item that lived squarely in B-line
CLI scope. The remaining items each belong to a surface owned
by a different working line.
