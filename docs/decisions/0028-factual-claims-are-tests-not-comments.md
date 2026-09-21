# ADR-0028: a factual claim about another component is a test, not a comment

Status: accepted (2026-09-07, S169)

## Context

This codebase writes long "why" comments deliberately, and they are load-bearing:
they are how a decision survives being read six months later. That style is a
strength and it is also the vector for the most expensive defects found so far.

In the week of 2026-09-01, every serious review finding was the same shape — **not
broken code, but a confident claim about how another component behaves, written
into a comment and never checked**:

| claim | reality |
|---|---|
| "the YAML error names the file it was read from" | `yaml.Unmarshal` receives bytes; it has no filename |
| "the cause is on the command's stderr" | `runDeployCommand` is called directly, and cobra only prints from `Execute()` |
| "there is no spelling that does not call `.Error()`" | `fmt.Sprintf("%v", err)` is one |
| "the console groups by stage" | nothing reads that field |
| "a dispose that never calls back, exactly like `connectWS`" | it calls back, and late |

Each was checkable in under two minutes. Each propagated — into an ADR, a PR body,
a commit message, a report to the operator — because a well-argued paragraph reads
as something somebody already verified. **A rule that is wrong fails once; a rule
that is wrong and carries a confident rationale keeps failing until somebody runs
it.**

Three of them reached already-merged slices, which is the other half of the
problem: a comment cannot notice when the dependency underneath it changes.

## Decision

A factual claim about another component may not be written as a comment. It is
written as a **test**, and the comment cites it by name.

```go
// yaml.Unmarshal has no filename to report
// (TestAssumption_YAMLErrorsCannotNameTheFile).
ParseError: err.Error(),
```

Facts go in tests. **Decisions stay in prose** — a trade-off ("a tee rather than an
adapter", "the exit code stays 0 because forget is deliberate") cannot be tested and
belongs in a comment or an ADR, which is what those are for.

Assumption tests live in `assumptions_test.go` per package, named
`TestAssumption_<claim>`, and each names the site that relies on it.

**The citation runs both ways**, and both directions earn their keep. The test
names its relying site, so deleting the site is visibly the moment the test stops
mattering. The site names its test, so a reader who doubts the claim verifies it
with one grep instead of reconstructing an argument — which is precisely what
nobody did for the five claims that failed.

## Why not a convention

Because S167 is the counter-example. "Do not put an error's text in a response body"
was a known rule, and it was violated at sixty-five sites; only a failing `go test`
stopped it. Adding "verify your claims" to the same pile would be the same mistake
one level up.

This works instead because it **inverts the cost**. Asserting used to be cheaper
than checking; now the check is the artifact. You cannot cite a test you did not
write, and writing it runs it — the discipline and the deliverable become one
action, with no gap for good intentions.

## Consequences

- Claims fail loudly when a dependency changes, which prose never did.
- A reviewer verifies a claim with one grep instead of reconstructing an argument.
- Test volume grows, bounded by "claims load-bearing enough to write a paragraph
  about".
- **It does not reach everything.** Facts about real Scaleway are expensive to test
  and stay prose — the S168 teardown finding is exactly that kind, and remains a
  risk this ADR does not cover.


## Amendment, 2026-09-21 (S193): an undefined rego rule is not a passing one

Rego rules are **undefined** rather than false when they do not exist, and an undefined
rule evaluates to zero results — which is exactly what a defined rule that found nothing
wrong returns. The evaluator cannot tell the two apart, so both render green.

That turned a scenario's `acceptance_criteria` into a claim nobody was checking.
`web-live-paris` names `check: region_restriction`; the run reported
`state_policy: pass`; and `region_restriction.rego` had no `deny_state` rule, so nothing
was evaluated against deployed state at all.

### Decision

**A policy named by a criterion that has no `deny_state` rule is reported, not passed.**
Rule existence is answered from the **AST** (`ast.ParseModule`), not by grepping the file
— a rule named in a comment or inside a string is not a definition.

It is a **skip**, not a failure: some policies are legitimately plan-only, and failing
them would refuse scenarios that are not wrong. The requirement is that the gap is
visible.

**Except for `expect: fail`,** which is a failure. A negative criterion asks the policy to
deny; a policy with no state rule can never deny, so skipping would report success for an
assertion nothing could satisfy.

**And the check emits exactly one stage.** A skip beside a pass is two contradictory
claims about the same thing, and the green one is the one people read. Green here means
*every policy this scenario named was evaluated against deployed state and none denied* —
nothing weaker earns it.

### The enabling fix

The state evaluator forwarded only `target`, never the criterion's `params`. So
`input.params.region` was undefined and **any** parameterised state rule would have
silently never fired. The rule was not merely unwritten; it was not writable. Params now
go through exactly as the plan evaluator passes them.

### Why this belongs to ADR-0028

The rule here is *factual claims are tests, not comments*. This is the same failure one
level down: a claim encoded as a **criterion** rather than a comment, which the system
accepted, displayed, and never evaluated. An assertion that cannot fail is documentation
wearing a test's clothes.

Two counting bugs while implementing it made the same mistake in miniature — "how many
were evaluated" was derived first by parsing the skip message's prose, then by subtracting
the skip count from the spec count. Both derive a structured fact from something shaped
for humans, inside the function that exists to stop a check being reported as run when it
was not. It is counted where the evaluation happens now.
