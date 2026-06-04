# B-line review: failure-handling semantics across the framework (2026-06-04)

Audience: B-line owner + reviewer. Origin: the "继续 review 整个项目框架"
tick of the `/loop`, after this session's targeted fixes. Where the
config-enforcement note (`review-b-line-config-enforcement-2026-06-04.md`)
found *gaps*, this pass audited the **failure-handling consistency** of the
paths I touched and the core paths I did not — and found the framework
follows one coherent invariant. The point of writing it down: so the
reviewer can trust the un-changed core without re-auditing it, and so future
changes preserve the rule.

## The invariant

The harness splits every operation into one of two failure regimes:

1. **Read / status / aggregation paths are best-effort.** A failure in a
   *secondary, informational* sub-operation must never fail the primary
   view. The operator's "what needs my attention" must still render.

2. **Mutating paths fail hard — and when an external side effect already
   happened, they leave a visible governance record** rather than failing
   silently or rolling back something they cannot roll back (a vault file
   on disk).

Audit writes sit underneath both: they are **best-effort but not silent** —
on failure they degrade health, which the operator sees in `lore status`.

## Evidence the invariant holds (audited this pass — all SOUND)

### Read / aggregation = best-effort

| Path | File:line | Behaviour |
|---|---|---|
| `OperatorQueue` usage glance | `internal/app/operator_queue.go:64-67` | `usage, _ := r.SummarizeUsage(now)` — a usage-store hiccup zeroes the glance; pending drafts/findings/candidates still surface. **Fixed + tested this branch** (`e948f90`). |
| `ManagedStatus` core docs | `internal/orchestrator/readapi.go:15-44` | read-only; computes `ready` from per-doc existence; never errors on a missing managed doc. |
| `coreDocStatus` | `internal/orchestrator/readapi.go:432-439` | `Exists: err == nil` — a stat failure degrades to "absent", never propagates. |
| `ProcessSinkDay` report | `internal/app/processsink.go:29-34` | `ErrNotFound` → `Report=nil` (a day with no rollup is valid); only a *real* store error propagates. |

### Mutation = fail-hard + governance record on partial side effect

| Path | File:line | Behaviour |
|---|---|---|
| `ApplyDraft` (vault write then state flip) | `internal/orchestrator/harness.go:482-549` | If the vault write succeeds but `State=applied` won't persist, emits a **critical** `FindingGovernanceReviewNeeded` (`recordApplyStateFailFinding`) and returns a reconcile-instructing error. Handles the double-failure (finding also fails) with a distinct message. Already hardened — architect P0 + reviewer round-2. |
| `updateFindingState` (app) | `internal/app/findings.go` | On audit-append failure returns the **committed** finding + a wrapped error (`finding %s moved to %s but the audit record failed`), not an empty struct — the state change is real, so don't pretend it didn't happen. **Fixed this branch** (`b1cd066`). |
| `transitionDraftState` (approve/reject/revision) | `internal/orchestrator/harness.go:792-825` | Pure in-store state flip: validates the transition, persists, publishes, then `recordAudit` (best-effort, see below). No vault side effect, so no Finding needed — the asymmetry with `ApplyDraft` is deliberate and correct. |
| `SupersedeDraft` | `internal/orchestrator/harness.go:363-480` | Validates `proposed_content`+`reason`, re-reads base version under root, atomic store swap, dual audit. Sound. |

### Audit = best-effort but visible

`recordAudit` (`harness.go:1218-1225`) and `recordReadAudit`
(`readapi.go:441-454`) are **void**: on failure they call
`h.health.MarkError("audit record failed: …")` rather than failing the
caller. That is *not* a silent swallow — the error reaches the operator:

```
recordAudit → health.MarkError → ManagedStatus.Health (readapi.go:37)
            → tui.RenderManagedStatus "Message"/"ReasonCodes"
              (internal/tui/managed_status_view.go:19-35)
            → `lore status` output
```

So an audit subsystem that starts failing shows up as a degraded health line
on the next `lore status`. The best-effort choice trades a hard stop for a
visible-degradation signal — appropriate, because the audit trail is a
*record of* the governance action, not the action itself.

## Why this is the right split (the persona/process-sink rationale)

The whole product premise is *proposal-first governance*: nothing mutates
the vault without an operator decision, and every mutation is auditable.
That gives two hard requirements that map exactly onto the invariant:

- The operator must **always be able to see the queue** to make decisions —
  so reads degrade rather than fail (regime 1).
- A mutation that already touched the vault but lost its bookkeeping is the
  one truly dangerous state — so it must be **loud and recorded**, never
  silently rolled back or dropped (regime 2 + visible audit).

This session's three behavioural fixes (`2e9479a`, `b1cd066`, `e948f90`)
were each a place the code violated the regime it belonged to; they pull the
codebase *onto* the invariant rather than adding a new convention.

## Not changed (and why)

- `transitionDraftState` / `recordAudit` void semantics: **intentional**, not
  a bug. Leaving as-is.
- `internal/tui/*` rendering of health: out-of-boundary (TUI line) and
  already correct. Mentioned only to confirm the audit-visibility loop
  closes.
- Daemon health *surfacing cadence* (does a long-running daemon re-poll
  health between commands?) is a daemon/server concern — out-of-boundary;
  noted for the A/daemon owner, not actioned here.

## Net

No code change from this review pass — the core is consistent. The value is
the recorded invariant + the audited-sound list, so the reviewer can sign
off the un-touched core paths without re-deriving the reasoning, and so the
next change knows which regime it must respect.
