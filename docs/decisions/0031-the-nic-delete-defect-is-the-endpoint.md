# ADR-0031: The private-NIC teardown defect is the endpoint, not the power state

## Status
Accepted. **Supersedes ADR-0030 entirely** and replaces the mechanism in
ADR-0029's Refutation. Read this one first; the other two are kept as the record
of how long it took to get here.

## Context

For two days, every Scaleway compute scenario failed teardown with:

```
precondition failed: Can't delete a private network interface attached to a server
```

Measured on 2026-09-10, against the **same NIC on the same server, seconds
apart**:

```
DELETE /instance/v2alpha1/zones/fr-par-1/private-network-interfaces/{id}
  412  {"precondition":"resource_not_usable",
        "help_message":"Can't delete a private network interface attached to a server"}

DELETE /instance/v1/zones/fr-par-1/servers/{server}/private_nics/{id}
  204
```

**A private NIC is by definition attached to a server**, so on v2alpha1 the
precondition can never be satisfied. Provider 2.81.0 destroys NICs through
v2alpha1. Therefore the provider can never destroy one, and no HCL — no shape,
no ordering, no lifecycle block — changes that.

### Three wrong answers, and why each survived

The investigation began from a manual recovery that worked: `scw instance server
stop`, then delete. The `scw` CLI calls **v1**. The stop did nothing; the
endpoint was the entire difference. Two variables changed at once and the wrong
one got the credit.

1. **ADR-0029 — "use the inline `private_network` block; it destroys with the
   server."** Refuted by a real run: identical 412. Verified beforehand against
   **mockway**, which reported `7 added, 7 destroyed` because the mock did not
   model the refusal. *A mock more permissive than reality does not miss a bug —
   it certifies a wrong fix.*
2. **ADR-0030 — "power the instances off before `tofu destroy`."** Refuted by a
   real run: the poweroff verifiably reached `stopped`, logged the server it
   stopped, and the destroy failed identically. Cost ~45s on every teardown for
   nothing.
3. **"It is eventual consistency; retry."** Refuted by three retries over sixty
   seconds, all identical. Plausible because the *project* delete really does
   need ~2 minutes to become consistent — a true fact about a different API.

What settled it was two `curl` calls varying one thing.

## Decision

**Delete the run's private NICs through Instance v1 before `tofu destroy`, at
both layers.**

- `harness.ScalewayPrivateNICDetach` lists a project's servers, lists each
  server's NICs from the dedicated route, and deletes them via v1. The
  provider's own delete then finds nothing to fail on.
- Scoped to one project, which must already have passed
  `AssertProjectDeletable`, and the project is re-checked on every server —
  the credential can see the whole organization.
- NICs are listed from `GET /servers/{id}/private_nics`, **not** from the
  `private_nics` field on the server object. Real Scaleway embeds that field and
  mockway does not, so depending on it made the Layer 2 detach silently find
  nothing: server located, project matched, zero NICs, no error.
- Layer 2 gets the same treatment pointed at mockway, because mockway now models
  the refusal (mockway#28) and therefore hits the same wall.
- The poweroff, its settle-retry, and `ScalewayInstancePowerOff` are **deleted**.
  Keeping machinery justified by a refuted theory is how the theory survives.

Also: the **auto-created default VPC is not an orphan**. Scaleway creates one per
project and places a `vpc_id`-less private network in it; Terraform never owns it
and never destroys it. Same class as the `Default security group` that ADR-0023's
purge exists for.

## Consequences

- Compute scenarios tear themselves down. `web-live-paris` reached
  `target_reached` in **one iteration** from the UI on 2026-09-10, every stage
  green in both layers, with the account clean afterwards and no manual cleanup.
- Teardown is ~45s faster than under ADR-0030, which was paying for a no-op.
- **infrafactory now depends on a v1 route the provider has moved off.** If
  Scaleway retires it before fixing v2alpha1, teardown breaks and the contract
  test in mockway is what will say so.
- ADR-0029's *rule* survives — a policy must not mandate a shape that cannot be
  destroyed — but its example no longer applies, because the detach makes both
  shapes destroyable. The Layer 3 gate still refuses the standalone NIC, and that
  refusal is now a **preference, not a correctness property**. It is retained
  only because whether the provider tolerates a 404 on an already-removed NIC is
  unverified, and verifying it costs a real apply. Removing the refusal is a
  follow-up gated on that test, not on an argument.

## The generalisation

**When two things changed and it started working, you have not found the cause.**
The manual recovery altered the power state *and* the endpoint; three fixes were
built, shipped and retracted on the wrong one. Each was "verified" — against a
mock, against a sibling API, against reasoning — and none against the thing
itself. The cheapest possible experiment, two `curl` calls differing in one
variable, was available on day one.


## Amendment, 2026-09-20 (S188): upstream fixed it by changing the endpoint

Provider **2.83.0** no longer deletes the interface. It calls
`POST /instance/v2alpha1/zones/{zone}/servers/{id}/detach-private-network-interface`,
naming the interface in `private_network_interface_id`
(scaleway/terraform-provider-scaleway#4354).

Confirmed by running both pins against mockway rather than by reading the release
note. 2.81.0 still fails with the 412 this ADR documents; 2.83.0 calls the new route
and, once mockway implemented it (mockway#29), tears the same stack down cleanly with
**no workaround**.

So this ADR's diagnosis holds exactly as written — the defect was the endpoint — and
upstream's fix is the same conclusion reached from the other side.

### The workaround stayed until it was tested, then went (S189)

`ScalewayPrivateNICDetach` and the `private_nic_detach` stage are retained. What has
been demonstrated is that 2.83.0 tears down cleanly **against mockway**. mockway is a
better witness than it was — it models the 412 refusal, and now the detach route — but
it is still the cheaper stand-in, and this project has a documented habit of retracting
fixes that were verified against one. The detach is idempotent and cheap: if the
provider now handles it, the stage finds nothing to remove and says so.

Removing it needs a real-cloud canary: `web-live-paris` applied and destroyed against
Scaleway with the stage disabled, and the account verified empty afterwards. That is a
separate decision and belongs with the evidence.


## Amendment, 2026-09-20 (S189): the workaround is removed

The previous amendment said removing it needed a real-cloud canary. It got three.

Two full `run`s of `web-live-paris` against real Scaleway and one `deploy` +
`live teardown`, all with the detach deleted: every destroy passed, every orphan sweep
passed, every run project was deleted, and the account finished on exactly the three
projects it started with. `Can't delete a private network interface attached to a
server` appears **zero** times across all of it.

So this ADR's mechanism is now history rather than active defence: the defect was the
endpoint, upstream changed the endpoint, and the code that routed around it is gone.

**What this does not license.** `vpc_required.rego` still refuses a standalone
`scaleway_instance_private_nic` on the grounds that it "cannot be destroyed". Nothing
here tested that shape on 2.83.0 — every canary used the inline `private_network` block.
The rule therefore stands, and its stated reason is now suspect rather than disproven.
Testing it is one canary; rewriting the rule without one would repeat the mistake the
Refutation section above documents.
