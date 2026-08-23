# Layer 3 coverage: which Scaleway scenarios can run against the real API

Audit date: 2026-08-23. Derived from the **actually generated HCL** in
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
   `scaleway_domain*`, `scaleway_iam*`, `scaleway_registry_namespace`.
2. **The `infrafactory-layer3` IAM policy** — `ProjectManager`,
   `BlockStorageFullAccess`, `LoadBalancersFullAccess`, `VPCFullAccess`.
   Nothing else (ADR-0023, credential amendment).

They do not agree, deliberately. Three allowlisted families —
`scaleway_iam*`, `scaleway_registry_namespace`, `scaleway_domain*` — pass the
local check and are then refused by the API with a 403. That looks like a bug
if you do not know it; it is the credential doing its job.

## Coverage

| Scenario | Status | Class | Blocked on |
|---|---|---|---|
| `block-paris` | **runnable** | instant | — (run 2026-08-22) |
| `lb-paris` | **runnable** | hourly | — (run 2026-08-23) |
| `registry-paris` | key only | instant | Registry |
| `iam-policies-paris` | key only | instant | IAM |
| `public-registry-iam-paris` | key only | instant | IAM + Registry |
| `domain-paris` | key only | instant | DomainsDNS **+ a registered domain** |
| `compute-lb-multi-paris` | allowlist + key | hourly | Instances, IPAM |
| `incremental-project-paris` | allowlist + key | hourly | Instances |
| `k8s-cluster-paris` | allowlist + key | slow + expensive | Kubernetes |
| `k8s-medium-override-paris` | allowlist + key | slow + expensive | Kubernetes |
| `mysql-ha-paris` | allowlist + key | slow + expensive | Instances + RDB |
| `private-lb-db-paris` | allowlist + key | slow + expensive | Instances + RDB |
| `redis-paris` | allowlist + key | slow + expensive | Redis |
| `redis-xlarge-session-paris` | allowlist + key | slow + expensive | Instances + Redis |
| `web-app-paris` | allowlist + key | slow + expensive | Instances, RDB, DomainsDNS, IPAM, VPCGateway |
| `full-stack-paris` | allowlist + key | slow + expensive | IAM, Instances, K8s, RDB, Redis, Registry |

**2 runnable, 4 blocked by the key alone, 10 blocked by both.**

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
# resource types per scenario, from real generated HCL
for s in $(grep -l 'cloud: scaleway' scenarios/training/*.yaml | xargs -n1 basename | sed 's/.yaml//'); do
  echo "$s: $(grep -ho 'resource "[a-z0-9_]*"' .infrafactory/runs/$s/*/generated/*.tf 2>/dev/null \
    | sort -u | tr '\n' ' ')"
done
```

Re-run it after any change to the allowlist, the IAM policy, or a scenario's
`resources:` block.
