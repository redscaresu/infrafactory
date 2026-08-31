# Layer 3 coverage: which Scaleway scenarios can run against the real API

> **Retraction, 2026-08-30 — read before the IPAM references below.**
> This document repeatedly names `IPAMFullAccess` as what gates private
> networking. That was wrong. A canary asked the real API, which wanted
> `write compute_private_networks` — `PrivateNetworksFullAccess`, granted
> 2026-08-30. Private *networks* now create. Private *NICs* still cannot, and
> **no permission fixes it**: `scaleway_instance_private_nic` has no `project_id`
> attribute (verified against provider 2.81.0), so it is created in the provider
> default project — the ADR-0010 containment project — while the server lives in
> the run's own, and the API refuses the mismatch.
>
> A second claim made the same day, that `vpc_required.rego` is enforced for AWS
> only, was **also wrong and is retracted**: `filterPolicyPathsByCloud` drops only
> *other* clouds, so all five `policies/scaleway/*.rego` run against a Scaleway
> plan.
>
> The real position **at the time**: Layer 1 required a private NIC on every
> instance server and Layer 3 could not apply one. The gates contradicted each
> other.
>
> **RESOLVED 2026-08-31 by the S166+S167 cutover (ADR-0025).** The run's project
> is created through the Account API before the apply and handed to the provider
> as the default, so the NIC lands in the same project as its server. Verified
> against real Scaleway: [docs/status/s168-cutover-canary.md](status/s168-cutover-canary.md).
> The "waits on IPAM" lines below are stale in their *reason*; whether each
> scenario is runnable is a costing question again, not a blocked one.


Audit date: 2026-08-24. **Refreshed** after `scaleway_instance_ip` and
`scaleway_instance_server` were admitted to the allowlist so a load
balancer could have a backend that serves — see the allowlist amendment
in ADR-0023. Derived from the **actually generated HCL** in
`.infrafactory/runs/<scenario>/<run_id>/generated/*.tf`, not inferred from
`mappings.yaml` — every one of the 16 Scaleway scenarios that existed at
audit time has prior run artifacts. `lb-serving-paris`, added afterwards,
has none: it is driven from fixed HCL through the gate rather than
generated.

The point of this file is to stop "let's expand Layer 3" being a vague
ambition. Expansion is a cost-and-blast-radius decision per scenario, and
this is the costed list.

## The two gates

A scenario reaches the real API only if **both** allow it:

1. **`validation.layers.sandbox_deploy.allow_resource_types`** — deny-by-default,
   checked after generation and before apply (ADR-0023 rule 5). Repo default:
   `scaleway_account_project`, `scaleway_block_volume`, `scaleway_block_snapshot`,
   `scaleway_vpc`, `scaleway_vpc_private_network`, `scaleway_lb*`,
   `scaleway_domain*`, `scaleway_iam*`, `scaleway_registry_namespace`,
   `scaleway_instance_ip`, `scaleway_instance_server`,
   `scaleway_instance_private_nic`.
2. **The `infrafactory-layer3` IAM policy** — `ProjectManager`,
   `BlockStorageFullAccess`, `LoadBalancersFullAccess`, `VPCFullAccess`,
   `InstancesFullAccess`. Nothing else (ADR-0023, credential amendments).

They do not agree, deliberately. Four allowlisted entries —
`scaleway_iam*`, `scaleway_registry_namespace`, `scaleway_domain*` and
`scaleway_instance_private_nic` — pass the local check and are then refused
by the API with a 403. That looks like a bug if you do not know it; it is
the credential doing its job.

The first three are least-privilege choices. The fourth is not: private
NICs are allowlisted only because `policies/scaleway/vpc_required.rego`
denies any instance server without one, so omitting them would leave
static policy demanding a resource the allowlist forbids — unsatisfiable
by any generated HCL. It waits on private networking becoming applicable at all (see the retraction at the top), not on `IPAMFullAccess`.

## Coverage

