# ADR-0029: A policy may not mandate a shape that cannot be destroyed

## Status
Accepted — **with its central mechanism claim REFUTED on 2026-09-10. Read the
"Refutation" section before relying on anything here.**

## Context

`policies/scaleway/vpc_required.rego` required every `scaleway_instance_server` to
be attached to a private network, and enforced that by looking for a standalone
`scaleway_instance_private_nic` resource pointing at the server. The matching
pitfall told the generator to write exactly that.

Against real Scaleway, on 2026-09-09, `web-live-paris` applied cleanly twice and
failed to destroy both times:

```
Can't delete a private network interface attached to a server
```

Terraform destroys in reverse dependency order. The NIC depends on the server, so
the NIC is deleted first — while the server is still running — and Scaleway
refuses. Auto-destroy failed on the same expression, so the run left real
infrastructure up and billing.

S168 already established the class: **a stack that reaches apply and cannot be
destroyed is the defect, and it is checkable before anything is created.** What is
new here is where it came from. This was not HCL the model happened to write. It
was HCL the model was *required* to write, by a policy and a pitfall that agreed
with each other. The static gate was manufacturing the undestroyable shape it
exists to prevent.

The precondition is **power state**, confirmed by hand the same day: `scw instance
server delete` refused with *"instance should be powered off"*, and
`scw instance private-nic delete` then succeeded on the stopped server. A running
server's NIC cannot be deleted; a stopped one's can.

Provider 2.81.0 offers a second shape for the same attachment — an inline
`private_network` block on the server (schema: list, `max_items: 8`, input
attribute `pn_id`). Deleting the server takes its NICs with it, because the
provider powers the server off before deleting, so there is no independent NIC
delete to fail.

Alternatives considered:

- **Keep the standalone NIC and fix the ordering.** There is no reliable way to
  invert Terraform's destroy order for a genuine dependency. Removing the
  dependency to free the ordering would make the create order undefined instead.
- **Drop the private-network requirement for compute scenarios.** This trades a
  teardown bug for a weaker security posture, and the requirement is sound.
- **Special-case the destroy in the harness** (stop the server, then destroy).
  Teaches the tool to work around HCL nobody should be generating, and does
  nothing for a stack applied by any other means.

## Decision

**A static policy must accept at least one shape that can be destroyed, and the
pitfall must prescribe that shape.** Concretely:

1. `vpc_required.rego` accepts the inline `private_network` block as well as the
   standalone NIC. The property being defended is *"the server is on a private
   network"*, not *"a particular resource type appears"* — the same reasoning that
   already made the policy accept both singleton and count-based NIC references.
2. The pitfall prescribes the inline block and says why, so generated HCL takes
   the destroyable path by default.
3. The Layer 3 HCL preflight **refuses** a standalone
   `scaleway_instance_private_nic` before anything is created, with a message
   that names the replacement.

Point 3 is an amendment, and the reasoning matters more than the rule. This ADR
first left the standalone shape accepted — "not wrong to apply; wrong to
*mandate*" — on the theory that the pitfall would steer generation away from it.

