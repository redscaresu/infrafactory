---
kind: code
status: ready
epic: aws-layer3-gate
depends_on: []
touches: ["internal/cli/layer3_hcl_shape.go", "internal/cli/layer3_gate_shared_rules_test.go (new)", "docs/stories/layer3-gate-shared-rules.md (delete)"]
risk: high
---

# The Scaleway gate's rules become named, cloud-parameterised functions a second cloud can call, terraform {} becomes deny-by-default, and a Scaleway stack refuses any type without the scaleway_ prefix

**Already done by #348 (2026-09-28):** the Scaleway gate's `terraform {}` block is deny-by-default (`layer3TerraformBlockProblems`: `required_providers` and `required_version` only). Build on it; do not redo it.

This story edits only internal/cli/layer3_hcl_shape.go, and the aws arm keeps refusing (layer3_cloud.go:35-41). Line numbers are main's.

Scaleway behaviour changes, named:
(1) Every child of a terraform block is on an allowlist, checked by layer3TerraformBlockProblems: the block may hold required_providers and the attribute required_version, and nothing else. encryption (key_provider "external" runs a command; aws_kms uses the ambient credential), provider_meta, experiments, backend, cloud and any unknown child are refused. Today only backend and cloud are refused (layer3DeniedNestedBlocks :58-84), and layer3ProviderSourceProblems skips every other child (:549-551). The lead's in-flight branch layer3-terraform-block-allowlist stages the same function; whichever merges first keeps it, and the other drops it.
(2) A resource type without the scaleway_ prefix is refused even when allow_resource_types lists it (:273-275). No Scaleway outcome changes today, because every default entry is scaleway_ (config.go:314-328).

Refactors, with no other behaviour change:
- layer3ParseDir(outputDir) returns (map[string]*hclsyntax.Body keyed by file base name, map[string]cty.Value, error). It takes over the parse loop at :173-218.
- Inline rules are hoisted into named funcs so the parity test can see them: layer3TopLevelBlockProblem (:266-268), layer3ResourceTypeProblem(prefix) (:273-275), layer3ProjectCountProblem (:240-244), layer3MissingProviderProblem(pin) (:245-252), and layer3ProviderBlockProblems(block, file, label, safeAttrs) (:280-291). The doc comment of layer3ProviderBlockProblems says it checks attributes only.
- layer3ProviderSourceProblems and layer3ProviderVersionProblem (:527-584) take a pin {local name, source, version}.
- layer3FunctionCallProblems (:770-793) takes an exempt predicate. Scaleway passes nil.
- Delete layer3ProjectBindingProblems and its orphaned doc comment (:586-599, :684-723). Nothing calls it.

layer3_hcl_shape_test.go is not edited.

**Done when:**
- layer3TerraformBlockProblems, reached through validateLayer3HCLShape, refuses each of these in a Scaleway stack and names the terraform block: encryption with key_provider "external" (with a command), encryption with method "external", key_provider "aws_kms", provider_meta "scaleway", backend "s3", cloud, experiments = [], and an unknown block. terraform { required_version = ">= 1.6" } is admitted
- layer3ResourceTypeProblem, reached through validateLayer3HCLShape, refuses aws_instance and google_compute_instance in a Scaleway stack, naming the type, both when the allowlist lists it exactly and when it lists aws_*. The same stack without that resource passes
- An AST test lists every string literal in validateLayer3HCLShape, layer3ParseDir and layer3BlockProblems, and fails on any literal outside a fixed non-refusal list (block type names, file suffixes, error-wrap formats). Planting an inline refusal string in a fixture copy of a driver fails it
- layer3ProviderSourceProblems with the pin (aws, hashicorp/aws, harness.AWSProviderVersion) admits that exact entry and refuses attacker/aws, ~> 5.100 and a missing version. With the Scaleway pin it returns golden strings identical to today's messages
- layer3FunctionCallProblems with a predicate exempting one attribute skips only that expression: file() is still refused in a sibling attribute, a nested block and a variable default. With nil it refuses file() everywhere
- layer3_hcl_shape_test.go, layer3_undestroyable_test.go, layer3_allowlist_test.go, vpc_required_lockstep_test.go and layer3_cloud_test.go pass with zero lines changed
- doc-hygiene passes, and the tip commit carries 'ADR: none — hoists ADR-0023 gate rules into named funcs; the terraform-block allowlist is recorded by ADR-0023's 2026-09-28 note'