| Scenario | Status | Class | Blocked on |
|---|---|---|---|
| `block-paris` | **runnable** | instant | — (run 2026-08-22) |
| `lb-paris` | **runnable** | hourly | — (run 2026-08-23) |
| `lb-serving-paris` | **runnable** | hourly | — (added 2026-08-24; the first `http_probe` against real Scaleway) |
| `web-live-paris` | key only | hourly | private networking (generation emits `scaleway_instance_private_nic`; canary 2026-08-30) |
| `incremental-project-paris` | key only | hourly | private networking (allowlist cleared 2026-08-24; see the retraction at the top) |
| `registry-paris` | key only | instant | Registry |
| `iam-policies-paris` | key only | instant | IAM |
| `public-registry-iam-paris` | key only | instant | IAM + Registry |
| `domain-paris` | key only | instant | DomainsDNS **+ a registered domain** |
| `compute-lb-multi-paris` | allowlist + key | hourly | IPAM; allowlist `instance_*`, `ipam_ip` |
| `k8s-cluster-paris` | allowlist + key | slow + expensive | Kubernetes |
| `k8s-medium-override-paris` | allowlist + key | slow + expensive | Kubernetes |
| `redis-paris` | allowlist + key | slow + expensive | Redis |
| `redis-xlarge-session-paris` | allowlist + key | slow + expensive | Redis; allowlist `instance_*` |
| `mysql-ha-paris` | allowlist + key | slow + expensive | RDB; allowlist `instance_*`, `rdb_*` |
| `private-lb-db-paris` | allowlist + key | slow + expensive | RDB; allowlist `instance_*`, `rdb_*` |
| `web-app-paris` | allowlist + key | slow + expensive | DomainsDNS, IPAM, RDB, VPCGateway |
| `full-stack-paris` | allowlist + key | slow + expensive | IAM, Kubernetes, RDB, Redis, Registry |

**Current: 3 have run, 0 ungated but unrun, 6 are blocked by the key alone, 9 by both.** As
first audited on 2026-08-23 it was 2 runnable, 1 blocked by the allowlist
alone, 4 by the key alone and 9 by both; `lb-serving-paris` did not exist
yet, and `incremental-project-paris` has since moved from the allowlist
column to the key one.

`incremental-project-paris` has swapped blockers rather than lost one.
Admitting `instance_ip`, `instance_server` and `instance_private_nic` on
2026-08-24 cleared its allowlist gate — and left the credential gate
holding it, because the scenario declares `private_network: true` and its
generated HCL creates `scaleway_instance_private_nic`, which needs
`IPAMFullAccess`. It is the clearest illustration in this table of why
two gates are counted separately: opening one moved the scenario sideways,
not forward.

Class is provisioning cost, not money: *instant* is seconds and negligible
(project, block volume, VPC, registry namespace); *hourly* bills for as long as
it exists (load balancer, instance); *slow + expensive* takes minutes to
provision and destroy and costs materially more (RDB, Redis, k8s). No euro
figures here on purpose — ADR-0010 deferred cost estimation because there is no
reliable Scaleway pricing source to code against, and inventing numbers would
be worse than none.

## The finding: the cheap end of the pool is empty

Every scenario marked **runnable** is one that has actually run against
the real API — the word means has-run here, deliberately, because an
untested claim about what the real cloud will accept is the exact thing
this arc exists to distrust. There is no free
expansion, and four of the five "key only" entries are not the easy wins
they look like:

- **`domain-paris` is not a permission problem.** The account holds no
  registered public domain — only the auto-created `privatedns` zone for
  private networks. `scaleway_domain_zone` needs a real one, so this is a
  purchase, not a policy line.
- **`iam-policies-paris` and `public-registry-iam-paris` should stay
  blocked.** Granting IAM to the sandbox credential defeats the credential:
  a sandbox that can mint API keys is not a sandbox. Keep them as Layer 2
  scenarios.
- **`incremental-project-paris` needs private networking** (recorded as IPAM at the time; see the retraction at the top), which is a real widening
  rather than a formality: IPAM hands out addresses across the whole
  organization's private networks. It is the cheapest *remaining* hourly
  scenario, and still a blast-radius decision rather than a config line.
- **`registry-paris` is the only defensible near-free expansion** — one
  permission set, instant provisioning. Note it still widens org-scoped
  registry access over the existing `funcscwblognolj7nc9` namespace, because
  product permission sets are project-scoped and per-run projects do not exist
  yet, so the rules must be organization-scoped.

Before widening anything here, check it against
`examples/layer3-plan-lied/`. The `iam-scope` case works precisely because
`scaleway_domain*` is allowlisted and refused by the credential, so a grant
that closed that gap would quietly invalidate committed evidence. Domains
is on nobody's expansion list, which is exactly why it was chosen for the
fixture.

Everything below those needs RDB, Redis or Kubernetes — both gates widened,
and the expensive class the allowlist exists to keep out.

### Two families that are easy to miss

Neither appears in any scenario's `resources:` block, and each needs its own
permission set:

