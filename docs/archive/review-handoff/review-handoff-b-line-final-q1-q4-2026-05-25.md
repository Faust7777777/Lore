# Handoff: B-line FINAL Q-1 / Q-2 / Q-3 / Q-4 — 2026-05-25

Audience: next reviewer / next B-line contributor.

Source: REVIEW-2026-05-25-FINAL.txt
(`Documents/xwechat_files/wxid_qaiahup7e1ft12_a4a7/msg/file/2026-05/REVIEW-2026-05-25-FINAL.txt`),
section 3 "代码质量缺陷". Operator instruction: address Q-1 and
Q-2 first; Q-3 / Q-4 hitched to the same slice; Q-5 / Q-6
explicitly deferred to a separate small slice so the cli.go
split does not become an un-reviewable mega-PR.

## TL;DR

| FINAL # | Headline | Commit |
|---|---|---|
| Q-2 | personaCandidateOut named type now used by both `show --json` and `list --json`; `newPersonaCandidateOut` is the single source of truth | `5609a80` |
| Q-3 | `readPersonaLogEntries` helper centralizes the three duplicate `os.ReadFile + line-split` blocks across summary / errors paths | `5609a80` |
| Q-4 | `personaLogEntry` carries `session` and `errMsg` extracted up-front in `parsePersonaLogLine`; JSON emit no longer re-splits raw | `5609a80` |
| Q-1 | `internal/cli/cli.go` 2856 → 1641 lines (-42%); persona surface lifted into 3 new files plus a small flag helper | `bd19935` |

## What Q-2 / Q-3 / Q-4 changed (commit 5609a80)

**Q-2 — DTO unification.** Before this slice
`emitPersonaCandidateDetailJSON` declared an anonymous JSON
struct inline while `emitPersonaCandidateListJSON` used the
named `personaCandidateOut`. Field set drift was a real risk:
adding a column to one path did not force the other. Now both
paths construct `personaCandidateOut` via
`newPersonaCandidateOut(record persona.PersonaCandidateRecord)`,
so `lore persona candidates show --json | jq` and
`lore persona candidates list --json | jq '.candidates[]'`
return identical shapes. Behavior unchanged: the JSON tags and
formatting (UTC + RFC3339Nano) carry over verbatim.

**Q-3 — helper centralization.** Three sites read
`persona-extract.log`:
1. `renderPersonaSummary` (plus `emitPersonaSummaryJSON`)
2. `renderPersonaExtractErrors`
3. `emitPersonaExtractErrorsJSON`

Each had its own `os.ReadFile(logPath)` + `strings.Split` +
trimming dance. The new helper
`readPersonaLogEntries(logPath string) ([]personaLogEntry, bool, error)`
returns the parsed entries, a `exists` boolean to distinguish
"no file yet" from "real error", and the error itself. All four
sites now route through it; the three previous duplicates are
gone.

**Q-4 — up-front parsing.** `personaLogEntry` previously held
`raw, ts, stage, parsed`. The errors-JSON path then re-split
`entry.raw` to recover `session=<x>` and `error=<quoted>` fields.
Now `parsePersonaLogLine` extracts those at parse time, stores
them as `session string` and `errMsg string`, and routes them
verbatim to the JSON emit. `errMsg` is unquoted via `unquoteSafe`
because the writer formats errors with `%q`. The change is
internal to the parser; tab-delimited rendering is unchanged.

### Files changed (5609a80)
- `internal/cli/cli.go` (one file; the split was deferred to
  Q-1 so each FINAL item lives in its own commit)

### Load-bearing test rationale (5609a80)
- Existing persona JSON tests assert against JSON tag keys, not
  Go type names, so collapsing the anonymous struct into the
  named type is a no-op at the assertion surface.
- `internal/cli/persona_summary_test.go`,
  `persona_errors_test.go`, and `persona_command_test.go` cover
  every helper site touched. All passed
  (`ok obsidian-harness/internal/cli 7.808s` after
  Q-2/Q-3/Q-4 prior to the split commit).

## What Q-1 changed (commit bd19935)

`internal/cli/cli.go` was 2856 lines and growing. The FINAL
review proposed an explicit four-file split. Implemented as
proposed:

| New file | Lines | Symbols |
|---|---|---|
| `internal/cli/persona_summary.go` | 245 | `runPersonaSummaryCommand`, `emitPersonaSummaryJSON`, `renderPersonaSummary` |
| `internal/cli/persona_candidates.go` | 508 | `runPersonaCommand`, the four `parsePersona*Flags` helpers, `renderPersonaDraftErrorHint`, `renderPersonaRecoverErrorHint`, `personaCandidateOut`, `newPersonaCandidateOut`, the two emit-JSON variants, `renderPersonaCandidateList`, `renderPersonaCandidateDetail`, `renderPersonaCandidateActionResult` |
| `internal/cli/persona_errors.go` | 348 | `runPersonaErrorsCommand`, `parsePersonaErrorsFlags`, `personaLogEntry`, `parsePersonaLogLine`, `readPersonaLogEntries`, `unquoteSafe`, `emitPersonaExtractErrorsJSON`, `renderPersonaExtractErrors` |
| `internal/cli/flag_utils.go` | 75 | `reorderFlagsBeforePositionals`, `isBoolFlag` |

