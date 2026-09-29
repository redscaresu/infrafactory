# Review pass 192 — S187 Layer 2 converge check

`codex exec review --base main`, 2026-09-20.

## Findings

### [P2] Preserve stderr for failed converge plans — ACCEPTED

> When `tofu plan -detailed-exitcode` fails with exit 1 rather than drift exit 2, this
> new `converge` branch records a failure, but `mockDeployFailureDetail` still only
> extracts stderr for `init` and `apply`. In that scenario the user sees only
> `exit status 1` and loses the provider error text.

Correct, and it is the defect ADR-0023 was amended for during the Layer 3 arc: Layer 3
reported failures as bare `exit status 1` and discarded the provider message, which
against a real API costs money to reproduce. **A guard that stops without saying why is
half a guard.** I added a stage that runs tofu and did not extend the one function whose
job is to carry its stderr out.

Fixed by adding the `converge` arm to `mockDeployFailureDetail`.
`TestABrokenConvergePlanIsNotReportedAsDrift` now asserts the provider text survives;
deleting the arm fails it.

## Mutation checks

Whole-package scope, compile-verified first.

| mutant | caught |
|---|---|
| exit 2 no longer detected as drift | yes |
| `-detailed-exitcode` dropped, so plan always exits 0 | yes |
| drift never stops the test | yes* |
| drift never stops the run loop | yes |
| converge stderr dropped from the failure detail | yes |

\* survived the first attempt. The assertion read `stageDetail(...)` for the destroy
stage, and a *passing* destroy stage carries an empty detail too — so it could not tell
"did not run" from "ran fine" and passed either way. Rewritten to assert that the
converge failure is the LAST stage, which is the property that actually means
"everything downstream was skipped".

## One real bug the tests caught, not the reviewer

`hasConvergeFailure` first matched on `Layer == "mock_deploy" && Stage == "converge"`
and therefore never fired: `runIteration` **rewrites** every failure it passes up as
`Layer: "run", Stage: "iteration_N_test"`. Only `Check` survives that rewrite. Found by
the run-level test counting iterations — the unit tests all passed, because they call
`executeTest` directly and never cross the rewrite.
