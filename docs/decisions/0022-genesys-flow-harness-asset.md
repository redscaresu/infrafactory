# ADR-0022: Pre-place `flow.yaml` in the workdir for `genesyscloud_flow` scenarios

Status: accepted
Date: 2026-06-06
Tags: harness, genesys, terraform

## Context

The `mypurecloud/genesyscloud` Terraform provider reads `genesyscloud_flow.filepath` at PLAN time via its `CustomizeDiff` hook (`resource_genesyscloud_flow.go`). This is unusual — most providers read external files during apply. The consequence is fatal for our smoke harness: no Terraform pattern (`local_file` + `depends_on`, `null_resource` provisioners, `terraform_data`) can satisfy the constraint because every such pattern evaluates AFTER plan.

Across sustain sweeps 1, 2, and 3 the `genesys-architect-flow` and `genesys-full-stack` scenarios exhausted their full 5-iteration repair budget for this single reason. S120's prompt-level guidance steered the LLM toward `local_file` + `depends_on` — the LLM did exactly what we asked, and it still couldn't work.

## Decision

When the LLM-generated HCL declares a `genesyscloud_flow` resource, infrafactory's generate step adds a default `flow.yaml` to the generated-files map BEFORE writing them to the workdir. The file is written alongside the `.tf` files in the same directory, so a bare `filepath = "${path.module}/flow.yaml"` resolves at plan time.

Three explicit constraints on the implementation:

1. **The harness only places ONE stub file** (`flow.yaml`). Scenarios with multiple flow resources reuse the same file. The smoke harness validates wire shape, not flow content; a single stub satisfies every flow create.
2. **LLM-provided `flow.yaml` is preserved.** If the LLM generates its own (e.g. via the inline-generate pattern that still works for non-architect scenarios), the harness does NOT overwrite it.
3. **The prompt is the LLM's source of truth for the filename.** `prompts/genesys/phase2_generate_hcl.md` § 11 names `flow.yaml` explicitly; if we ever rename the harness asset, the prompt must change in the same PR.

## Why a stub instead of asking the LLM to generate the YAML

The Genesys Architect Flow YAML schema is large (5+ pages of nested choices), validates strictly on real Genesys, and contributes nothing to what the smoke harness actually tests (wire-shape coverage + provider plan/apply lifecycle). Asking the LLM to author it adds tokens, error surface, and tail latency for zero coverage gain. The fakegenesys mock accepts any YAML the wire delivers; a fixed stub keeps the inner loop fast.

## Alternatives considered

1. **Bake the flow YAML into the scenario YAML** (a new `assets:` field). Rejected for this PR — generalises the concept across clouds (fakeaws Lambda zip, etc.) and deserves its own design ADR. The narrower fix lands the unblock first; the assets-section work can be a follow-up.
2. **Drop `genesyscloud_flow` from the architect-flow + full-stack scenarios**. Rejected: flow is one of the marquee Genesys resources and the scenarios were designed around it. A stub satisfies the resource without sacrificing scenario semantics.
3. **Patch the provider to read `filepath` at apply time instead of plan**. Rejected: not our code, would require vendoring.

## Consequences

- Sweeps after S122 land should converge architect-flow and full-stack alongside the other 3 genesys scenarios.
- Any future genesys scenario that uses `genesyscloud_flow` inherits the same auto-placement — no per-scenario work needed.
- The `assets:` section idea is now logically queued: when a second file-on-disk dependency appears (e.g. a fakeaws Lambda needing a deployment zip), revisit ADR-0022 and design the generalised mechanism.

## Related

- ADR-0020 (fakegenesys as the 4th cloud).
- ADR-0021 (auto-learning cloud-prefix set).
- infrafactory#98 (the fix).
- fakegenesys#16 (the bundled group-subresources fix for rbac-and-oauth).
