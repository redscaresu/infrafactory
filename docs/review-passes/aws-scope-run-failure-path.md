# Review — an aws run acts on the scope's claim only when it holds it

Named for the story rather than numbered: pass numbers are a shared counter, and
parallel agents picking the next one collide.

One finding declined across the loop; the loop itself is in the PR body.

## Declined: [P1] "Check kept AWS claims before declaring success"

The claim: a passing iteration under `--no-destroy`, or with destruction disabled,
keeps the claim without a failure, so the run reports `target_reached` and success
while the claim and the stack are still held.

That is the contract, not a gap. A deliberate keep is not a failure: `test` asserts
exactly that (`TestAWSTestKeepsTheClaimWhenDestructionIsNotWanted`, "a deliberate
keep is not a failure"), and `--no-destroy` exists to keep a successful run's state
for an incremental follow-up. Failing it would make the flag unusable on aws and put
`run` and `test` at odds over the same iteration.

What the finding worries about is covered. The run does end: a passing iteration
breaks the loop. The kept stage, whose detail names the reap command, is carried
into the run's own output. `TestAWSRunWithNoDestroyEndsAfterOneIteration/passing
iteration` pins both. The next aws run is refused naming the holder, and reap's
`--take-over` releases it. Only a kept claim after a failing iteration is a failure,
and that ends the run with `aws_scope_claim_kept`.