- **`scaleway_ipam_ip`** → `IPAMFullAccess`. Emitted by `compute-lb-multi-paris`
  and `web-app-paris`.
- **`scaleway_vpc_public_gateway`, `scaleway_vpc_public_gateway_ip`,
  `scaleway_vpc_gateway_network`** → `VPCGatewayFullAccess`, and a public
  gateway **bills hourly**. Emitted by `web-app-paris` only.

The allowlist entries are the exact strings `scaleway_vpc` and
`scaleway_vpc_private_network`, *not* a `scaleway_vpc*` glob, so the gateway
types are already denied locally. That is the allowlist working as intended —
widening it to a glob would silently admit an hourly-billed resource.

### The types are not stable across runs

Worth internalising before trusting any row here: **10 scenarios declare
private networking, but only 2 emitted `scaleway_ipam_ip` and only 1 emitted
gateway resources.** Topologically similar scenarios diverge because the LLM
writes the HCL — `mysql-ha-paris` and `private-lb-db-paris` both attach servers
to a private network and neither pulled in IPAM, while `compute-lb-multi-paris`
did.

So this table is a snapshot of what the generator *has produced*, not a
guarantee of what it *will* produce. A scenario currently marked runnable could
emit a new type on its next run and be refused by the allowlist — which is the
allowlist doing its job, and is why it is deny-by-default rather than a warning.
Re-run the refresh command after any regeneration rather than trusting a
months-old row.

**Consequence for `http_probe`.** Proving infrastructure actually *serves*
traffic rather than merely accepting TCP needs a backend behind the load
balancer, so `scaleway_instance_server`, so Instances on both gates. It is not
a tidy-up; it sits in the same bucket as everything else here.

## Caveat on the data

14 of the 16 artifacts date from 2026-06-07, before ADR-0010's
`scaleway_account_project` requirement was enforced — only `block-paris` and
`lb-paris` contain one. Regenerating those scenarios for Layer 3 would add a
project resource, which is both allowlisted and permitted, so no classification
changes. Their other resource types are what the generator actually produced
and are the basis of this table.

## Refreshing this

```bash
# Resource types per scenario, from real generated HCL.
# Scans BOTH the final snapshot and per-iteration snapshots: a scenario that
# converged on iteration 3 still generated -- and would have applied -- the
# types from iterations 1 and 2. compute-lb-multi-paris emits
# scaleway_ipam_ip only in an iteration snapshot, and web-app-paris does the
# same for its VPC gateway resources, so scanning only generated/*.tf drops
# real blockers.
for s in $(grep -l 'cloud: scaleway' scenarios/training/*.yaml | xargs -n1 basename | sed 's/.yaml//'); do
  echo "$s: $(grep -ho 'resource "[a-z0-9_]*"' \
    .infrafactory/runs/$s/*/generated/*.tf \
    .infrafactory/runs/$s/*/iterations/*/generated/*.tf 2>/dev/null \
    | sed 's/resource "//;s/"//' | sort -u | tr '\n' ' ')"
done
```

Re-run it after any change to the allowlist, the IAM policy, or a scenario's
`resources:` block.

## Refresh, 2026-08-24

`scaleway_instance_ip` and `scaleway_instance_server` are now allowlisted, and the policy already carried `InstancesFullAccess`, so both gates admit a small compute backend.

