# ADR-0032: An apply that succeeds is not a stack that converges

## Status
Accepted — 2026-09-20 (S187)

## Context

Layer 2 ran `init` then `apply` and stopped. Layer 3 ran `init`, `plan`, `apply`. Layer
1 ran `init`, `validate`, `plan`, `show`. **No layer ever asked whether a second plan
was empty.**

That leaves an entire class of defect invisible to every layer. Terraform records what
the API *returned*, not what you wrote: the provider POSTs the config, immediately GETs
it back, and core stores the response in state. If the mock echoes a different shape
than it was sent, the apply succeeds, the criteria pass, and the run reports green —
while the config and the state permanently disagree, so every subsequent plan proposes
a change to a resource nobody touched.

This is not hypothetical here. `mockway`'s own `CRITICAL[lb-ip-ids-array]` says it:

> the Scaleway Terraform provider sends the LB-IP association as an ARRAY field
> `ip_ids`, NOT the deprecated singular string `ip_id` … if we shipped the deprecated
> singular shape, **every LB-with-static-IP apply would diff every plan**

mockway catches that in its own CI, with `plan -detailed-exitcode` in
`e2e/provider_smoke_test.go`. infrafactory — the thing that *consumes* the mock and
whose whole purpose is to decide whether generated HCL is safe — did not.

## Decision

### 1. Layer 2 runs a second plan, and its exit code is the answer

`tofu plan -detailed-exitcode` after the apply. `0` is converged, `2` is changes
present, `1` is a plan that could not run. Exit 2 comes back as a `Drifted` result;
anything else non-zero is an error on a `converge` stage. Collapsing those two would
report a drift the run never observed.

Layer 2 and not Layer 3, for now. Layer 2 is free and runs in seconds, which is where a
check you want on every iteration belongs. Extending it to Layer 3 is a later decision
with a real cost attached.

### 2. Drift always fails the run

A stack that never converges is a defect whichever component caused it. There is no
warn-only mode, because a warning nobody reads is the false green this project keeps
deleting.

### 3. Two modes, because the signal is AMBIGUOUS and we cannot resolve it

Drift at Layer 2 has two possible causes:

1. the mock does not return what the provider sent, or
2. the HCL genuinely cannot converge.

**infrafactory cannot tell them apart.** Its only reference for "what should the API
have returned" is the mock, and a mock cannot certify its own fidelity. So the failure
detail names *both* hypotheses and the experiment that separates them — run the same
HCL at Layer 3; drift there too means the HCL, drift only here means the mock.

The mode decides what happens next:

- **Stop (default).** The run ends with terminal reason `drift`. Nothing downstream
  runs, nothing further is spent, and the pitfall harvest never fires — it triggers on
  `stuck` and `repair_budget_exhausted`, and a lesson learned from an ambiguous signal
  is a lesson aimed at the wrong component.
- **Continue (`--continue-on-drift`).** The run proceeds and the drift reaches the
  repair loop as ordinary feedback. Its risk is stated wherever it is offered: if the
  cause is the mock, the model rewrites HCL that was never wrong.

Stop is the default because its failure mode is a wasted re-run and continue's is a
corrupted config plus a spent repair budget.

### 4. Stopping still tears the mock down

The early return skips the destruction branch, and a mock left up satisfies two of
`detectRunMode`'s three incremental conditions — so the next run of a scenario with any
earlier success would silently build on state already known to disagree with its config.
Layer 2 teardown is free, so nothing is bought by skipping it. Same reasoning as
ADR-0027's `--keep`.

### 5. The mode is in the log, not only in the stage list

`run` does not persist `StageSummary` anywhere a human reads mid-run, so a finding that
exists only as a stage is invisible on the Live Run page for however long the rest of
the iteration takes — which under continue mode can be minutes and a real apply.
`mock_deploy_drift` is logged with status `stopping` or `continuing`. "Drift found" and
"drift found, and here is what I did about it" are different messages, and only the
second lets a watcher decide whether to interrupt.

## Consequences

**What this buys.** The `lb-ip-ids-array` class of bug now fails a run rather than
passing one. Any mock that accepts a field on write and does not return it on read is
caught on the first iteration, free.

**What it does not buy, and this is the important half.** *This does not detect a
permissive mock.* A mock that allows something real Scaleway refuses produces an empty
plan and a green run — which is exactly the v2alpha1 NIC delete of ADR-0031, where
Layer 2 reported `7 added, 7 destroyed` and certified a wrong fix. **Convergence and
fidelity are different properties.** This check buys the first one only; the second
still requires either a real apply or a contract test in the mock.

**Cost.** One extra `tofu plan` per Layer 2 apply — seconds, against a local mock, on
every iteration.

**A false positive is possible and deliberately not suppressed.** A resource that
legitimately never converges (a timestamp, a write-only field) would fail here. No
allowance list exists, because a suppression list is how this check would rot: the
entry that was added for a genuine normalisation is the entry that later hides a real
bug. If one appears, the fix goes in the HCL or the mock, and the argument gets made in
the open.
