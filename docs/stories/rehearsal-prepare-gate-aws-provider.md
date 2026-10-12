---
kind: code
status: blocked
blocked_by: [rehearsal-prepare-gate-core, rehearsal-prepare-gate-modules]
epic: rehearsal-prepare-gate
depends_on: [rehearsal-prepare-gate-core, rehearsal-prepare-gate-modules]
touches: [internal/cli/rehearse_gate_aws.go, internal/cli/rehearse_gate_aws_test.go, internal/cli/rehearse_gate_aws_schema_test.go, internal/cli/rehearsal_assumptions_test.go, internal/cli/export_test.go, docs/decisions/0028-factual-claims-are-tests-not-comments.md, .github/fakeaws-e2e-tests]
risk: high
---

# provider "aws" argument allowlist tied to the 5.100.0 schema, and goal 11 on the resolved region

depends_on rehearsal-prepare-gate-modules only for the hot-file chain (`.github/fakeaws-e2e-tests`, ADR-0028, rehearsal_assumptions_test.go): gate-core, gate-modules, this story, first-init, second-init.

**Table.** Widen rehearse_gate_aws.go. The rules apply to every provider block whose resolved source is hashicorp/aws (gate-core forces its local name to `aws`). The table names every attribute and nested block of provider "aws" in the 5.100.0 schema, each with a verdict. Admitted, where the schema has them: alias, default_tags, ignore_tags, region (by the rule below), allowed_account_ids, forbidden_account_ids, max_retries, retry_mode, skip_credentials_validation, skip_metadata_api_check, skip_requesting_account_id, s3_use_path_style. Every other key is refused, including endpoints, http_proxy, https_proxy, no_proxy, shared_config_files, shared_credentials_files, profile, access_key, secret_key, token, assume_role, assume_role_with_web_identity, custom_ca_bundle, ec2_metadata_service_endpoint*, insecure, sts_region, use_fips_endpoint and use_dualstack_endpoint. An argument the table does not name is refused under any lock version, so prepare makes no runtime schema call.

**Goal 11, statically.** Every root provider "aws" block, aliased ones included, must carry a `region`. It is a literal, or var.x resolved by tofu's precedence: the variable default, then terraform.tfvars, then *.auto.tfvars in lexical order, then the --var-file. Anything else (locals, functions, data, a variable with no value) is refused naming it. An absent region is refused naming the block, and a root that uses aws_* types with no provider "aws" block is refused (the implied empty block has no region). A resolved region other than --region is refused. The same rule covers a `region` argument on any aws_ resource or data block (6.x locks).

**Assumption test (ADR-0028).** The precedence above is a tofu claim. `TestAssumption_VariablePrecedenceMatchesGate`, in rehearsal_assumptions_test.go, gated by e2e.SkipUnlessEnabled: an outputs-only root (no providers, offline) with a variable default, terraform.tfvars, `a.auto.tfvars` and `b.auto.tfvars`, applied with and without `-var-file`, and `tofu output -raw` read back: the var file wins, and without it `b.auto.tfvars` wins. Add its ADR-0028 row.

**Drift test.** `TestRehearsalAWSProviderTableMatchesSchema` in rehearse_gate_aws_schema_test.go (package cli_test), gated by e2e.SkipUnlessEnabled. It uses harness.ExtractProviderSchemaForCloud (provider_schema.go:54-107; precedent internal/e2e/aws_allowlist_schema_test.go), with the table exported through export_test.go. Add both new e2e names to `.github/fakeaws-e2e-tests` (sorted), so CI runs them.

Tip: `ADR: none — ADR-0042 records the provider allowlist and goal 11` (the ADR-0028 edit also satisfies doc-hygiene).

**Done when:**
- The drift test fails when the 5.100.0 schema holds a key the table does not name, and when the table names a key the schema lacks. Shown red by deleting one table row (cp-backup).
- `endpoints {}`, `assume_role {}`, `profile` and `https_proxy` are each refused by name; an argument absent from the table (e.g. `made_up_arg`) is refused naming the table's version; `default_tags` and `ignore_tags` are admitted.
- With --region us-east-1, each of these is refused: `variable "region" { default = "us-east-1" }` plus a --var-file setting `region = "eu-west-1"`; the literal `"eu-west-1"`; `local.region`; an aliased provider with another region; a provider "aws" block with no region; aws_* resources with no provider "aws" block. A matching literal, a matching terraform.tfvars and a matching auto.tfvars are each admitted.
- A `region = "eu-west-1"` on an aws_ resource is refused.
- `grep -x` finds TestAssumption_VariablePrecedenceMatchesGate and TestRehearsalAWSProviderTableMatchesSchema in `.github/fakeaws-e2e-tests`; ADR-0028's table names the assumption.
- Each new test is shown failing by a cp-backup break recorded in the PR.
