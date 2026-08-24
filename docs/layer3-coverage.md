# Layer 3 coverage: which Scaleway scenarios can run against the real API

Audit date: 2026-08-24. **Refreshed** after `scaleway_instance_ip` and
`scaleway_instance_server` were admitted to the allowlist so a load
balancer could have a backend that serves — see the allowlist amendment
in ADR-0023. Derived from the **actually generated HCL** in
`.infrafactory/runs/<scenario>/<run_id>/generated/*.tf`, not inferred from
`mappings.yaml` — every one of the 16 Scaleway scenarios has prior run
artifacts.

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
   `scaleway_instance_ip`, `scaleway_instance_server`.
2. **The `infrafactory-layer3` IAM policy** — `ProjectManager`,
   `BlockStorageFullAccess`, `LoadBalancersFullAccess`, `VPCFullAccess`,
   `InstancesFullAccess`. Nothing else (ADR-0023, credential amendments).

They do not agree, deliberately. Three allowlisted families —
`scaleway_iam*`, `scaleway_registry_namespace`, `scaleway_domain*` — pass the
local check and are then refused by the API with a 403. That looks like a bug
if you do not know it; it is the credential doing its job.

## Coverage

| Scenario | Status | Class | Blocked on |
|---|---|---|---|
| `block-paris` | **runnable** | instant | — (run 2026-08-22) |
| `lb-paris` | **runnable** | hourly | — (run 2026-08-23) |
| `incremental-project-paris` | **allowlist only** | hourly | allowlist: `instance_*` — the key already permits it |
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

**2 runnable, 1 blocked by the allowlist alone, 4 by the key alone, 9 by both.**

`incremental-project-paris` is the one that moved when Instances was granted:
the credential now permits it and only the deny-by-default allowlist stands in
the way. That is the two-gate design working — granting a permission did not
silently enable a scenario.

Class is provisioning cost, not money: *instant* is seconds and negligible
(project, block volume, VPC, registry namespace); *hourly* bills for as long as
it exists (load balancer, instance); *slow + expensive* takes minutes to
provision and destroy and costs materially more (RDB, Redis, k8s). No euro
figures here on purpose — ADR-0010 deferred cost estimation because there is no
reliable Scaleway pricing source to code against, and inventing numbers would
be worse than none.

## The finding: the cheap end of the pool is empty

The two runnable scenarios are the two already run. There is no free
expansion, and three of the four "key only" entries are not the easy wins they
look like:

- **`domain-paris` is not a permission problem.** The account holds no
  registered public domain — only the auto-created `privatedns` zone for
  private networks. `scaleway_domain_zone` needs a real one, so this is a
  purchase, not a policy line.
- **`iam-policies-paris` and `public-registry-iam-paris` should stay
  blocked.** Granting IAM to the sandbox credential defeats the credential:
  a sandbox that can mint API keys is not a sandbox. Keep them as Layer 2
  scenarios.
- **`registry-paris` is the only defensible near-free expansion** — one
  permission set, instant provisioning. Note it still widens org-scoped
  registry access over the existing `funcscwblognolj7nc9` namespace, because
  product permission sets are project-scoped and per-run projects do not exist
  yet, so the rules must be organization-scoped.

Everything else needs Instances, RDB, Redis or Kubernetes — both gates widened,
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

**Runnable: 3 of 16.** `block-paris`, `lb-paris`, and the new `lb-serving-paris`.

`lb-serving-paris` is the first scenario to satisfy an `http_probe` against real Scaleway. It goes green end to end in **144 seconds** — apply, HTTP 200 through the load balancer frontend, destroy, orphan sweep — and it is the scenario that surfaced the auto-created security group defect (ADR-0023, second amendment of this date).

What still gates the remaining 13 is unchanged and unchanged deliberately:

- **Cost/time.** `scaleway_k8s_*`, `scaleway_rdb_instance`, `scaleway_redis_cluster` stay commented out. They take minutes to create *and* minutes to destroy, on every iteration of the repair loop.
- **Policy.** `scaleway_iam*`, `scaleway_registry_namespace` and `scaleway_domain*` pass the allowlist and are refused by the API with a 403. That is the credential doing its job. `scaleway_domain*` is also, now, the `iam-scope` case in the plan-lied corpus — it is deliberately never granted.
- **Private networking.** `IPAMFullAccess` has still not been granted. Add it when a scenario actually needs it, not before.
