# Decision Records

This folder stores stable architecture decision records (ADRs).

Use ADRs for decisions that affect long-term behavior, interfaces, or contributor workflow.

## Index

<!-- adr-index:start -->
- [0001](0001-foundations.md) Foundations — Accepted
- [0002](0002-cli-command-contract.md) CLI Command Contract Freeze for Slice 7 — Accepted
- [0003](0003-permanent-sandbox-live-deploy-block.md) Permanent Sandbox/Live Deploy Block — **Superseded by ADR-0010**
- [0004](0004-generator-transport-contract.md) Generator Transport Contract in Config and Runtime — Accepted
- [0005](0005-dual-iteration-controls.md) Dual Iteration Controls for Run Loop — **Superseded by ADR-0006**
- [0006](0006-run-failure-only-retry-control.md) Run Loop Uses Failure-Only Retry Control — Accepted
- [0007](0007-scenario-schema-resource-expansion.md) Scenario Schema Resource Expansion (Slice 18) — Accepted
- [0008](0008-ui-command-and-noui-api-mode.md) UI Command and `noui` API-Only Mode — Accepted
- [0009](0009-incremental-deployment-model.md) Incremental Deployment Model — Accepted
- [0010](0010-layer3-real-scaleway-deploy.md) Layer 3 Real Scaleway Deploy — Accepted; supersedes ADR-0003
- [0011](0011-topology-derivation-layer.md) Topology Derivation Layer — Accepted
- [0012](0012-dynamic-pitfalls.md) Dynamic Pitfalls by Cloud Provider — Accepted
- [0013](0013-cross-repo-e2e-and-multi-cloud.md) Cross-Repo E2E Testing and GCP Multi-Cloud Support — Proposed
- [0014](0014-provider-endpoint-flag-discipline.md) Provider-Endpoint Flag Discipline for v5 GCP Provider — Accepted
- [0015](0015-classifier-routing.md) Classifier-routed failure handling at stuck/budget termination — Accepted
- [0016](0016-orphan-subshape-classification.md) Orphan-check sub-shape classification — Accepted
- [0017](0017-policy-pitfall-conflict.md) policy_pitfall_conflict detection — Accepted
- [0018](0018-n11-retirement-criteria.md) N11 prompt-rule retirement criteria — Accepted
- [0019](0019-learning-system-vocabulary.md) Learning-system vocabulary — concept names over slice IDs — Accepted
- [0020](0020-fakegenesys-fourth-cloud.md) fakegenesys — Genesys Cloud CCaaS as the 4th cloud — Accepted
- [0021](0021-cloud-prefix-set-in-auto-learning.md) Cloud-prefix set in the auto-learning pipeline — Accepted
- [0022](0022-genesys-flow-harness-asset.md) Pre-place `flow.yaml` in the workdir for `genesyscloud_flow` scenarios — Accepted
- [0023](0023-layer3-sealed-environment-and-orphan-verification.md) Layer 3 Sealed Environment and Real-Orphan Verification — Accepted; amends ADR-0010
- [0024](0024-live-deployments-bounded-by-mandatory-ttl.md) Live deployments are bounded by a mandatory TTL, and unreadable means expired — Accepted
- [0025](0025-run-project-created-before-the-apply.md) The run's project is created before the apply, not by it — Accepted
- [0026](0026-the-ui-api-answers-only-loopback-origins.md) the UI API answers only loopback origins — Accepted
- [0027](0027-deploying-from-the-ui.md) deploying from the UI — Accepted
- [0028](0028-factual-claims-are-tests-not-comments.md) a factual claim about another component is a test, not a comment — Accepted
- [0029](0029-a-policy-may-not-mandate-an-undestroyable-shape.md) A policy may not mandate a shape that cannot be destroyed — Accepted, mechanism refuted
- [0030](0030-teardown-powers-instances-off.md) Teardown powers the run's instances off before `tofu destroy` — **Superseded by ADR-0031**
- [0031](0031-the-nic-delete-defect-is-the-endpoint.md) The private-NIC teardown defect is the endpoint, not the power state — Accepted; supersedes ADR-0030
- [0032](0032-apply-succeeding-is-not-convergence.md) An apply that succeeds is not a stack that converges — Accepted
- [0033](0033-a-holdout-is-a-negative-check-against-the-running-stack.md) A holdout is a negative check, probed against the running stack — Accepted
- [0034](0034-a-prohibition-is-a-specification.md) A prohibition is a specification — Accepted
- [0035](0035-process-weight-follows-value.md) Process weight follows value — Accepted
- [0036](0036-destruction-is-a-layer-not-a-criterion.md) Destruction is a layer, not a criterion — Accepted
- [0037](0037-docs-is-the-planning-vault.md) docs/ is the planning vault, and epics are scoped and built by a visible swarm — Accepted
- [0038](0038-work-starts-from-an-agreed-hld.md) Work starts from an agreed high-level design — Accepted
- [0039](0039-layer-neutral-aws-generated-hcl.md) Layer-Neutral AWS Generated HCL — Accepted
- [0040](0040-aws-whole-scope-ownership.md) Whole-Scope Ownership for AWS Layer 3 — Accepted
<!-- adr-index:end -->

Generated from the ADR files by `make adr-index`; CI fails if it is stale. Do not edit between the markers.

- `DECISION_RUBRIC.md`: yes/no gate for deciding when ADR is required.
- `ADR_TEMPLATE.md`: copy/paste template for new ADRs.

## When to add an ADR

Add an ADR when a change affects one or more of:
- cross-package architecture or boundaries
- external tool/service contracts (OpenTofu, Mockway, OPA, CLI behavior)
- source-of-truth precedence or schema semantics
- long-term contributor workflow or governance
- irreversible or expensive-to-revert implementation choices

If unsure, run the rubric in `DECISION_RUBRIC.md`.

A new ADR needs a `# ADR-NNNN: Title` heading and a status (`## Status` section or `Status:` line). Then run `make adr-index`.

## ADR template

Use this structure for new ADRs:

```md
# ADR-XXXX: Title

## Status
Accepted | Proposed | Superseded

## Context
Problem and constraints.

## Decision
Chosen approach.

## Consequences
Benefits, tradeoffs, and follow-up work.
```
