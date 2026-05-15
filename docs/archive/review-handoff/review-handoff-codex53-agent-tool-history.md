# Review Handoff: Agent Tool Result History Ordering

## Change

Extended `TestModelAgentRespondRunsNativeToolCallThenFinal` to assert the native tool-call loop history passed to the second model request.

## Behavior Locked

After a native tool call:

- The synthetic tool call is appended as an `assistant` message.
- The tool result is appended after it as a `user` message.
- The tool result content includes `Tool result for <tool>:` and the actual tool output.

## Reason

This locks the P0 agent reliability requirement that tool result injection order and roles remain stable. The test catches regressions where native tool calling stops giving the model a coherent tool-call/result history.

## Verification

```powershell
.\.tools\go\bin\go.exe test ./internal/operatoragent -run TestModelAgentRespondRunsNativeToolCallThenFinal -count=1 -v
.\scripts\verify.ps1
```

Both passed locally.

## Review Focus

- Confirm role ordering matches the current prompt-loop contract.
- Confirm this does not change production behavior.