`cli.go` retains entry-point dispatch (`Run`, console / TUI
shells, draft / findings / usage / daemon / process-sink /
models / sessions / codex JSONL paths) and the cross-cutting
helpers (`defaultWorkDir`, `closeRuntime`, `clipOneLine`,
`renderSessionSnapshot` etc.). Imports were trimmed: cli.go no
longer imports `encoding/json`, `errors`, or
`obsidian-harness/internal/persona` — each lives in its own
destination file.

Pure mechanical move; no symbols renamed, no signatures changed,
nothing exported as a side-effect (every function that was
unexported before stays unexported; Go resolves cross-file
references inside the same package directly).

### Files changed (bd19935)
- `internal/cli/cli.go` (-1198 lines)
- `internal/cli/persona_summary.go` (new, 245 lines)
- `internal/cli/persona_candidates.go` (new, 508 lines)
- `internal/cli/persona_errors.go` (new, 348 lines)
- `internal/cli/flag_utils.go` (new, 75 lines)

Net diff: +1176 / -1198 = -22 lines (a few duplicated comment
headers were dropped in transit; no behavior change).

### Load-bearing test rationale (bd19935)
- The split is mechanical: every test in
  `internal/cli/persona_*_test.go` and
  `cli_persona_command_test.go` that passed against the old
  monolithic file must pass against the split files unchanged,
  because no symbol moved across package boundaries.
- Confirmed: `go test ./... -count=1` shows all 24 packages
  green, with `internal/cli 7.597s` and `internal/app 10.204s`
  the dominant timings. No test was modified or added in
  bd19935 — that is the assertion: "pure refactor" means the
  pre-existing test surface validates the post-split layout.

## Boundary statement (B-line scope only)

This slice did NOT touch:

- A-line vault symlink S-1 (`internal/vault/query.go`,
  `internal/orchestrator/readapi.go`, `internal/orchestrator/harness.go`).
  The operator's parallel S-1 fix is visible in the working
  tree but is excluded from both B-line commits via pathspec.
- A-line runtimeDocs S-2 isolation
  (`internal/operatoragent/model.go`).
- A-line sessionlog S-3 lock-order documentation
  (`internal/sessionlog/index.go`, `writer.go`).
- TUI Q-8 context-background audit (`internal/tui/...`).
- SDK frame-size limits (any `internal/mcp` / `internal/sdk`
  surface).

Each of those has its own architect-driven handoff under
`docs/archive/review-handoff/review-handoff-codex53-a-line-*-2026-05-25.md`.

## What is intentionally NOT done in this slice

- **Q-5 errors.Is in renderPersonaRecoverErrorHint.** Today the
  fallback branch uses `strings.Contains(err.Error(), "not found")`.
  Replacing that with `errors.Is(err, store.ErrNotFound)`
  requires confirming the exact sentinel error path through
  `runtime.RecoverPersonaCandidateLink` and
  `harness.GetDraft`. Deferred to the Q-5/Q-6 slice.
- **Q-6 RFC3339 vs RFC3339Nano JSON timestamp consistency.**
  Persona summary emits `RFC3339`; persona candidates emit
  `RFC3339Nano`. Operator preference (truncate to seconds vs
  preserve nanoseconds) needs a single decision; deferred.
- **Q-7..Q-16** (the rest of the FINAL P2/P3 list). Out of
  scope for this slice; handled when their owning surface
  changes for unrelated reasons.

## Push readiness

Two B-line commits added on `main`:

```
bd19935 refactor(cli): split persona surface out of cli.go
5609a80 refactor(cli): unify persona DTO and centralize persona log parsing
```

Working tree still carries A-line edits (S-1/S-2/S-3 plus
README/release-gate touches and four codex53 a-line handoff
docs) — those are NOT part of these two commits. The push
decision belongs to the human operator; this document only
records that the B-line work is bench-clean and does not
introduce a push blocker.

## Reading order for a new contributor

1. This document.
2. `REVIEW-2026-05-25-FINAL.txt` section 3 "代码质量缺陷"
   (Q-1 through Q-6).
3. The two new commits' diffs:
   `git show 5609a80` then `git show bd19935`.
4. The four new files in `internal/cli/`:
   `persona_candidates.go` (largest, the dispatch),
   `persona_errors.go` (log parser is here),
   `persona_summary.go` (dashboard),
   `flag_utils.go` (the `reorderFlagsBeforePositionals` helper
   used by every persona subcommand).
5. The persona test files in the same package — they are the
   regression net.
