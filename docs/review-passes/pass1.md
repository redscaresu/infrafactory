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

## Outcome

1 finding, 1 accepted, 0 declined. Not yet converged — the loop needs two
consecutive passes with no substantive findings.
