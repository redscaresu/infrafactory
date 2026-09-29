# Review passes 204–207 — S193 state policies actually run

`codex exec review --base main`, 2026-09-21. Three findings, all accepted.

**[P2] Respect failing expectations for plan-only policies.** `expect: fail` asks a policy
to DENY. My new skip-and-`continue` jumped past the expectation switch, so a negative
criterion against a plan-only policy passed unevaluated — strictly worse than the old
behaviour, where the absent rule returned zero denials and the switch caught it. Now a
failure with its own message; only `expect: pass` is a skip.

**[P2] Check deployed zones against `params.zone`.** The plan rule has a separate
exact-match zone rule; my state rule only did the region prefix. A criterion asking for
`fr-par-1` was satisfied by a resource in `fr-par-2` — the deployed-state check weaker than
the plan check it exists to confirm. Mirrored.

**[P2] Avoid reporting pass when state policies were skipped.** A mixed set emitted BOTH a
skip and a pass for the same check. My own comment two lines up warned about exactly this,
and I had only handled the all-skipped case. Now one stage, decided in one place: anything
unchecked means it is not green.

## Two counting bugs in my own code, same shape

The "how many were evaluated" value was derived twice before being counted once:

1. by parsing the skip message's **prose** — counting commas before a colon
2. by subtracting `len(skips)` from `len(specs)` — which counts an `expect: fail` failure
   as evaluated, because it left the loop without being skipped

Both are the same error: deriving a structured fact from something shaped for humans, in a
function whose whole purpose is to stop a check being reported as run when it was not.
Counted at the point of evaluation now.

## And one caught only by running it

`PolicyDefinesRule` took a package name and was handed the criterion's `check:` value —
`"region_restriction"` against a package called `"scaleway.region_restriction"`. It never
matched, so **every** policy reported as plan-only and the skip fired for a rule that was
right there. Unit tests passed; the first real Layer 2 run showed it. Rewritten to take the
file the caller already resolved.

## Mutation checks

| mutant | caught |
|---|---|
| params never reach the state evaluator | yes |
| plan-only policy silently passes again | yes |
| any rule counts as the named one | yes |
| `expect: fail` skipped again | yes |
