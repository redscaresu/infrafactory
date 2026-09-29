# Review pass 188 — S186 `run --keep`

`codex exec review --base main`, 2026-09-20.

## Findings

### [P2] Propagate keep registration failures to run status — ACCEPTED

> When `--keep` reaches `target_reached` but `registerKeptRun` returns a failure, for
> example because the applied state cannot be copied into an isolated workdir, that
> failure is appended to `allFailures` but `status` and the process exit code still
> depend only on `terminalReason`, holdouts, and stray projects. Because the fallback
> deployment record points at the mutable output directory, reporting success lets a
> user run the same scenario again and overwrite the only teardown state for the kept
> stack despite the `keep_workdir` failure.

Correct, and it is the same false-green shape S184 removed for stray projects three
weeks ago: a check whose failures land *after* the status decision reports success
with the thing it exists to catch outstanding.

Fixed by adding `keepBlocked` to `runCommandStatus`, which both the JSON status and
the exit code go through, so the two cannot drift. `TestRunKeepFailsTheRunWhenThe
RecordCannotDestroyWhatItKept` drives it end to end by putting a **file** where the
per-deployment workdirs go: the copy fails while the record beside it still writes,
which isolates this failure from every other way the store can break. Mutating
`keepBlocked = false` fails that test.

## Mutation checks run this slice

Whole-package scope, each verified to compile first — a `-run` filter that matches
nothing prints `ok` having run zero tests, and that has produced a false "mutant
survived" in this project before.

| mutant | caught |
|---|---|
| keep no longer skips the real destroy | yes |
| kept project reported as an accident, not a decision | yes |
| state never copied out of the run dir | yes |
| `assertKeepable` never refuses | yes (4 tests) |
| kept stack never registered | yes |
| keep never reaches the test layer | yes |
| early guard removed from the run loop | yes |
| keep+no-destroy allowed together | yes |
| keep allowed without the deploy grant (API) | yes |
| keep+no_destroy accepted (API) | yes |
| `server_allows_keep` always reported off | yes |
| `keepBlocked` never set | yes |

Two of these initially read as survivors and were not: one mutant did not compile,
and the first `-run 'Keep'` filter matched none of the `TestRegisterKeptRun*` tests.
Both were re-run before being believed.
