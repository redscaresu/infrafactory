# Codex review pass 1 — Layer 3 arc (PRs #148 + #149)

Base: `c901f5e` (pre-arc-close). Scope: the whole Layer 3 close-out — run 2
diagnostics fix, failure-path orphan sweep, bounded apply retry.

Triage per `feedback_codex_anti_nitpick.md`: act on substantive only, ignore
style. Stop after 2 consecutive clean passes.

## Findings

| # | Severity | Finding | Verdict |
|---|---|---|---|
| 1 | P3 | `infrafactory reap %s` in the recovery message is not shell-safe for paths containing whitespace or metacharacters (`run_command.go:1366`, and the structured-log helper below it) | **Accepted — fixed** |

### 1. Recovery command not shell-safe — accepted

Borderline on severity, accepted on **purpose**. The entire value of that
message is that an operator can paste it at the moment real resources may
be billing. A path under a directory with a space — routine on macOS, where
home directories and `/Volumes` mounts frequently contain them — would be
split by the shell and the cleanup silently would not happen. A recovery
command that does not run is a correctness defect in the recovery path, not
a style preference, so it falls under "act on" rather than "ignore".

Fixed with a `shellQuote` helper using the standard shlex algorithm:
already-safe paths print unquoted (this line is read under stress, and
gratuitous quoting makes it harder to scan), anything else is single-quoted
with embedded quotes escaped as `'\''`.

Tested by round-tripping eight paths — spaces, apostrophes, `;`, `$`,
backticks, `*` — through `/bin/sh` and asserting the shell sees the original
string. That tests the actual claim ("this is pasteable") rather than the
implementation. Synthetic drift verified: neutering the quoting fails four
of the eight cases.

## Declined

None this pass.

## Pass 2 — clean

No findings. "The changes add bounded Layer 3 apply retry handling, better
stderr surfacing, and post-failure cleanup verification without introducing
an obvious regression."

## Pass 3 — one finding, accepted

| # | Severity | Finding | Verdict |
|---|---|---|---|
| 2 | P2 | The reap hint drops the run's `--config`, so a run started with a non-default config sends the operator to the wrong output directory (`run_command.go:1387`) | **Accepted — fixed** |

### 2. Recovery command drops `--config` — accepted

The best finding of the loop, and it describes the exact flow this arc used
all evening: `--config /tmp/l3run/infrafactory.yaml` with its own
`paths.output: /tmp/l3run/output`.

`reap` rebuilds its runtime from `--config`. A hint that names only the
scenario sends the operator to the default `./infrafactory.yaml` and the
default output directory, where there is no live state — so reap reports
nothing to do and the operator concludes they are clean, while the real
resources keep billing from the directory the hint omitted. A cleanup hint
that confidently reports success against the wrong directory is worse than
no hint at all.

Fixed by routing both call sites through one `reapCommand(configPath,
scenarioPath)` builder, which also removes the duplicated formatting pass 1
noted in passing. The flag is included only when the config is non-default,
so the common case stays short.

This reset the convergence counter: pass 3 had a substantive finding, so two
further clean passes are required.

## Outcome

Passes 1–3: 2 findings, both accepted, 0 declined. Notably **zero nitpicks** —
the anti-nitpick filter did not have to reject anything, which is unusual and
suggests the prompt scope (real-money safety paths) kept the reviewer on
target.
