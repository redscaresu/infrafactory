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
