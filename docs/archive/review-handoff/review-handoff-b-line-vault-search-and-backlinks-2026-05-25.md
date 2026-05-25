# Review Handoff: B-line Vault Read-API Coverage

Date: 2026-05-25

Owner line: B-line (vault / read-api).

Slice ID: continuation of B-line task 8 family. Architect P2
items under "Config and Vault Infra Need Hardening" in
`docs/handoff-full-project-review-2026-05-23.md`. The earlier
task 8 slice (atomic write durability) closed the write side;
this slice closes two read-side coverage gaps.

## Scope

Two vault read APIs were under-reporting for reasons that became
load-bearing once the operator started using the persona pipeline
and ContextPack to crawl their own notes:

1. `vault.SearchText` returned at most ONE hit per file. The
   scanner had an early `break` after the first match, so
   skimming a long note for a recurring term only surfaced the
   first occurrence. For a 100-line meeting log mentioning a
   keyword on lines 12, 47, and 89, the operator only saw line 12.
2. `vault.FindBacklinks` only matched Obsidian wiki-link syntax
   `[[name]]`. Many of this project's own `docs/` files (and
   notes from external markdown editors) use the standard
   inline-link form `[label](relative/path/to/note.md)`. The
   backlink panel under-reported references and the operator's
   "what links to this doc?" view was incomplete.

This slice closes both gaps without touching the public
signatures or the wiki-link path that callers already depend on.

## Changed files (2)

- `internal/vault/query.go`
  - `SearchText`: removed the in-file early-exit `break`. The
    global `limit` still caps the total result count via
    `fs.SkipAll` across files; only the per-file collapse is
    gone.
  - `FindBacklinks`: refactored the per-line check so wiki-link
    pattern matching runs first, then the new
    `lineHasMarkdownLinkTo` branch fires only when no wiki
    pattern matched. The match becomes a single hit per line as
    before -- deduping at the line level continues to be the
    contract.
  - New helper `lineHasMarkdownLinkTo(line string, suffixes []string) bool`:
    scans the line for `](URL)` tokens, optionally accepts the
    `](<URL>)` angle-bracket form, strips a leading `./`,
    truncates at `#` (anchor), `"` (title), `)`, and whitespace,
    and tests whether the normalized URL matches one of the
    `suffixes` (the target's full normalized path and its
    basename-with-extension).
- `internal/vault/query_test.go`
  - 4 new tests: `TestSearchTextReturnsAllMatchesInFile`,
    `TestSearchTextRespectsGlobalLimitAcrossFiles`,
    `TestFindBacklinksRecognisesMarkdownLinks` (six
    sub-assertions plus a wiki-link regression check),
    `TestFindBacklinksRespectsLimit`.

## Why a hand-rolled URL scan instead of regex

`lineHasMarkdownLinkTo` runs once per scanned line in the backlink
walk -- on a vault with thousands of markdown files, the per-line
hot path is exercised millions of times per `FindBacklinks` call.
A hand-rolled `strings.Index("](")` plus a single forward scan to
the URL terminator is faster and easier to reason about than a
regex with backtracking, and avoids importing the `regexp`
package into a file that doesn't otherwise need it. The terminator
set is exactly the CommonMark inline-link spec: URLs end at the
first unescaped whitespace, `)`, `#` (anchor), or `"` (title).

## Why the basename-with-ext suffix is intentionally NOT a bare
basename

A bare basename match (no extension) would treat `[doc](other.md)`
and `[doc](other.txt)` as both linking to `other.md`, which is
wrong: they are different files. The two suffixes
`lineHasMarkdownLinkTo` checks are:

1. The full normalized target path (`notes/refactor.md`).
2. The basename WITH extension (`refactor.md`).

This is enough to catch every legitimate relative-link form a
markdown author would write -- `./notes/refactor.md`,
`../../notes/refactor.md`, `refactor.md` from a sibling -- without
introducing extension-blind false positives.

## Why search keeps the global limit but drops the per-file cap

The `dedupeHits` helper in `internal/orchestrator/readapi.go` keys
by `path+":"+line` so different lines in the same file survive
dedup, but same-line hits collapse cleanly. The global limit is
still respected: once `len(hits) >= limit`, the walk returns
`fs.SkipAll` and short-circuits across files. The per-file early-
exit was the only thing collapsing useful information; removing
it does not introduce any unbounded growth.

## What was NOT touched

- No public signature change. `SearchText(root, relDir, query, limit) ([]TextHit, error)`
  and `FindBacklinks(root, relDir, targetPath, limit) ([]TextHit, error)`
  are preserved.
- No new dependency. No `regexp` import; everything stays in
  `bufio` + `strings` + `fs`.
