# ADR-0030: Teardown powers the run's instances off before `tofu destroy`

## Status
Accepted. Supersedes the Decision in ADR-0029 on the question of *where* the
private-NIC teardown failure is fixed.

## Context

A Scaleway private NIC is deletable only while its server is **powered off**.
Terraform destroys in reverse dependency order, so the NIC — which references
the server — is deleted first, with the server still running, and the API
refuses:

```
precondition failed: Can't delete a private network interface attached to a server
```

Measured on real Scaleway across 2026-09-09 and 2026-09-10: four applies, four
failed destroys. The auto-destroy failed on the same expression each time, so
every one of those runs left billable infrastructure up. Recovery was identical
every time and required a human with shell access and cloud credentials:
`scw instance server stop`, then delete.

**ADR-0029 tried to fix this in the configuration and failed.** It refused the
standalone `scaleway_instance_private_nic` and prescribed the provider's inline
`private_network` block, reasoning that a server's own delete powers it off
first and takes its NICs with it. Run `20260910T104418Z` iteration 4 generated
exactly that shape and failed teardown identically. The provider detaches the
NIC as its own API call either way.

So **no HCL shape expresses a destroyable private-network attachment on a
running instance**, and when no configuration works, the configuration is not
where the fix goes.

ADR-0029 explicitly dismissed this alternative — "teaches the tool to work
around HCL nobody should be generating, and does nothing for a stack applied by
any other means". That dismissal assumed a working alternative existed.

## Decision

**The teardown powers off every Instance in the run's project before
`tofu destroy`, and reports what it stopped.**

Four properties, each load-bearing:

1. **Before the first destroy, not as a retry.** The auto-created security-group
   purge is *remediation* — it reacts to a failure nobody could predict from the
   plan. This is a *precondition*, known in advance for every stack that declares
   compute. Running it as a retry would make every compute teardown pay for a
   failed destroy first.
2. **Behind the same `AssertProjectDeletable` guard as the purge.** It changes
   the state of real servers over HTTP with Terraform nowhere in the loop. A
   state file that is stale, hand-edited, or names the organization's default
   project must not reach it.
3. **The API is asked to filter by project and the response is checked anyway.**
   The credential can see every project in the organization; a query parameter is
   not allowed to be the only thing standing between this and someone else's
   servers.
4. **It waits for `stopped`, and says so when a server never gets there.**
   `poweroff` returns as soon as the task is accepted, and a destroy started at
   that moment hits the very precondition being cleared — which is how the first
   manual recovery failed, before `-w` was added to the stop.

Best-effort otherwise. The authoritative "did we leak?" answer stays with
`ScalewayOrphanSweep`, which fails closed.

## Consequences

- Compute scenarios can tear themselves down at Layer 3, so the escape hatch
  ADR-0024 depends on actually opens.
- Every teardown of a project containing running instances now costs a poweroff
  (tens of seconds). Paid on stacks that would otherwise have failed.
- **ADR-0029 keeps its rule and loses its mechanism.** A policy still must not
  mandate a shape that cannot be destroyed, and the standalone NIC is still
  refused. What is retracted is the claim that the inline block *solves* it.
- The generalisation: **when a precondition belongs to the API rather than to the
  configuration, no amount of policy on the configuration will reach it.** Layer 1
  and the Layer 3 shape gate can only refuse what HCL can express. This one could
  not be expressed, and three attempts to write it as a rule produced two
  retracted pitfalls and one refuted ADR before the constraint was located
  correctly.

## Amendment, 2026-09-10 — it has to say why it did nothing

The first implementation had three silent `return nil` paths. Run
`20260910T141427Z` then failed its destroy with **no `instance_poweroff` stage at
all**, and the log could not distinguish "no project id" from "guard refused"
from "found nothing to stop". A real apply bought no information.

That is the finding the Layer 3 arc closed with — *a guard that stops without
saying why is half a guard* — reproduced inside the change whose own ADR quotes
it. Every skip now carries its reason into the stage summary, rendered as a
**skip**, never as a pass.

**And the likely cause is fixed, not merely reported.** Callers pass
`sweepTargetProjectID(...)`, which is empty whenever `CaptureSweepTarget` failed.
Both remediations are scoped by that id, so one failed capture disabled the purge
and the poweroff *together* — which is why that run shows neither stage. The
project id is now resolved once, in `destroySandbox`, falling back to the
run-project marker: the same provenance `AssertProjectDeletable` reads, one step
earlier, and that check still runs.

The boundary this moves was previously asserted as a safety property ("without a
project id, do not purge"). It was not one. An empty id is not a caller declining
to act; it is a capture that failed, and treating it as a refusal turned a
recoverable teardown into a leak. The guard that makes purging safe is
`AssertProjectDeletable`, and it is untouched.