That theory is **untested**, not disproven. The run that followed did emit a
standalone NIC, but the repository had been switched to a branch without the new
pitfall thirty-three seconds earlier, so the generator was still being told to
write one. (Recorded because the obvious reading — "the model ignored the
advice" — was the first conclusion drawn, and it was wrong.)

The gate is added anyway, on grounds that do not depend on that question:

- **The cost is asymmetric.** A gate that never fires costs nothing. A pitfall
  that is not followed costs a real apply, a failed destroy, and a hand recovery
  needing `scw instance server stop` plus `scw instance private-nic delete` from
  someone holding cloud credentials.
- **Advice is not a control.** Whether or not the model would have complied, a
  pitfall cannot be relied on to make it comply — S167's finding about
  conventions, applied to the generator instead of to contributors.
- **The preflight covers `deploy` too**, which never evaluates a pitfall at all.

The refusal is deliberately not done by removing the type from
`allow_resource_types`. Those are different questions —
`allow_resource_types` answers *"may this cost money"*, and the NIC's cost is
fine; the shape check answers *"can this be undone"*, and its teardown is not.
Conflating them would make the allowlist lie about its purpose and let a config
edit quietly re-enable an undestroyable stack.

The rule is read from `configuration.expressions`, not `planned_values`. When the
private network is created in the same plan, the block renders in planned values
as `[{}]` — present and entirely unknown — which would let an empty block satisfy
the policy. The expression's `references` are what tie the server to a real
private network.

## Consequences

- Generated compute stacks destroy cleanly, so the escape hatch that ADR-0024
  depends on actually opens.
- One fewer resource per instance, and no parallel `count` to keep in step with
  the server's.
- Generation that does not take the pitfall's advice now fails at the gate, for
  free, with the correct shape in the error.
- The policy is looser in shape and identical in guarantee. A server with no
  private network is still denied; both fixtures asserting that were kept, and a
  third was added for an inline block belonging to a *different* server.
- **Generalisation worth applying elsewhere:** when a policy mandates a specific
  resource shape rather than a property, ask whether the mandated shape can be
  torn down. Layer 1 runs before anything exists, so a policy that gets this wrong
  is not a gate — it is a generator of the exact defect Layer 3 will later charge
  money to discover.


## Postscript: the working tree decides what a run believes

`paths.policies` and `paths.pitfalls` are `./policies` and `./pitfalls`, read
from the working tree at run time. Switching branches therefore changes what the
next run is told, silently, with no record in the run's own metadata — which is
how a run on 2026-09-10 came to be judged against a pitfall file it had never
read. Recording the commit a run was generated under would close that, and is
not done here.


## Refutation, 2026-09-10

**The inline block does not destroy either.** Run `20260910T104418Z`, iteration
4, generated exactly the shape this ADR prescribes —

```hcl
resource "scaleway_instance_server" "web" {
  private_network { pn_id = scaleway_vpc_private_network.main.id }
}
```

— applied it to real Scaleway, and failed teardown with the identical error:

```
Can't delete a private network interface attached to a server
```

So the reasoning in "Decision" is wrong where it matters. *"Deleting the server
takes its NICs with it, because the provider powers the server off first"* is
not what the provider does. It detaches the NIC as its own API call whether the
attachment was declared inline or as a standalone resource, and that call fails
while the server is running.

What survives, and what does not:

- **Survives.** The standalone `scaleway_instance_private_nic` genuinely cannot
  be destroyed, and a policy genuinely should not mandate a shape that cannot be
  torn down. The gate and the amended `vpc_required` stay.
- **Does not survive.** The claim that the inline block *solves* it. It does not.
  It is a smaller, tidier stack with the same teardown failure.
- **Unchanged.** The precondition is power state. Both hand recoveries worked the
  same way: `scw instance server stop`, then delete.

**The consequence is that no HCL shape can express a destroyable private-network
attachment on a running Scaleway instance.** When no configuration works, the
configuration is not where the fix goes. The teardown path has to power the run's
instances off before `tofu destroy` — which is precisely the move this ADR
dismissed as "teaching the tool to work around HCL nobody should be generating".
That dismissal assumed a working alternative existed. It does not.

Superseding decision to be recorded separately; this section exists so nobody
reads the Decision above and believes the problem is solved.

### Why this was believed

The mechanism was inferred from one true observation — a NIC deletes cleanly once
its server is stopped — and never tested against the inline shape, because
testing it costs a real apply. The Layer 2 verification that *was* run passed
(`7 added`, `7 destroyed`) because mockway does not enforce the precondition. A
mock that is more permissive than reality cannot refute a claim about reality,
and this ADR shipped a claim it had only mock evidence for.
