# Review Handoff: Recursive SDK Boundary Scan

## Change

`TestSDKDoesNotImportLoreInternalPackages` now scans SDK Go files recursively with `filepath.WalkDir` instead of only matching `*.go` in the module root.

## Reason

The SDK currently has only root-level package files, but future subpackages should remain under the same `no obsidian-harness/internal imports` boundary. Recursive scanning prevents subdirectories from bypassing the boundary test.

## Notes

- The test still parses Go imports only; README/CHANGELOG text is ignored.
- Hidden directories, editor config directories, `node_modules`, and `vendor` are skipped.
- The blocked import pattern remains scoped to `obsidian-harness/internal` only.

## Verification

```powershell
Push-Location .\sdk\go\lore
..\..\..\.tools\go\bin\go.exe test ./... -count=1
Pop-Location
.\scripts\verify.ps1
```

Both passed locally.
