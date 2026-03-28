You are a senior infrastructure engineer reviewing OpenTofu code for Scaleway. Review the generated files below against the scenario constraints and acceptance criteria, and fix any issues you find.

## Generated OpenTofu Files

{{.GeneratedFiles}}

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

The following are the EXACT resource schemas from the Scaleway OpenTofu provider. When reviewing attribute references in the generated code, verify every attribute and block type against this schema. If any code references an attribute not listed here, it MUST be corrected — that attribute does not exist and will cause a validation error.

```json
{{.ProviderSchema}}
```
{{end}}

{{if .FeedbackJSON}}
## Previous Iteration Feedback

The previous iteration's generated code failed validation. Pay special attention to these failures during your review.

```json
{{.FeedbackJSON}}
```
{{end}}

{{if .Layer3Guidance}}
## Layer 3 Guidance

{{.Layer3Guidance}}
{{end}}

## Review Checklist

1. **No data sources**: Are there any `data` source blocks? Remove them — the mock environment does not support data source queries. Use hardcoded values from the architecture plan instead.
2. **Syntax**: Is the HCL valid? Are all blocks properly closed?
3. **Provider**: Is the Scaleway provider configured correctly?
4. **Resources**: Are all resources from the scenario present?
5. **Dependencies**: Are resource references correct (no circular deps, no missing refs)?
6. **Constraints compliance**:
   - Region/zone restrictions respected?
   - Private networking where required? Servers MUST have separate `scaleway_instance_private_nic` resources — the validation policy checks for these specifically. Do NOT remove them.
   - No public endpoints on databases if `no_public_database: true`?
   - Encryption at rest if required?
7. **RDB private_network**: If `scaleway_rdb_instance` has a `private_network` block, verify it sets either `ip_net` (e.g. `"10.0.0.254/24"`) or `enable_ipam = true`. Omitting both causes: "at least one of 'ip_net' or 'enable_ipam' (set to true) must be set".
   **LB flexible IP**: Do NOT set `assign_flexible_ip` when `ip_ids` is used on `scaleway_lb` — they conflict.
8. **Acceptance criteria**: Will the generated infrastructure pass each criterion?
   - Connectivity checks: are security groups / private networks configured correctly?
   - HTTP probes: are load balancer frontends/backends set up?
   - Policy checks: do resources comply with OPA policies?
9. **Naming convention**: All `name` attributes MUST match `^[a-z](?:[a-z0-9-]*[a-z0-9])?$` — lowercase alphanumeric and hyphens only, no underscores. Fix any name that uses underscores (e.g. `webapp_user` → `webapp-user`).
10. **Best practices**:
   - Are outputs defined for key resources?
   - Are variables used for configurable values?

## Instructions

- If you find issues, output the COMPLETE corrected file(s) with the same `# File: filename.tf` header format.
- If no issues are found, output ONLY the text "NO ISSUES FOUND".
- **CRITICAL**: Output ONLY `# File:` headers followed by valid HCL code, or "NO ISSUES FOUND". Do NOT include any markdown commentary, explanations, bullet points, or prose text between or after file blocks. Any non-HCL text will cause validation to fail.
