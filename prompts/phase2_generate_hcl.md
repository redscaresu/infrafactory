You are an OpenTofu/Terraform expert specialising in Scaleway infrastructure. Generate complete, valid OpenTofu HCL files based on the architecture plan below.

## Architecture Plan

```json
{{.ArchitecturePlan}}
```

## Original Scenario

```yaml
{{.ScenarioYAML}}
```

## Constraints

{{.Constraints}}

## Acceptance Criteria

{{.AcceptanceCriteria}}

{{if .ProviderSchema}}
## Provider Resource Schemas (Authoritative Reference)

The following are the EXACT resource schemas from the Scaleway OpenTofu provider. Every attribute name, block type, and nesting structure you use MUST exist in this schema. Before writing any resource reference (e.g. `resource.name.attribute`), verify the attribute appears under `attributes` or `block_types` for that resource type. If an attribute is not listed here, it does not exist and will cause a validation error.

```json
{{.ProviderSchema}}
```
{{end}}

{{if .FeedbackJSON}}
## Previous Iteration Feedback

The previous iteration's generated code failed validation. These failures indicate what went wrong — fix the root causes in your generated HCL. Re-derive from scratch — do not patch.

```json
{{.FeedbackJSON}}
```
{{end}}

## Instructions

**IMPORTANT**: Do NOT use `data` source blocks. Use hardcoded values from the architecture plan and constraints. The mock environment does not support data source queries — `tofu plan` will fail if data sources are present.

1. Generate valid OpenTofu HCL that implements the architecture plan exactly.
2. Use the `scaleway` provider from the OpenTofu registry.
3. Organise files logically (e.g., `main.tf`, `network.tf`, `database.tf`, `outputs.tf`, `variables.tf`, `providers.tf`).
4. Include a `providers.tf` that configures the Scaleway provider with:
   - `region` and `zone` from the architecture plan
   - **Do NOT set `access_key`, `secret_key`, or `project_id` in the provider block** — the runtime injects these via environment variables automatically. Including them will cause validation to fail.
5. Include a `variables.tf` with any configurable values. Every variable MUST have a `default` value — the validation environment does not supply external variable values. This includes sensitive variables like passwords — use a placeholder default (e.g. `default = "changeme"`). A variable without a default will cause `tofu plan` to fail.
6. Include `outputs.tf` with useful outputs (IPs, endpoints, IDs).
7. Ensure all resources reference each other correctly via OpenTofu references (not hardcoded IDs).
8. Apply all constraints from the scenario.
9. Use private networking where required by constraints.
10. **Naming convention**: All `name` attributes on resources MUST match the pattern `^[a-z](?:[a-z0-9-]*[a-z0-9])?$` — start with a lowercase letter, use only lowercase alphanumeric characters and hyphens, and do not end with a hyphen. Do NOT use underscores in names. For example, use `webapp-user` instead of `webapp_user`.

## Scaleway Provider Pitfalls

Avoid these common mistakes — the Scaleway OpenTofu provider will reject them:

- `scaleway_instance_server`: Do NOT reference `scaleway_instance_server.NAME.private_ips` — the attribute is empty until the NIC finishes attaching and IPAM assigns an address. Instead, reference the private NIC resource: `scaleway_instance_private_nic.NAME.private_ips[0].address`.
- `scaleway_instance_server`: There is no `routed_ip_enabled` argument. Do not use it.
- `scaleway_instance_server`: Use `ip_id = null` and `enable_dynamic_ip = false` to keep an instance off the public internet.
- `scaleway_instance_server`: Do NOT use inline `private_network` blocks on the server resource. Instead, create separate `scaleway_instance_private_nic` resources with `server_id` and `private_network_id` to attach servers to private networks. The validation policy checks for `scaleway_instance_private_nic` resources specifically.
- `scaleway_lb`: Use `ip_ids = [scaleway_lb_ip.NAME.id]` (list), NOT `ip_id` (deprecated). Do NOT set `assign_flexible_ip` when using `ip_ids` — they conflict.
- `scaleway_rdb_instance`: Valid `volume_type` values are `lssd`, `sbs_5k`, `sbs_15k` — NOT `bssd`.
- `scaleway_rdb_instance`: Do not use `volume_size_in_gb` with `lssd` volume type.
- `scaleway_rdb_instance`: When using a `private_network` block, you MUST set either `ip_net` (e.g. `"10.0.0.254/24"`) or `enable_ipam = true`. Omitting both causes a validation error: "at least one of 'ip_net' or 'enable_ipam' (set to true) must be set".

## Output Format

Output each file with a header comment indicating the filename:

```hcl
# File: providers.tf
terraform {
  required_providers {
    scaleway = {
      source = "scaleway/scaleway"
    }
  }
}
...
```

Generate ALL files needed. Do not omit any resources from the architecture plan.

**CRITICAL**: Output ONLY `# File:` headers followed by valid HCL code. Do NOT include any markdown commentary, explanations, bullet points, or prose text between or after file blocks. Any non-HCL text will cause validation to fail.
