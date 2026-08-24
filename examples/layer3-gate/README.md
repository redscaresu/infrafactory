# Layer 3 gate fixtures

The infrastructure the real-cloud PR gate applies
(`.github/workflows/layer3-gate.yml`).

**This is the point of the gate, not scaffolding for it.** A PR that changes
HCL in here gets that HCL applied to real Scaleway, verified and destroyed
before the PR can merge — which is the thing `terraform plan` cannot tell you.
Editing a file here and watching the gate is the demo.

The generator is deliberately not in this path. `infrafactory run` produces
HCL with an LLM in 40–60s with real variance; the gate applies **committed**
HCL, so it is deterministic and fast enough to watch. Generation is a separate
story, told separately.

Each subdirectory is named for its scenario, and its contents are copied into
the run's output directory before `infrafactory test` executes. The HCL must
satisfy the same rules any Layer 3 stack does:

- a `scaleway_account_project` resource (ADR-0010) — each run creates and
  destroys its own project, which is what bounds the blast radius
- only resource types in the gate's `allow_resource_types`
- a bare `provider "scaleway" {}` block: the endpoint comes solely from the
  sealed environment, which is what makes the same HCL appliable to both
  mockway and the real API
