# Review Handoff: B-P10 List Shows DRAFT Column

Date: 2026-05-22

Owner line: B-line (persona memory candidate pipeline, business / data)

Commit: `ccf1940 feat(cli): list adds DRAFT column showing linked DraftID`

## Scope

Closes the visibility gap B-P7 left: `lore persona candidates show
<id>` displays the linked DraftID, but `list --state drafted`
required running `show` on every row to trace candidates → drafts.
Operators inspecting a workdir's drafted backlog can now see the
relationship at a glance.

## Changed files (2)

- `internal/cli/cli.go` — `renderPersonaCandidateList` header gains
  a seventh column DRAFT; each data row prints the linked DraftID
  (when non-empty) or "-" otherwise.
- `internal/cli/persona_command_test.go` — 2 new tests covering the
  drafted-row case (real DraftID shown) and the open-row case
  (dash placeholder maintains schema).

## Design note: dash placeholder

Empty DraftID (Open / Dismissed / partial orphan) renders as `-`
rather than blank. The reason is downstream `awk -F'\t' '{print
$7}'` / `cut -f7` consumers expect a stable 7-column schema; an
empty cell would collapse adjacent tabs and shift offsets.
`-` is the conventional Unix sentinel for "not applicable" in
tabular CLI output and is self-documenting.

## What was NOT touched

- No `internal/app/*`, `internal/store/*`, `internal/persona/*`
  changes — this is a pure CLI rendering polish.
- No new flags or commands.
- No header / footer text change besides the new column.
- No MCP, TUI, or CI changes.

## Review focus

- Confirm 7-column schema stability is correct (no test should
  break because a 6-column expectation now sees 7). The
  acceptance-gate test in `cmd/obsidian-harness/main_test.go`
  asserts string substrings only, not column count, so it stayed
  green; verified by full sanity in the commit.
- Confirm the dash sentinel is acceptable for the
  in-progress-but-empty case (Open candidate during initial
  observation). Alternative `(none)` was considered and rejected
  for cell-width reasons.

## Validation

```powershell
go test ./internal/cli -count=1 -run "TestRunPersonaCandidatesListShowsDraftColumnForLinkedCandidate|TestRunPersonaCandidatesListDraftColumnDashForOpen" -v
# 2 tests PASS

go test ./internal/cli/... ./cmd/obsidian-harness/... -count=1
# all green (acceptance gate substring asserts unaffected)
```

Load-bearing verified by temporarily reverting the DRAFT column
addition and confirming both new tests fail (missing DRAFT header +
missing `\t-\n` suffix), then restoring.

## Known limitations / future follow-up

- No column for candidate `State` because list is already filtered
  by state; the column would be redundant. If a future
  `--state all` flag lands, that flag should reintroduce STATE
  alongside DRAFT.
- No truncation on DraftID. Draft IDs are `draft-<UnixNano>` which
  is already short (~20 chars), but a future longer ID format
  would need `clipOneLine` like PROPOSED uses.
