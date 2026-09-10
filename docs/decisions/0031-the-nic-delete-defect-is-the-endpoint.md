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
