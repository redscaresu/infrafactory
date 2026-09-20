# ADR-0033: A holdout is a negative check, probed against the running stack

## Status
Accepted — 2026-09-20 (S192)

## Context

The generator is shown a scenario's `acceptance_criteria` in its prompt, and the repair
loop feeds their failures back. It is therefore **optimising against the visible
checks**. Passing them proves it satisfied the checks it was given; it does not prove
the configuration is correct.

A holdout is the other half: criteria held back, evaluated against the result.

The machinery for this existed and was **dead**. `scenarios/holdout/` held one file that
discovery could never return — it declared neither `type: holdout` nor `references:`,
and it carried a `resources:` block, which disqualifies it three ways over. Every run
logged `holdout/discovery: pass (0 holdouts)`, which reads as coverage and was not.

## Decision

### 1. Every holdout criterion is a NEGATIVE one

Every visible criterion on `web-live-paris` is positive: the load balancer serves on
port 80, the region is fr-par. Nothing in that set punishes a configuration that is
merely too permissive — **the cheapest way to be reachable on 80 is to be reachable on
everything.**

So the rule for writing a holdout is: *for each thing the scenario asks for, what is the
sloppiest configuration that still satisfies it?* Then assert against that. The first
one asserts SSH is not open to the internet, which nothing in the scenario mentions and
Scaleway's default security group permits.

One check is expected to PASS, deliberately. A holdout whose every check fails cannot be
told from a broken holdout.

### 2. It probes the RUNNING stack; it does not re-run the pipeline

The previous mechanism called `executeTestWithScenario`, which re-ran everything — mock
reset, apply, destroy, and at Layer 3 a whole real apply and destroy — against the
**same output directory**. That is expensive, and it is wrong in kind: it re-applies over
the state of the stack it is meant to be judging, and under `--keep` it would destroy
the stack the operator asked to preserve.

The replacement reads instead of writes. The stack is already up; the holdout dials it.

### 3. Therefore: probe criteria only, and an unsupported one is REPORTED

`connectivity`, `http_probe` and `dns_resolution` work against a running stack. `policy`
and `destruction` do not, and are refused rather than skipped — a holdout that silently
drops half its checks is the false coverage this whole idea exists to remove.

That exclusion is also principled. A `policy` holdout is not really *unseen*: the rego
applies to every scenario and the pitfall corpus already teaches the model about it.
`destruction` is about teardown, not overfitting.

### 4. One insertion point, three modes

It runs after the scenario's own criteria and before any teardown. That single point
covers every mode: a plain `run` is about to destroy the stack, `--keep` and `deploy` are
about to keep it.

**After the criteria** matters as much as before teardown. A holdout is mostly
`expect: blocked`, and a blocked check passes TRIVIALLY against a stack that is not
serving yet. The scenario's own `http_probe` is what waits for the load balancer, so
probing first would report a clean pass for a machine that had not finished booting. It
is skipped, with a reason, whenever an earlier check has already failed.

### 5. A holdout failure ends the run WITHOUT a repair

This is the property the whole idea rests on.

Feeding a holdout failure back to the repair loop would make the unseen check **seen**.
The generator would fix against it, the number would keep looking good, and it would
have stopped meaning anything. At Layer 3 it also costs a real apply and destroy per lap.

So a holdout failure sets its own terminal reason — `holdout_failed` — and breaks. Same
shape as the drift stop (ADR-0032). It fails the run.

### 6. Matched on scenario NAME, not path

The path form varies by caller: the CLI passes whatever the operator typed, the UI
server builds one by joining its configured scenarios directory. A mismatch is
**silent** — discovery returns nothing and the run reports "0 holdouts", which is
indistinguishable from having none.

### 7. Three states on the estate page, not two

The deployment record carries `pass`, `fail`, or empty. "No holdout ran" is not
"the holdout passed", and a boolean would render them identically — putting "unseen
checks passed" on a deployment nothing ever probed.

This was got wrong twice in implementation before review caught it: once recording a
pass for a scenario with no holdout files, once keying off the `--holdout` request
rather than the result.

## Consequences

**A `--keep` run whose holdout fails does not keep the stack.** `keepingSandbox` requires
no failures, so the stack is torn down. That is the right default: keeping infrastructure
that failed a check it was never shown is worse than not keeping it. It also means the
estate page shows `fail` only for `deploy`, where the stack is already up.

**`--holdout` without Layer 3 is refused before generation**, rather than accepted and
skipped, for the same reason `--keep` is: a run must not report `target_reached` having
probed nothing.

**The old mechanism is deleted**, along with its `--no-destroy` and `--keep` skips.

## What this does not prove

A holdout is evidence about *one* scenario against *one* set of withheld checks. It does
not measure overfitting in general, and a holdout that passes says only that these
particular unseen checks passed. The set is small and hand-written; it is a sample, not a
proof.


## Verified against real Scaleway, 2026-09-20

`web-live-paris` with `--holdout`, first run:

```
run/iteration_2_test: pass                     every criterion the generator was SHOWN
holdout/web-live-paris-unseen: fail (1 of 2)
  connectivity probe 163.172.164.126:22: tcp connect unexpectedly succeeded
run/terminal_reason: holdout_failed
```

**1 of 2, not 2 of 2.** The 443 check passed, which is what distinguishes a holdout that
found something from a holdout that is broken — and is why one check here exists to pass.

The finding is real and structurally invisible to every other layer:

- **not the rego** — Scaleway auto-creates the security group, Terraform never owns it, so
  it is absent from the plan. A static check on generated HCL cannot see a resource the
  model did not write.
- **not the visible criteria** — all positive; a too-permissive configuration satisfies
  every one of them.
- **not the mock** — mockway has no firewall to model.

So the defect class is not "the generator wrote something sloppy". It is "the cloud granted
something nobody asked for, and the only way to find out is to ask the running system".

### A cost-model bug found while sizing this

`runConnectivityProbe` retried `expect: blocked` identically to `expect: success`. Retrying
waits for a condition to ARRIVE — correct for a load balancer coming up, meaningless for a
port, because an open port does not close itself. The retry bought only the full window (up
to five minutes at `retries: 60`) before reporting what the first dial already knew, and a
`blocked` check slow to fail reads as a hang at exactly the moment the holdout is supposed
to give a fast clear answer. `success` retries; `blocked` asks once.
