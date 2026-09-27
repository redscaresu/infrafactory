# ADR-0034: A prohibition is a specification

## Status
Accepted — 2026-09-27 (S194)

## Context

ADR-0033's verification run found SSH open to the public internet on `web-live-paris`.
Every visible criterion passed; a withheld connectivity check dialled port 22 and got a
connection.

The obvious reading is that the generator was sloppy — it wrote no firewall because
nothing told it to. That reading is wrong, and the real one is worse.

`pitfalls/scaleway.yaml` carried this, learned from `web-live-paris` itself:

> NEVER declare a `scaleway_instance_security_group`. It is not in
> `allow_resource_types`, so Layer 3 refuses the whole configuration before anything is
> applied and the run loses an iteration to it.
>
> **You do not need one. Scaleway CREATES a "Default security group" in every project on
> the first instance, and it permits what these scenarios need.**

Every sentence there is true. The project had no security group on its Layer 3 allowlist,
so declaring one did cost an iteration, and the API-created default group does permit
what these scenarios need.

It also permits everything else. `inbound_default_policy` on that group is `accept`
(confirmed against the real API, 2026-09-27: `project_default: true`,
`organization_default: true`, inbound and outbound both `accept`). "Permits what the
scenario needs" and "permits every port" are the same sentence about the same object, and
the pitfall asserted the first while the second was what shipped.

So the generator did not omit a firewall. It was instructed not to write one, on a
justification checked against the scenarios and never against the threat — and the
instruction was reinforced on every iteration, because pitfalls are injected into every
generation prompt.

## Decision

### 1. A pitfall that forbids a resource type is reviewed like a policy that requires one

A policy in `policies/` gets tests, a rubric and an ADR, because it constrains what may
be built. A pitfall that says NEVER constrains it at least as hard: a policy denies a
shape after the model writes it and tells it why, while a prohibition removes the shape
from the space the model draws from at all. Nothing downstream can recover it — no
iteration, no repair, no probe. It is a specification, so it is scrutinised as one.

This applies to the *forbidding* kind specifically. A pitfall that teaches a provider
quirk, or records a fix that worked, is guidance. One that removes a resource type is a
design decision about what this project can express.

### 2. The justification is what carries the weight, so it is checked, not merely true

The failure here was not a false claim. It was a true claim doing a job it could not do:
"it permits what these scenarios need" is evidence about the scenarios, offered as
evidence about safety.

ADR-0028 made factual claims about other components into tests. This extends the same
discipline to a claim's *scope*: when a prohibition rests on "you do not need one", the
question is not only whether the current scenarios pass without it, but what the
prohibition makes impossible to write.

### 3. `scaleway_instance_security_group` is allowlisted

The allowlist asks "may this cost money". A security group is free, and unlike the
API-created one, Terraform owns a declared one — applied and destroyed cleanly against
real Scaleway on 2026-09-27 with the project left empty.

Its absence created exactly the contradiction the `private_nic` entry is there to
prevent, in the other direction: static guidance forbade the only resource that could
satisfy a check, so no generated HCL could satisfy both.

### 4. A declared security group must be a real firewall

`policies/scaleway/default_deny_ingress.rego` denies any
`scaleway_instance_security_group` that does not set `inbound_default_policy = "drop"`,
with both halves — `deny` against the plan and `deny_state` against deployed state.

This rule exists because the obvious fix does not work. A group declaring only a name
applies with inbound AND outbound `accept`: indistinguishable in effect from having no
group at all. Adding a security group is not the fix; adding one that denies by default
is. The provider schema does not publish that default and the API spec calls it
`unknown_policy`, so neither source answers the question — an apply does.

Readable at Layer 1 because the provider renders its own default into the plan: an
omitted attribute shows as the literal `"accept"` in `planned_values`, with
`after_unknown` false. That makes it the cheap case, unlike `vpc_required`, whose subject
is a cross-resource reference unknown until apply.

### 5. The API-created default group is excluded from `deny_state`, and that is the limit

Scaleway creates its default group on the first instance in every project, with inbound
`accept`, and Terraform never owns it — which is why teardown has a purge step. Denying
it would fail every run forever, for a shape no repair iteration could fix. A check that
cannot be satisfied is not a check.

So the property this policy defends is precisely **"a security group this configuration
declares is a real firewall"**. It is NOT "the server is behind a firewall": a
configuration declaring no security group at all leaves its server on the API default and
passes. That gap is named in the rego, asserted by a test, and stated here rather than
left to be discovered.

## Consequences

**ADR-0033's "not the rego" bullet stays true.** It says a static check cannot see a
resource the model did not write, and that is still the case — the auto-created group is
absent from the plan. What changed is that the fix is now expressible, and a declared
group is now verified. The holdout remains the only thing that catches a server with no
group at all.

**The allowlist is written down in three places** — `internal/config/config.go`,
`infrafactory.yaml`, and `docs/layer3-coverage.md` — and two audit tests enforce
agreement. Both caught this change before review did, which is the intended behaviour and
worth recording as evidence that the pattern from ADR-0028 works.

**The pitfall now prescribes the three-part shape** (`drop`, an `inbound_rule` per port in
use, and `security_group_id` on the server), under the 1000-byte cap the source ratchet
enforces. The history of the reversal lives in the config comments and here, not in the
prompt, because a pitfall is paid for on every iteration.

**A third copy of the refuted "standalone NIC cannot be destroyed" claim** was found in
`infrafactory.yaml` during this slice and corrected. S191 fixed the same sentence in
`vpc_required.rego` and missed this one. The claim has now been wrong in three places
across three slices, which is an argument for the lockstep-audit treatment
(`TestCloudPrefixLockstep`'s shape) rather than another careful edit.

## What this does not prove

**Whether the generator now writes a locked-down security group is untested.** Removing a
prohibition is not the same as creating a requirement: nothing in `web-live-paris`'s
visible criteria mentions a firewall, so the cheapest configuration that satisfies them
may still declare no group. This slice makes the correct shape expressible, verified when
present, and no longer forbidden. Whether that is enough to change what gets generated is
an empirical question, and the answer needs a run that has not been done.

If it is not enough, the requirement cannot come from the holdout — feeding it back would
make the unseen check seen (ADR-0033 §5). It would have to come from a rule about the
server rather than the group.
