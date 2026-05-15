# Review Handoff: SDK README Argument Section Scan

## Change

`TestREADMEArgumentTableMatchesContractArtifact` now parses only the `## Argument Contract` section of `sdk/go/lore/README.md`.

## Reason

The previous parser scanned the entire README for table rows that looked like tool argument rows. That worked today, but future README sections could add similar tables and create false positives. Section-scoped parsing keeps the test focused on the intended contract table.

## Verification

```powershell
Push-Location .\sdk\go\lore
..\..\..\.tools\go\bin\go.exe test ./... -run TestREADMEArgumentTableMatchesContractArtifact -count=1 -v
Pop-Location
.\scripts\verify.ps1
```

Both passed locally.

## Review Focus

- Confirm `readmeSection` stops at the next level-2 heading.
- Confirm the test still fails if the argument table drifts from `docs/contracts/mcp-sdk-tools-v0.json`.
