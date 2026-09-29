# Review pass 162 — S163f

**13 findings, 11 accepted, 2 declined.** The strongest round of the arc, and
the one that mattered most: **the slice's stated rationale was false, and the
slice made its own primary audience worse off.**

## The premise was wrong

The adapter was justified — in the code comment, in ADR-0027, in the PR body,
and in a test message — by "the Live Run console renders `LogEntry` records and
groups by stage, so raw bytes would arrive as an unparsed blob".

Verified against the code, both halves are false:

- `live/+page.svelte:216` does `lines = [...lines.slice(-999), JSON.stringify(msg)]`
  and the console renders each string directly. **Every frame is a blob.** There
  was no well-formed rendering to be inconsistent with.
- `run-view.js:135` matches only `event === "stage_start" && status === "start"`,
  so the stage recovered into its own field fed no grouping, no badge, no filter.
  `stage_log_writer_test.go` asserted it with the message "the console groups by
  stage" — a claim about UI behaviour that does not exist.

And the design **regressed the case that matters most**. `AppLogger`'s default
sink is stderr, so `infrafactory test` printed
`{"level":"info","event":"sandbox_deploy_progress",...}` where `infrafactory
deploy` prints `  apply: running` for the identical event — and the S144 PR gate
runs `test`. The human reading the gate's job log, a stated audience for this
slice, got the worse rendering.

**Reworked into a tee**: the readable line to stderr, byte for byte what `deploy`
writes, and the structured entry to the log as well.

## Accepted

| # | Finding | Fix |
|---|---------|-----|
| 2 | the false premise, above | tee; comment, ADR, PR body and test message all corrected to what is true |
| 10 | `test` printed a JSON blob where `deploy` prints a sentence, in the gate's own log | the readable half, asserted at the call site |
| 3 | a failed apply logged at `info` with no status, so grepping `"level":"error"` missed why a gate run failed | `error` + `status: failed` for FAILED and giving-up lines |
| 4 | `Command` hardcoded `"test"`, no RunID/Iteration, so two iterations' `apply: running` were byte-identical and a foreign `test` was indistinguishable on the global socket | `LogScope` threaded from `runIteration` |
| 9 | the stage was the text before the first colon of ANY line, so a multi-line provider error made `Error: creating instance` into stage "Error" | shape-checked: harness indent, single token, then colon. Empty beats wrong; the line always gets through |
| 5 | the typed-nil hazard was *created* by the constructor then defended with an interface dance, an ADR section and two tests — for a branch `CommandRuntime` makes unreachable | branch removed. The hazard is gone rather than documented, and finding 8's zero-value panic goes with it |
| 7 | an empty `Command` makes `AppLogger.Log` discard every entry silently — this slice's own failure mode, one layer down | refused in the constructor |
| 1 | `TestStageProgress...WhileTheApplyIsRunning` claimed to test the wiring and could not see it; reverting the call site left it green | docstring corrected to what it does test (timing). The wiring is asserted where the call site is |
| 11 | `Close()` at the call site was covered by nothing — deleting it left the package green | asserted at the call site; what Close *does* stays a unit test |
| 12 | `liveWriter`'s `reflect.Interface` case is unreachable — `reflect.ValueOf` reports the dynamic type's kind | removed, in a helper whose whole point is precision about that distinction |

## Declined

**6 — `Write`/`Close` duplicate `api.ProgressSink`.** Two copies, not three, and
they are about to diverge rather than converge: this one tees and stamps a
scoped `LogEntry`, that one does not. Extracting a shared line buffer now would
couple two types across a package boundary to save nine lines. Revisit at a third
caller.

**13 — `drainLogDetails` duplicates `drainProgress`.** Same reason, in test
helpers, where the cost of the duplication is lowest and the cost of an
over-general helper taking an extractor func is highest.

## What this round is really about

Three of the eleven were **false explanations** rather than broken code — a
comment, an ADR paragraph and a test message asserting behaviour the codebase
does not have. The code worked; the reasons given for it were fiction, and the
fiction is what stopped anyone noticing that the CLI output had got worse.

A design justified by an unverified claim about another layer will be defended
by everyone who reads the justification. Checking `live/+page.svelte` took two
minutes and would have changed the slice before it was written.
