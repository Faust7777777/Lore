# Review Handoff: Resume Failure Leaves Context Empty

## Change

Resume failure tests now also assert `session.History` and `session.WorkingSet` remain empty.

## Reason

Failed resume paths should not partially inject old context into the active session. Previous assertions checked only `session.Recorder == nil`.

## Verification

```powershell
.\.tools\go\bin\go.exe test ./internal/cli -run 'TestConfigureSessionRecorderResume' -count=1 -v
.\scripts\verify.ps1
```

Both passed locally.

## Review Focus

- Confirm all resume failure paths call `assertSessionContextEmpty`.
- Confirm successful `--resume-id` still restores history.
