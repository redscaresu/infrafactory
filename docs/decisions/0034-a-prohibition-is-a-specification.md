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
with both halves — `deny` against the plan and `deny_state` against the Layer 2 mock's state.

`deny_state` reads the Layer 2 mock's state on every run, including one that also applied at
Layer 3: criteria evaluation is handed the mock deploy's snapshot, and no policy is evaluated
against what Layer 3 created (pinned by `internal/cli/state_policy_mock_only_test.go`). The
holdout is the only real-state check of this property.

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

### 6. On AWS the prohibition is on openings, in every shape that makes one

An AWS security group has no inbound default to set: it admits exactly what its rules say. So
`policies/aws/default_deny_ingress.rego` prohibits openings. No rule from a public source may
admit all traffic (protocol `-1` or `all`), ports 0-0, or tcp covering 22 (0-65535 included),
and no rule may name a prefix list, on any port, because its CIDRs are not in the
configuration. Public means outside 10/8, 172.16/12, 192.168/16 and fc00::/7, so `::/0`, the
two halves of `0.0.0.0/0` and a public /32 all count. A security-group reference is not a
source.

Four shapes open ingress: inline `ingress` on `aws_security_group` and on
`aws_default_security_group`, `aws_security_group_rule` with `type = "ingress"`, and
`aws_vpc_security_group_ingress_rule`. A walk that misses one fails open on it, so the policy
walks all four, over `resource_changes`, which is flat across modules, so a group at any
module depth is walked. The Layer 3 allowlist cannot close a shape for it: the allowlist binds
only when Layer 3 is enabled, and this policy runs on every AWS plan.

Unknowns fail closed. An unknown source counts as public, and an unknown protocol or port as
admitting 22. hashicorp/aws 5.100.0 plans an unknown inside an inline rule, a dynamic block
included, as that one rule's unknown fields. Only an `ingress` list that is unknown as a whole
plans as a single unknown value, and so does a group that declares no `ingress`. For those
the configuration decides, and anything but a found block with no `ingress` expression is
denied.

`deny_state` reads `ec2.security_groups[].ip_permissions` in fakeaws's state, where every
shape is folded into its group. As in section 4, it reads the Layer 2 mock's state and never
real state (pinned for AWS too by `internal/cli/state_policy_mock_only_test.go`). A group with
no `ip_permissions` field is denied rather than read as a group with no rules.

## Consequences

**ADR-0033's "not the rego" bullet stays true.** It says a static check cannot see a
resource the model did not write, and that is still the case — the auto-created group is
absent from the plan. What changed is that the fix is now expressible, and a declared
group is now verified. The holdout remains the only thing that catches a server with no
group at all.

**The allowlist is written down in three places** — `internal/config/config.go`,
`infrafactory.yaml`, and `docs/layer3/coverage.md` — and two audit tests enforce
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

## Amendment 2026-09-28: an avoid rule is retired only on evidence that contradicts it

§2 checks a prohibition's justification before it is kept. The same discipline applies when
it is removed: a `source: avoid` rule may leave the corpus only on evidence that the shape it
forbids now works, and the evidence is kept.

**The layer that taught a rule is recorded.** A learned pitfall carries `learned_layer`, the
layer whose failure taught it (`mock_deploy`, `sandbox_deploy`, ...). An avoid candidate is
deduplicated only against entries learned at the same layer, so a real-cloud occurrence of a
mock-learned prohibition is kept beside it.

**Mock evidence retires only a mock-learned rule.** `RetireAvoidPitfall`
(`internal/generator/pitfalls_avoid_retire.go`) retires an entry only when every one of these
holds, and otherwise refuses with the corpus and the ledger byte-identical:

- the rule parses in the extractor's attribute form (`ParseAvoidRule`); a resource-type
  clause or free text is never retired;
- the check's outcome is `contradicted`, with `tofu apply` exit 0 and converge plan exit 0;
  `recurred` and `inconclusive` keep the rule and record why;
- every check field is present, and the stored shape under
  `pitfalls/avoid-checks/shapes/<id>/` hashes to the recorded `shape_sha256` and sets every
  attribute the rule forbids to something other than a literal false, null or `""`;
- the entry was learned at `mock_deploy`, or predates `learned_layer` and the caller gives
  `layer_evidence` from the run artifacts that establish it was;
- it is the only entry on the resource naming those attributes, snake or camel case. A
  descriptive, fix or real-cloud entry saying the same thing would outlive the retirement.

Retirement is all-or-nothing: an entry forbidding two attributes needs a shape setting both,
and moves whole.

**The ledger is append-only.** `pitfalls/avoid-checks/<cloud>.yaml` holds `retired` and
`kept` records, each with the whole entry, its attributes, the layer and the check, and
`relearned` records. The record is written before the entry is removed. A malformed ledger
refuses every retirement.

**A recurrence is sorted by where it failed.** `AppendPitfall` refuses whole a candidate
learned at `mock_deploy`, or at an unknown layer, that names a retired attribute on its
resource, whatever its source or wording, and names the retiring check and any other
attribute it would have forbidden: the mock regressed. Learned at any other layer, it is
appended after a `relearned` record, and it then carries a non-mock layer, so it is never
mock-retirable. `live` entries are exempt. A malformed ledger does not stop learning: the
candidate is appended and the error says the ledger was not checked.

`TestRetiredAvoidPitfallsStayRetired` closes the paths that bypass `AppendPitfall` (hand edits,
the API's PUT, pitfall-merge). It fails CI on a malformed ledger, a stored shape whose hash
differs from its record, and any non-live entry naming a retired attribute with no later
non-mock `relearned` record.

**The evidence is produced by a replay with no LLM.** `infrafactory pitfalls check-avoid
<cloud> --resource R --attribute A --from DIR` (`internal/cli/pitfalls_avoid_check.go`) is the
only caller. It is built with `withRuntimeNoGenerator`, so a generate call errors. Before any
tofu call it refuses, writing nothing, when: the cloud is not `aws` (the only cloud wired) or
has no mock URL; the ledger is malformed; the corpus holds no retirable entry; an entry with no
`learned_layer` has no layer established from its run artifacts; or no `R` block in `DIR` sets
every attribute, or one sets it to a literal false, null or `""`.

- The shape is every `R` block in `DIR` that sets every attribute, plus the resource blocks
  those reference (`CutAvoidShape`). It is stored under `shapes/<id>/` before the replay, and
  the replay applies the stored bytes.
- A legacy layer is established only when `DIR` is a run's `<scenario>/<run>/iterations/<n>/
  generated` with `<scenario>` equal to `discovered_from`, every `provider "aws"` endpoint
  there is on a loopback host, and `../iteration.json` holds an apply failure naming `R` and
  every attribute (`EstablishLegacyLayer`).
- The replay is a clean Layer 2 mock deploy over fakeaws's own state client, never the
  scenario router, which falls back to mockway when no scenario is loaded. Providers come from
  `ensureAwsProviderWiring` with no run id; the env is `awsLayer2Env` with `Layer2StripEnv`.
- apply 0 and plan 0 is `contradicted`. A failed apply, a failed plan or a drifting plan whose
  output names `R` or an attribute is `recurred`; anything else is `inconclusive`. A reset,
  init or state failure records nothing: the check never answered.

**CI re-runs the evidence.** `TestE2E_RetiredAvoidPitfallsStayContradicted` replays every
retired record's stored shape against fakeaws at the CI pin, through the same constructor, and
fails naming the record unless it is still contradicted. A retired record for a cloud with no
CI mock fails it.
