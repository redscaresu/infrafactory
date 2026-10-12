---
kind: code
status: blocked
blocked_by: [rehearsal-prepare-command-group]
epic: rehearsal-prepare-gate
depends_on: [rehearsal-prepare-command-group]
touches: [internal/cli/rehearse_gate.go, internal/cli/rehearse_gate_aws.go, internal/cli/rehearse_gate_paths.go, internal/cli/rehearse_gate_modules.go, internal/cli/rehearse_gate_test.go, internal/cli/rehearsal_assumptions_test.go, docs/decisions/0028-factual-claims-are-tests-not-comments.md, .github/fakeaws-e2e-tests]
risk: high
---

# The rehearsal gate: parse every module directory, allow by name, with its own extension policy

**Shape.** New file rehearse_gate.go. Its input: root dir, module dirs, boundary, region and var file (the last three serve the module, path and aws rules). It parses with hclsyntax, never by scanning text, collects every problem, sorts them and returns one wrapped error naming file:line and address (model: layer3_hcl_shape.go:175-206). It reuses only the leaf helpers (layer3VariableDefaults, layer3ResolveConstant, layer3CallsIn) and reads layer3PureFunctions without editing it. It never calls or loosens validateLayer3HCLShape, layer3ParseDir or layer3UnreadableConfigExt. Every rule is an allowlist: what is not named is refused, an unparseable file is refused, and nothing is skipped.

**Rules.**
1. Files, per gated module dir: `.tf` and `.tfvars` are parsed. `.tf.json`, `.tofu`, `.tofu.json`, `.tfvars.json`, `.tftest.hcl`, `.tftest.json`, `.tofutest.hcl`, `.tofutest.json` and `.tfmock.hcl` are refused by name. A `terraform.d` entry is refused (tofu's implied local provider mirror; init 1 runs without -plugin-dir). Other files are data, admitted unparsed, because tofu's loader does not read them: that claim is pinned by the assumption test below. A `.tfvars` file holds only attributes whose values are literals; a block, a reference or a function call in one is refused.
2. A user override file (`override.tf*`, `*_override.tf*`, infrafactory_override.tf included) is refused.
3. terraform{} admits required_providers, required_version, and backend in the root only. encryption and cloud are refused by name; anything else is refused.
4. Top-level blocks admitted: terraform, provider, variable, locals, output, resource, data, module, moved, import, removed. check, ephemeral and anything else are refused by name.
5. Provider sources, from required_providers and implied type prefixes, must be hashicorp/{aws,random,null,tls,time,cloudinit,local,archive}. A required_providers local name that differs from its source's type is refused (so `x = { source = "hashicorp/aws" }` cannot dodge the aws rules), at root and in child modules.
6. Resource and data types: aws_* types and terraform_data are admitted. Each other provider admits a closed list, and an unnamed type is refused by name: random_{id,integer,password,pet,shuffle,string,uuid,bytes}; null_resource and data null_data_source; tls_{private_key,self_signed_cert,cert_request,locally_signed_cert} and data tls_public_key; time_{offset,rotating,sleep,static}; cloudinit_config and data cloudinit_config; local_file, local_sensitive_file and data local_file, local_sensitive_file; archive_file and data archive_file. So data http, external, terraform_remote_state, tls_certificate and any newer type (e.g. a local_command) are refused.
7. provisioner and connection are refused at any depth.
8. A provider block whose resolved source is hashicorp/aws, in a non-root module, is refused by name.
9. Provider blocks of the other admitted providers admit only `alias`.
10. Functions: a call is admitted only if its name is in layer3PureFunctions. Any name containing `::` (`core::file`, `provider::local::direxists`) is refused by name.

Three rules start closed, each in its own file, and later stories widen them: every provider "aws" argument except alias is refused (rehearse_gate_aws.go); file-family functions and local_file/archive_file path arguments are refused (rehearse_gate_paths.go); every module block is refused (rehearse_gate_modules.go). Fixtures are written in-test to t.TempDir().

**Assumption test (ADR-0028).** `TestAssumption_LoaderIgnoresUnknownSuffixes`, in rehearsal_assumptions_test.go, gated by e2e.SkipUnlessEnabled: a root with one valid `output` plus `a.hcl`, `a.tf.bak`, `notes.txt`, `x.tfvars.example` and `sub/b.tf` each holding broken HCL passes `tofu init -backend=false` and `tofu validate` offline (no providers). Add its row to ADR-0028's claim table and its name to `.github/fakeaws-e2e-tests` (sorted). That file and ADR-0028 are hot: this story is first in the chain gate-core, gate-modules, gate-aws-provider, first-init, second-init, each depending on the one before.

Tip: `ADR: none — ADR-0042 records the gate` (the ADR-0028 edit also satisfies doc-hygiene).

**Done when:**
- A table test has one case per rule. Each planted fixture is refused with a message naming its file and address.
- A fixture of admitted constructs passes: data, lifecycle, count/for_each, moved, import, removed, terraform_data, a literal-only terraform.tfvars, a README.md, and one admitted type of each of the eight providers.
- `terraform { encryption {...} }` in a .tf, a .tofu file, a .tf.json file and a .tftest.hcl file are each refused by name; `cloud {}` is refused by name; a `terraform.d/` dir is refused.
- zz_override.tf and infrafactory_override.tf are each refused; an unparseable .tf is refused; a .tfvars holding `region = var.x` is refused; `provider "aws" {}` in a child module dir is refused by name; `data "http"` nested inside a `check` block is refused.
- `required_providers { x = { source = "hashicorp/aws" } }` plus `provider "x" { endpoints {} }` is refused, at root and in a child module.
- An unnamed type of an admitted provider (`data "local_command"`, `resource "random_made_up"`) is refused naming the type.
- `core::file("/etc/passwd")` and `provider::local::direxists("/")` are each refused by name.
- While the closed rules hold, a provider "aws" `region` argument, a `file()` call and a `module` block are each refused.
- TestLayer3GateDriversHoldNoInlineRefusals and the Layer 3 gate tests pass with layer3_hcl_shape.go unchanged.
- `grep -x TestAssumption_LoaderIgnoresUnknownSuffixes .github/fakeaws-e2e-tests` hits; ADR-0028's table names it.
- Each new test, the assumption test included, is shown failing by a cp-backup break recorded in the PR.
