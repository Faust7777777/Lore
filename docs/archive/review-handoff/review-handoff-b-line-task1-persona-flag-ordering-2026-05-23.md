# Review Handoff: B-line Task 1 — Persona CLI Flag Ordering Fix

Date: 2026-05-23

Owner line: B-line (CLI / persona).

Slice ID: B-line task 1 (per `docs/handoff-full-project-review-2026-05-23.md`
section 4 / 8 "B Line first tasks").

## Scope

P1 finding from the full project review: `lore persona candidates
show <id> --json`, `... draft <id> --retry-rejected`, and
`... recover <id> --link <draft-id>` silently fell back to the
no-flag path because Go's stdlib `flag.FlagSet.Parse` stops at
the first positional argument. Docs and operator intuition
expected both `--flag <id>` and `<id> --flag` to work.

## Changed files (2)

- `internal/cli/cli.go` — new `reorderFlagsBeforePositionals`
  helper + `isBoolFlag` helper next to `defaultWorkDir`. Every
  persona parser invokes the helper before `flagSet.Parse`:
  - `parsePersonaListFlags`
  - `parsePersonaShowDismissFlags`
  - `parsePersonaDraftFlags`
  - `parsePersonaRecoverFlags`
  - `parsePersonaErrorsFlags`
  - inline summary parsing in `runPersonaSummaryCommand`
- `internal/cli/persona_command_test.go` — four new
  `*WorksWithIDFirst` tests pin the previously-broken form on
  show / draft --retry-rejected / recover --link /
  recover --force-dismiss.

## Helper design

```go
func reorderFlagsBeforePositionals(args []string, flagSet *flag.FlagSet) []string {
    var flagArgs, positionals []string
    for i := 0; i < len(args); i++ {
        arg := args[i]
        if arg == "--" {
            positionals = append(positionals, args[i+1:]...)
            break
        }
        if !strings.HasPrefix(arg, "-") || arg == "-" {
            positionals = append(positionals, arg)
            continue
        }
        name := strings.TrimLeft(arg, "-")
        eq := strings.IndexByte(name, '=')
        explicit := eq >= 0
        if explicit {
            name = name[:eq]
        }
        f := flagSet.Lookup(name)
        if f == nil {
            flagArgs = append(flagArgs, arg)
            continue
        }
        flagArgs = append(flagArgs, arg)
        if explicit { continue }
        if isBoolFlag(f) { continue }
        if i+1 < len(args) {
            i++
            flagArgs = append(flagArgs, args[i])
        }
    }
    return append(flagArgs, positionals...)
}
```

Notes:

- Uses `flagSet.Lookup(name)` to distinguish flag-shaped tokens
  from unknown flags. Unknown flag tokens pass through to the
  front so `flag.Parse` reports them in its usual format.
- `IsBoolFlag()` (the same interface stdlib `flag` uses internally
  to short-circuit value consumption) decides whether to grab
  `args[i+1]` as the flag's value.
- The `--` terminator pushes everything after it to positionals
  verbatim, matching standard CLI convention.

This is intentionally lighter than swapping to `spf13/pflag`: no
new dependency, no behavior change for the flag-first form,
single-file scope, easy to audit.

## What was NOT touched

- No app / store / persona / MCP / TUI changes.
- No CI / release-gate change.
- No persona prompt / parser change.
- No new sentinel errors.
- `flag.Parse` itself is unchanged; only its input order changes.

## Review focus

- Confirm every `parsePersona*Flags` site invokes the reorder
  helper exactly once, after all flag definitions and before
  `flagSet.Parse`. Inverting the order (Parse first, then reorder)
  is a no-op and silently regresses.
- Confirm `isBoolFlag` correctly identifies all current persona
  flags: `--json`, `--retry-rejected`, `--force-dismiss`,
  `--fail-on-orphan` are bool; everything else (`--workdir`,
  `--limit`, `--state`, `--link`, `--tail`, `--stage`, `--since`)
  is value. The implementation derives bool-ness from the
  optional `IsBoolFlag() bool` interface that stdlib `flag` itself
  emits on bool flag values, so this stays correct as new flags
  are added.
- Confirm the `--` terminator and unknown-flag branches behave
  reasonably. The test surface focuses on the previously-broken
  forms; explicit `--` handling is a defensive convention rather
  than something a persona test currently exercises.

## Validation

```powershell
go test ./internal/cli -count=1 -run "TestRunPersonaCandidates.*WorksWithIDFirst" -v
# 4 tests PASS

go test ./internal/store/... ./internal/app/... ./internal/cli/... ./internal/console/... ./cmd/... -count=1
# all green
```

Load-bearing verified by temporarily removing the reorder hook
in `parsePersonaShowDismissFlags` and confirming
`TestRunPersonaCandidatesShowJSONWorksWithIDFirst` fails on
"expected JSON output (starts with '{')" because the show output
reverts to the human detail block, then restoring.

## Known limitations / future follow-up

- The helper is persona-scoped today by call-site. Other CLI
  subgroups (`lore draft`, `lore findings`, `lore status`) keep
  the historical Go flag.Parse behavior. If a reviewer wants the
  same UX for those surfaces, the helper can move to a shared
  package; no contract change required.
- The helper does not understand `-flag value` (single-dash)
  styles because the persona CLI uses `--flag` throughout. Adding
  single-dash support is a one-line change if a future surface
  needs it (the helper already strips both leading dashes).
- This is a fix, not a feature. Cookbook and pipeline overview
  docs were updated to note both flag orderings work; the
  full-project-review handoff (which describes the pre-fix
  defect) intentionally stays as the historical baseline so the
  fix's commit message provides the resolution context.
