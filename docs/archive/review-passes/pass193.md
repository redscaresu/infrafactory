# Review pass 193 — S187 Layer 2 converge check

`codex exec review --base main`, 2026-09-20.

## Findings

### [P2] Make continue-on-drift usable from `test` — ACCEPTED

> When `infrafactory test` hits a drift failure, this message tells the operator to
> pass `--continue-on-drift`, but the flag is only registered for `run`. A standalone
> `test` run that encounters drift therefore cannot follow the suggested recovery path.

Correct. The message is emitted by `executeTest`, which both `run` and `test` call, and
I only registered the flag on one of them. Following my own advice on `test` produces
`unknown flag: --continue-on-drift`.

Of the two fixes offered — register the flag, or make the message command-specific — I
took the first. `test` is a real entry point, the converge check runs there identically,
and there is no reason the choice should be available from one command and not the other.
Making the message conditional would have preserved a capability gap and described it,
rather than closing it.

`TestBothCommandsThatCanHitDriftAcceptTheFlag` asserts both `newRunCmd` and `newTestCmd`
register it, so a third caller of `executeTest` cannot reintroduce the gap silently.

## Note

This is the second finding in a row about the same thing: a stage that emits guidance
without the guidance being actionable. Pass 192 was a failure detail that dropped the
provider's stderr; this one is a failure detail naming a flag that did not exist. Both
were "the check works, the message does not", which is the half-a-guard failure ADR-0023
names — and worth noticing as a pattern in how I add stages.
