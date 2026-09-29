# Codex review pass 8 — S150 CI hardening

| Pass | Findings | Outcome |
|---|---|---|
| 1 | 1 (P1) | accepted — and reproducing it found something worse |
| 2 | none | — |
| 3 | none | **converged** |

## Pass 1 — accepted, and the real defect was different

**Reported**: the new `govulncheck` job would fail on a clean runner, because
`cmd/infrafactory/embed.go` has `//go:embed all:ui/build` and that directory
is generated and gitignored.

**Reproduced**, by moving the build directory aside — and the outcome was not
what was reported. `go build ./...` does fail:

```
cmd/infrafactory/embed.go:10:12: pattern all:ui/build: no matching files found
```

But `govulncheck ./...` **exits 0 and reports nothing**. It does not fail on a
package it cannot load; it skips it.

So the job would have gone **green while scanning less than it appeared to**.
That is worse than the reported breakage, because a broken job gets noticed on
the first PR and a silently-reduced scan does not — and it is the same false
green shape this repo has hit repeatedly: the S139 env leak, and the
`--no-destroy` cleanup gate.

Fixed with `-tags noui`, which swaps the embed for a stub so every package
loads and is actually scanned.

**What was not verified**: a probe comparing scanned packages between the two
modes was inconclusive, because govulncheck's JSON reports findings rather
than scanned packages. The argument rests on the build evidence — a package
that cannot load cannot be analysed — and that is stated rather than dressed
up as a measurement.

## Note

Third time in this project that "the check passes" and "the check ran" have
come apart. Worth treating as a standing question for any new gate: *what
does this look like when it silently covers less than I think?*
