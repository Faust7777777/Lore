# Review Handoff: B-line Task 8 (extra) — Vault Atomic Write Durability

Date: 2026-05-23

Owner line: B-line (vault / infra).

Slice ID: B-line task 8, extra (taken on top of the original
ordered first-tasks 1-6; corresponds to architect P2 from
`docs/handoff-full-project-review-2026-05-23.md` section 4
"Config and Vault Infra Need Hardening" +
`hermes-workspace/lore-infra-handover.md`).

## Scope

`vault.WriteFileAtomic` was the durability load-bearer for
`ApplyDraft`, `WriteLowRiskNote`, and daemon scaffolding -- every
managed-core write went through it. Before this slice it:

- Used `os.WriteFile` for the temp file, which only flushes to
  the kernel page cache. Power loss between the rename and the
  next periodic sync could leak the bytes.
- Used `os.Rename` directly with no cross-device fallback.
  Workdirs on bind mounts, Docker volumes, or split Windows
  drive letters tripped `EXDEV` with no recovery; the caller
  saw an opaque rename error.
- Did not sync the parent directory after the rename, so the
  rename's metadata could outlive a power loss without being
  durable.

This slice closes all three gaps while preserving the existing
public signature and return contract (still `(sha256, error)`).

## Changed files (2)

- `internal/vault/atomic.go` — `WriteFileAtomic` rewritten on
  top of three new helpers (`writeFileSync`, `copyFileSync`,
  `syncParentDirBestEffort`) + an `isCrossDeviceError`
  predicate. Imports gained `errors`, `io`, `syscall`.
- `internal/vault/atomic_test.go` — 6 new tests + the pre-
  existing happy path. Covers overwrite, default suffix,
  custom suffix cleanup, empty payload, 1 MiB round-trip,
  EXDEV predicate routing, and the parent-is-a-file failure
  branch.

## The write pipeline

```
EnsureParentDir(path)
        |
        v
writeFileSync(path + tempSuffix, data)
        |   (OpenFile O_WRONLY|O_CREATE|O_TRUNC + Write + Sync + Close)
        v
os.Rename(temp -> path)
        |
        |  error?
        |   |
        |   +-- isCrossDeviceError(err) ?
        |   |     yes -> copyFileSync(temp -> temp+".xdev")
        |   |            Rename(temp+".xdev" -> path)
        |   |            Remove(temp)
        |   |     no  -> Remove(temp); return err
        |   |
        v   v
syncParentDirBestEffort(path)
        |
        v
return ComputeSHA256(data), nil
```

Cleanup on every failure branch removes both the original temp
and the cross-device fallback temp so the daemon's vault scan
never sees a half-finished file masquerading as a backup.

## Why fsync the temp file (and the parent dir, best-effort)

The previous `os.WriteFile` path was crash-safe **only** under
the assumption that the operating system's periodic sync would
catch up before a crash. For governance writes (persona update,
managed-core scaffold, progress index, weekly executions) that
is too weak: the operator's audit trail could record a draft
as Applied while the on-disk content was lost.

`File.Sync` after the write returns only after the kernel has
acknowledged the bytes are on stable storage. The parent
directory `Sync` (best-effort) ensures the rename's directory
entry is also durable, so recovery sees the new path rather
than the old one.

## Why errors.Is + syscall.EXDEV instead of GOOS-specific code

`syscall.Errno` implements `Is(target error) error` on every
supported platform, and `*os.LinkError.Err` (the wrapper
`os.Rename` returns) is a `syscall.Errno`. So
`errors.Is(err, syscall.EXDEV)` routes correctly without any
`runtime.GOOS` branching:

- POSIX: `EXDEV = 18` ("invalid cross-device link").
- Windows: `ERROR_NOT_SAME_DEVICE = 17` ("the system cannot
  move the file to a different disk drive"), surfaces as
  `syscall.EXDEV` via the syscall mapping table.

The `isCrossDeviceError` predicate is a one-liner; the
`atomic.go` file stays GOOS-agnostic.

## What was NOT touched

- No public signature change. `WriteFileAtomic(path, data,
  tempSuffix) (string, error)` is preserved; callers see no
  diff.
- No new dependency. `errors`, `io`, `syscall` are stdlib.
- `ComputeSHA256`, `EnsureParentDir`, `ReadFileWithHash`
  unchanged.
- No app / store / persona / orchestrator / CLI / MCP / TUI /
  SDK changes.

## Review focus

- Confirm `writeFileSync` calls `Sync` BEFORE `Close`. Inverting
  that order would silently drop the durability guarantee on
  some filesystems.
- Confirm the cross-device fallback removes BOTH temps
  (original + `.xdev`) on every branch (success, copy fail,
  second rename fail). A leaked temp would survive into the
  next vault scan.
- Confirm `syncParentDirBestEffort` silently ignores errors.
  Returning the error would force callers to handle a
  non-fatal degradation (data is already durable; only the
  rename metadata is at stake).
- Confirm `isCrossDeviceError(nil) == false`. Some predicates
  collapse nil into true via a generic check; this one must
  not, or every legal-rename path would take the fallback.

## Validation

```powershell
go test ./internal/vault -count=1 -v
# 7 tests PASS (6 new + 1 existing)

go test ./internal/store/... ./internal/app/... ./internal/cli/... ./internal/console/... ./internal/orchestrator/... ./internal/config/... ./internal/vault/... ./cmd/... -count=1
# all 10 packages green
```

The 1 MiB round-trip test proves the buffer is flushed (a
broken `Sync` would leave the readback short or zeroed). The
parent-is-a-file branch proves no temp leaks when the initial
OpenFile fails.

## Known limitations / future follow-up

- The cross-device branch is not exercised by an in-process
  test: reproducing `EXDEV` would require mounting two
  filesystems, which is out of scope for a unit test. The
  predicate's routing is covered by
  `TestIsCrossDeviceErrorMatchesEXDEV`; the body is
  straightforward (copy + sync + rename + cleanup). A future
  infra test that runs against a real bind-mount setup should
  add the round-trip case.
- `syncParentDirBestEffort` silently skips errors. On
  filesystems where directory sync fails (Windows, some FUSE
  drivers) the rename metadata might temporarily be at risk
  across a power loss. The architect's primary concern was
  data loss; this slice addresses that. Tightening the dir
  sync to a hard requirement would need per-platform decision
  logic.
- No truncate-detection on read-back. If a future filesystem
  truncates `path` between the rename and the next read, the
  caller's sha256 will not match `ReadFileWithHash`'s. Out of
  scope for this slice; the ApplyDraft baseVersion check
  catches the next-step inconsistency.
- A `WriteFileAtomicContext` variant that respects a caller
  `context.Context` would let `ApplyDraft` abort the write
  on operator cancellation. A-line is currently threading
  context through console / TUI / operatoragent; the vault
  write call site could pick up the same pattern in a later
  slice.

## Companion to first-tasks completion handoff

This slice was not in the original B-line first-tasks list (1-6
shipped earlier today). It is recorded as "task 8 extra" so
the next reviewer can see the work landed against the architect's
P2 list rather than as ad-hoc polish. The first-tasks
completion handoff at
`docs/handoff-b-line-first-tasks-completion-2026-05-23.md`
mentions vault durability as a candidate for the next slice;
this slice fulfils that pointer.
