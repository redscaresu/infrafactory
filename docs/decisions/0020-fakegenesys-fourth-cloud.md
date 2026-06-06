# ADR-0020: fakegenesys — Genesys Cloud CCaaS as the 4th cloud

Status: accepted
Date: 2026-06-06
Tags: architecture, multi-cloud, dispatch, schema

## Context

After the first three clouds (Scaleway, GCP, AWS) sustain-validated at 39/39 deterministic across two arcs, the project added a 4th cloud — Genesys Cloud CCaaS via the `mypurecloud/genesyscloud` Terraform provider. The motivation is documented in `docs/plans/fakegenesys-arc-plan.md` § "Big picture":

1. No existing OSS fake for the Genesys Terraform provider.
2. Generalizes infrafactory beyond IaaS (proves the architecture isn't accidentally tied to networking primitives).
3. Simpler wire format (REST/JSON, single OAuth grant) — a cleaner integration shape than the AWS / GCP / Scaleway predecessors.

The fakegenesys sibling-mock was built across 6 PRs (S108-S113); S114 wires it into infrafactory. The decision boundary this ADR records is the dispatch / schema / cloud-detection contract that integration crosses.

## Decision

Genesys is registered as a peer of `scaleway` | `gcp` | `aws` across every cloud-aware dispatch point:

- `internal/config/config.go`: `FakegenesysConfig{URL, AutoReset}` block parallel to `Fakegcp` / `Fakeaws`.
- `internal/cli/validate_command.go`: `knownClouds` includes `"genesys"` so policy-path filtering drops `policies/{aws,gcp,scaleway}/*` for Genesys scenarios.
- `internal/cli/mockway_client.go`: `cloudMockStateRouter.genesys` + `pick()` branches on `"genesys"` + `ResetAll` cascades.
- `internal/harness/topology_derive.go`: `detectCloud` probes for the `routing_queues` + `flows` + `schema_version` triple (Genesys check runs BEFORE the AWS check so they don't collide on `schema_version=1`).
- `internal/harness/topology_derive_genesys.go` (new): per-cloud deriver, emits `routing_queue:{id}` and `flow:{id}` connectivity entries. `http_probe` stays empty (CCaaS doesn't expose externally-reachable URLs).
- `scenario.schema.json`: `cloud` enum += `"genesys"`; new `genesys_resource_anchors` field; new `$defs/genesys_resource` (loose CCaaS shape — `purpose` only, `additionalProperties: true` since CCaaS doesn't map to the IaaS resource templates).
- `prompts/genesys/*.md` (3 phase prompts), `policies/genesys/*.rego` (3 policies), `pitfalls/genesys.yaml` (empty for cold-start auto-learning test).
- `scenarios/training/genesys-*.yaml` × 5.
- `infrafactory.yaml`: `fakegenesys` block at `:8083`. `Makefile`: `fakegenesys-up/-down/-restart` targets + `mocks-up` / `mocks-down` cascade + `sweep-39` → `sweep-N` rename.

## Why a new $def vs. reusing IaaS resource keys

`scenario.schema.json::$defs/genesys_resource` is deliberately loose (`additionalProperties: true`, only `purpose` required) because Genesys CCaaS doesn't map onto the IaaS-shaped resource templates (`compute`, `networking`, `database`, ...). Reusing the IaaS keys would either misclassify the LLM's planning surface (a Genesys queue is not "compute") or force every scenario to thread an irrelevant size field. The loose shape lets the LLM derive resources from `description` + the prompt rather than the structured `resources` dict — which is the same path Genesys integrations would take in production.

## Detection ordering

`detectCloud` checks Genesys (`schema_version + routing_queues + flows`) BEFORE AWS (`schema_version + iam + s3`) because both mocks set `schema_version=1`. The Genesys probe requires `routing_queues` to be present, which fakeaws never emits. Without the explicit ordering an AWS-shaped state with the keys `schema_version` and `iam` could accidentally pattern-match if Genesys added an `iam` key in the future. The defensive ordering documents the invariant.

## Alternatives considered

1. **Reuse the existing `iam` resource key for Genesys auth_role + oauth_client**. Rejected: the IaaS `iam_resource` defines `purpose` + `application` + `api_key` + `policy` flags that don't map cleanly onto Genesys's role/grant model.
2. **Drop the `resources` dict for Genesys scenarios entirely and rely on `description` + `genesys_resource_anchors`**. Rejected: schema's `allOf` block requires `resources` for non-holdout scenarios, and the LLM uses the dict as a mapping cue. The loose `genesys_resource` $def preserves that cue without forcing fictitious IaaS shapes.
3. **A fifth `cloudMockStateRouter.ccaas` slot vs. naming it `genesys`**. Rejected: the project's naming convention is one cloud per slot (`gcp` / `aws` / `genesys`), not by service category.

## Consequences

- Future per-cloud dispatch points only need a single literal added (`"genesys"`).
- Schema additions are backward-compatible — existing scenarios don't change.
- The cold-start auto-learning test in S115 is now well-defined: `pitfalls/genesys.yaml` is empty and the loop has 3 sweeps to populate it.

## Related

- `docs/plans/fakegenesys-arc-plan.md` (the arc plan).
- `docs/decisions/0019-learning-system-vocabulary.md` (the renamed pitfall vocab fakegenesys inherits).
- fakegenesys repo: `https://github.com/redscaresu/fakegenesys` (S108-S115 archive in its own ARCHIVE).
