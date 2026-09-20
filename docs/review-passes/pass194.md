# Review pass 194 — S187 Layer 2 converge check

`codex exec review --base main`, 2026-09-20.

## Findings

### [P2] Clean up drifted mock state before returning — ACCEPTED

> When the default drift path is hit after a successful mock apply, this early return
> skips the normal destruction branch, leaving both the mock resources and
> `terraform.tfstate` behind. If the scenario has any earlier successful run metadata,
> the next `run` without `--clean` will satisfy `detectRunMode`'s incremental conditions
> using this failed state.

Verified both halves in the code before accepting: the early return does sit above the
destruction branch, and `detectRunMode` does key incremental mode on exactly
`hasMockResources && hasTFState && previousRunID != ""` — two of which a drift stop
would leave true.

Fixed by tearing the **mock** down before returning, which is the same argument already
made for `--keep`: Layer 2 teardown is free and proves something, so nothing is bought
by skipping it. Consistency matters here — two paths that both "stop without
destroying" should not disagree about whether the mock counts.

The obvious objection is that destroying removes the evidence the operator was just told
to go and look at. It does not, materially: the plan output is already captured in the
failure detail, which is the part that names the drifting attribute, and Layer 2
reproduces in seconds for nothing. Weigh that against a contaminated baseline that is
invisible — the next run simply reports `incremental` and nobody knows why it behaved
oddly — and the teardown wins.

Layer 3 is still never reached, which the test asserts by layer.

### A test fixture that proved nothing, found while fixing this

The new assertion failed on correct code: `driftRuntime`'s fake returned a
`DestroyResult` with no `Destroy.Stage`, and `appendDestroyResult` only emits the stage
when that field is set. Worth recording because the failure mode is the useful one —
had I written the assertion loosely enough to pass, the fixture would have silently
verified nothing.

## Mutation check

`mock left up on drift stop` → caught.
