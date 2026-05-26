# Review Handoff: A-Line S-1 Vault Symlink Boundary

Date: 2026-05-25

## FINAL Finding

Addresses `REVIEW-2026-05-25-FINAL.txt` finding:

Canonical finding ID: `S-1` / `Symlink path traversal` / `P0`.

- `S-1: Symlink 路径穿越 (P0)`

## Scope

Fixes vault path resolution so vault reads, list/walk/search/backlinks, and
governed writes do not follow symlink/junction paths to files outside the
configured vault root.

## Modified Files

- `internal/vault/query.go`
- `internal/vault/query_test.go`
- `internal/orchestrator/readapi.go`
- `internal/orchestrator/readapi_test.go`
- `internal/orchestrator/harness.go`
- `internal/orchestrator/harness_test.go`
- `scripts/release-gate.ps1`
- `README.md`

## Implementation Notes

- Read path:
  - `ResolveExistingUnderRoot` performs lexical root containment, rejects
    unsafe existing parent components, then checks the symlink-resolved target
    remains under the symlink-resolved root.
  - `ReadRelativeWithHash` now uses that read resolver.
- Write path:
  - `ResolveWriteUnderRoot` handles new targets by checking the nearest
    existing parent, so missing final files do not force the unsafe
    `EvalSymlinks(final)` pattern called out in the review.
  - `WriteRelativeAtomic` wraps `WriteFileAtomic` with the write resolver.
  - `ApplyDraft`, `WriteLowRiskNote`, bootstrap, and proposal base-version
    reads now route through relative vault helpers.
- Walk/list path:
  - `ListEntries` skips symlink/junction-like non-regular children that resolve
    outside the vault.
  - `walkMarkdownFiles` validates resolved paths during traversal and skips
    symlink entries.
  - `SearchText`, `FindBacklinks`, and `WalkMarkdownPaths` consume that safe
    traversal path.

## Load-Bearing Tests

- `TestReadRelativeWithHashRejectsSymlinkFileOutsideRoot`
  - Covers direct symlink file read.
  - Skips when the OS account cannot create file symlinks.
- `TestWalkListSearchAndBacklinksSkipFileSymlinkOutsideRoot`
  - Covers list/walk/search/backlink behavior for external file symlinks.
  - Skips when the OS account cannot create file symlinks.
- `TestWalkListSearchAndBacklinksSkipDirectorySymlinkOutsideRoot`
  - Covers directory symlink/junction traversal; on Windows it falls back to
    `mklink /J` so this is load-bearing even without symlink privilege.
- `TestVaultReadRejectsSymlinkFileOutsideRoot`
  - Covers orchestrator `VaultRead` against a direct external symlink file.
- `TestVaultReadRejectsParentSymlinkOutsideRoot`
  - Covers orchestrator `VaultRead` through an external parent symlink/junction.
- `TestWriteLowRiskNoteRejectsParentSymlinkOutsideRoot`
  - Covers direct low-risk note write through an external parent symlink.
- `TestApplyDraftRejectsParentSymlinkOutsideRoot`
  - Covers approved markdown-note draft apply through an external parent
    symlink/junction.

Release gate additions:

```powershell
go test ./internal/vault -run "Test(ReadRelativeWithHash(BlocksTraversal|RejectsSymlinkFileOutsideRoot)|WalkListSearchAndBacklinksSkip(FileSymlink|DirectorySymlink)OutsideRoot)$" -count=1 -v
go test ./internal/orchestrator -run "Test(VaultReadRejects(SymlinkFile|ParentSymlink)OutsideRoot|WriteLowRiskNoteRejectsParentSymlinkOutsideRoot|ApplyDraftRejectsParentSymlinkOutsideRoot)$" -count=1 -v
```

## Explicit Non-Scope

- Did not change MCP exposed tool surface.
- Did not change persona candidate state machine or draft governance semantics.
- Did not change TUI rendering or interaction behavior.
- Did not change generic `WriteFileAtomic` semantics for non-vault callers such
  as sessionlog; the vault boundary is enforced by vault-relative helpers.

## Reviewer Focus

- Confirm new-file writes check the nearest existing parent instead of
  requiring `EvalSymlinks` on a non-existent final file.
- Confirm directory junction tests run on Windows without symlink privilege.
- Confirm `ApplyDraft` and `WriteLowRiskNote` cannot write outside the vault
  through a parent symlink/junction.