- No SDK / CLI / TUI / MCP changes. The orchestrator's
  `VaultSearchText` and `VaultBacklinks` wrappers are unchanged
  and now naturally return the richer results.
- No change to wiki-link recognition, including the `|alias`
  variant `[[name|aliased]]`. The new markdown branch fires only
  when no wiki pattern matched, so wiki-link callers see no diff.
- `FindBacklinks` still returns at most one hit per LINE per
  file. A single line containing both a wiki-link and a markdown
  link to the same target counts as one hit, not two -- deduping
  at line granularity is the long-standing contract.

## Review focus

- `lineHasMarkdownLinkTo` advances `rest = rest[urlStart:]` on
  every iteration so a line with multiple `](URL)` tokens is
  walked exhaustively. Confirm an early-return on the first
  match is still emitted (so the per-line hit count stays at 1)
  -- the function returns `true` as soon as any URL matches a
  suffix, but the OUTER scanner loop in `FindBacklinks` is what
  enforces "one hit per line" via `return nil` inside the
  matched branch. Both behaviors are tested.
- The angle-bracket branch returns `false` if the closing `>` is
  missing rather than scanning to end-of-string. A malformed
  `](<no-close` would otherwise produce an unbounded URL.
- The `wrong-ext.md` case in `TestFindBacklinksRecognisesMarkdownLinks`
  is the most important guard: it pins the contract that
  basename-with-ext must match exactly, not just basename
  without ext.
- `TestSearchTextRespectsGlobalLimitAcrossFiles` proves the walk
  short-circuits across files via `fs.SkipAll` once the cap is
  reached. Without that, removing the in-file `break` could be
  read as "scan everything"; the test pins that the global
  budget is still honored.

## Validation

```powershell
go test ./internal/vault/... -count=1 -v
# All 13 tests PASS (5 pre-existing + 4 task-8 + 4 new)

go test ./internal/vault/... ./internal/orchestrator/... ./internal/console/... ./internal/tools/... ./internal/cli/... ./internal/store/... ./internal/app/... ./internal/config/... ./cmd/... -count=1
# 14 packages, all green
```

Load-bearing verification:

- For SearchText: I reverted just the `break` removal,
  re-ran the new test, and confirmed
  `TestSearchTextReturnsAllMatchesInFile` reports
  "hits = 1, want 3 (one per matching line)". The
  global-limit test reports "hits = 2, want 4 (limit cap)"
  in the reverted state. Both fail in the right place.
- For FindBacklinks: I disabled just the
  `lineHasMarkdownLinkTo` branch, re-ran the new test, and
  confirmed only the two wiki-link cases pass; the four
  markdown-link cases (full-path, basename, relative, angle-
  bracketed) all fail with "missing backlink hit". The
  wiki-link regression continues to pass. The branch is the
  only thing producing the new hits.

## Known limitations / future follow-up

- `lineHasMarkdownLinkTo` does not recognize reference-style
  links (`[label][refid]` with `[refid]: path/to/note.md`
  defined elsewhere in the file). This is the next form an
  external editor might emit; matching it would require a
  pre-pass to collect the definition table, which is out of
  scope for a per-line scan. Not common in this project's docs.
- The basename-with-ext suffix can produce a false positive
  if two different folders contain different files with the
  same name (e.g. `notes/api.md` and `archive/api.md`). The
  full-path suffix takes precedence so any link that names
  the folder is unambiguous, but a bare `[ref](api.md)` from
  a third folder would match BOTH targets if both queried in
  turn. The architect's plan accepts this -- bare basename
  matching is the primary win and the operator can scope the
  query via `relDir` if needed.
- No fuzzy / case-insensitive markdown-link match. Wiki-links
  are case-sensitive in Obsidian by default and the markdown
  branch follows the same convention. URLs are matched
  byte-for-byte after the stripping pass.
- `SearchText` does not yet deduplicate adjacent same-line
  matches (a line containing the query twice still produces
  one hit per line, as before -- this is the same semantics
  as the original `break`-based code, just no longer
  collapsing across the rest of the file). Adding intra-line
  multi-hit support would change the result shape and is
  out of scope.

## Companion to the task 8 vault durability handoff

This slice is a continuation of the B-line task 8 family --
read-side coverage on the same architect P2 list. The earlier
write-side handoff
(`docs/archive/review-handoff/review-handoff-b-line-task8-vault-atomic-durability-2026-05-23.md`)
closed the durability gap on `WriteFileAtomic`; this slice
closes the under-reporting gap on `SearchText` and
`FindBacklinks`. Together they finish the architect's vault
infra review at the read+write-API level. Next natural slice
on the same axis would be vault-side reference-style markdown
link support or a fuzzy-name resolver -- both lower-priority
than what is now in.