**Runnable: 3 of 17** Scaleway training scenarios — 18 counting the holdout. `block-paris`, `lb-paris`, and the new `lb-serving-paris`, all three of which have actually run. *(Denominator as of 2026-08-24. `web-live-paris` made it 18 training scenarios on 2026-08-30 — see the update below, which supersedes this line's counts.)*

**Update, 2026-08-30 (S152).** `web-live-paris` brings the Scaleway training set
to 18 — 19 counting the holdout. It needs a status this table did not previously
have to express. Nothing gates it: every resource it declares is allowlisted and
the key permits them, exactly as for `lb-serving-paris`. But it **has not been
run**, and the totals line below counts `**runnable**` rows as scenarios that
*have run*. Marking it `**runnable**` would therefore have made this document
claim a real-cloud run that never happened, so it is recorded as
`runnable, unrun` and is deliberately absent from the totals. The row becomes
`**runnable**` on the day it goes green, and not before.

**Corrected same day, by running it.** `web-live-paris` is *not* ungated. Its
first generation emitted `scaleway_instance_private_nic`, because
`pitfalls/scaleway.yaml` instructs the generator to attach a private NIC to every
`scaleway_instance_server`, and it is recorded as **key only**.

**The blocker is not `IPAMFullAccess`.** That was this document's diagnosis and it
was wrong; a canary on 2026-08-30 asked the real API, which wanted
`write compute_private_networks` — `PrivateNetworksFullAccess`, granted the same
day. Private *networks* now create. Private *NICs* still do not, and no permission
can fix it: `scaleway_instance_private_nic` takes no `project_id`, so it is created
in the provider default project — `scaleway.fallback_project_id`, the ADR-0010
containment project — while the server lives in the run's own project, and the API
refuses the mismatch. **Blast-radius containment and private NICs are mutually
exclusive as things stand.**

**A second claim made the same day, that `vpc_required.rego` is "wired for AWS
only", was also wrong and is retracted.** It is absent from
`constraint_policies`, but that is a different mechanism:
`filterPolicyPathsByCloud` drops only *other* clouds, so every
`policies/scaleway/*.rego` — all five, `vpc_required` included — is evaluated
against a Scaleway plan. A generation run the same day failed on exactly that
rule (`scaleway_instance_server.web is not attached to a private network`).

So the real position was a **contradiction between the two gates**, not a missing
permission: Layer 1 required a private NIC on every instance server, and Layer 3
could not apply one. No Scaleway compute scenario satisfied both, which is what
actually blocked `web-live-paris` and every other compute scenario equally.

**Resolved 2026-08-31 by the S166+S167 cutover (ADR-0025)**: the run's project is
created before the apply and is the provider default, so the NIC lands with its
server. The counts below are from before that and are stale in their *reason* —
what remains for each compute scenario is cost, not a contradiction. Ungated is
3 of 18, not 4; have-run is 3 of 18. The `runnable, unrun` bucket now has no
members and is kept because the state is real and will recur.

Two things are worth keeping from how this was found. The claim came from reading
the allowlist and concluding nothing blocked it; the correction came from
actually generating the scenario, which is the same lesson S139–S143 paid for at
much greater length. And the pitfall that forces the NIC cites the `vpc_required`
policy, which is **not** in Scaleway's `constraint_policies` — it is wired for AWS
only for the `constraint_policies` mapping — but that is a different mechanism,
and the rego IS evaluated for Scaleway via the per-cloud directory walk. See the
retraction at the top of this document: the blocker is the contradiction between
the two gates, not a permission and not the pitfall.

`incremental-project-paris` had looked like a fourth: admitting the Instances types cleared its allowlist gate. It is not. The scenario declares private networking and its generated HCL creates `scaleway_instance_private_nic`, so it still needs private networking, which is unresolvable today — it swapped blockers rather than losing one. (Recorded here as IPAM at the time; see the retraction at the top.)

(The earlier "3 of 16" here counted `lb-serving-paris` in the numerator and not the denominator: adding it made 16 into 17.)

`lb-serving-paris` is the first scenario to satisfy an `http_probe` against real Scaleway. It goes green end to end in **144 seconds** — apply, HTTP 200 through the load balancer frontend, destroy, orphan sweep — and it is the scenario that surfaced the auto-created security group defect (ADR-0023, second amendment of this date).

Fifteen scenarios remain gated, and what gates them is unchanged and
unchanged deliberately:

- **Cost/time.** `scaleway_k8s_*`, `scaleway_rdb_instance`, `scaleway_redis_cluster` stay commented out. They take minutes to create *and* minutes to destroy, on every iteration of the repair loop.
- **Policy.** `scaleway_iam*`, `scaleway_registry_namespace` and `scaleway_domain*` pass the allowlist and are refused by the API with a 403. That is the credential doing its job. `scaleway_domain*` is also, now, the `iam-scope` case in the plan-lied corpus — it is deliberately never granted.
- **Private networking.** `PrivateNetworksFullAccess` was granted 2026-08-30 and private networks now create; `IPAMFullAccess` remains ungranted and is **not** what blocks private NICs. See the retraction at the top: no permission unblocks them.

`scaleway_instance_private_nic` is allowlisted alongside the server, because `policies/scaleway/vpc_required.rego` denies any instance server without one — admitting the server but not the NIC would leave static policy demanding a resource the allowlist forbids, and no generated HCL could satisfy both. It sits in the "allowed locally, refused by the API" group permanently, not pending a grant: see the retraction at the top.

That distinction matters for reading the count above. `lb-serving-paris` is runnable **through the gate**, which uses `infrafactory test` against fixed HCL and does not run the static layer. Driving the same scenario through `infrafactory run` — generate, then validate — additionally needs private networking, which is unresolvable today (see the retraction at the top).
