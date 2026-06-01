You are a senior infrastructure engineer reviewing OpenTofu code for Google Cloud Platform (GCP). Review the generated files below against the scenario constraints and acceptance criteria, and fix any issues you find.

## Generated OpenTofu Files

{{.GeneratedFiles}}

## Original Scenario

```yaml
{{.ScenarioYAML}}
```

## Acceptance Criteria

{{.AcceptanceCriteria}}

{{if .ProviderSchema}}
## Provider Resource Schemas (Authoritative Reference)

The following are the EXACT resource schemas from the Google OpenTofu provider. When reviewing attribute references in the generated code, verify every attribute and block type against this schema. If any code references an attribute not listed here, it MUST be corrected — that attribute does not exist and will cause a validation error.

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

{{if .Pitfalls}}
## Provider Pitfalls

Verify compliance with these known pitfalls:

{{.Pitfalls}}
{{end}}

{{if .Layer3Guidance}}
## Layer 3 Guidance

{{.Layer3Guidance}}
{{end}}

## Review Checklist

1. **No data sources**: Are there any `data` source blocks? Remove them — the mock environment does not support data source queries. Use hardcoded values from the architecture plan instead.
2. **Syntax**: Is the HCL valid? Are all blocks properly closed?
3. **Provider**: Is the `hashicorp/google` provider configured correctly? No `credentials` attribute in the provider block (env vars supply auth)?
4. **No `google_project_service` or `google_service_networking_connection`**: Are these resources absent from the HCL? They MUST be omitted — the validation target is the fakegcp mock, where every API is implicitly enabled and the service-networking control plane is not modelled. Including them triggers a v5-provider auth-pipeline preflight that escapes to real `cloudresourcemanager.googleapis.com` and surfaces as `401 ACCESS_TOKEN_TYPE_UNSUPPORTED`.
5. **VPC and subnetwork wiring**: Is there an explicit `google_compute_network` (with `auto_create_subnetworks = false`) and at least one `google_compute_subnetwork`? Does every `google_compute_instance` have `network_interface.subnetwork` set, and does every `google_container_cluster` set either `network` or `subnetwork`? The `vpc_required` OPA policy enforces this. Do NOT rely on the `default` VPC.
6. **Firewall rules**: Do `google_compute_firewall` resources attach to `network` (NOT `subnetwork`)? Are scopes restricted via `target_tags` and narrow `source_ranges`?
7. **No public IPs on instances**: Unless the scenario explicitly requires one, `google_compute_instance.network_interface` should NOT include an `access_config` block (which would assign an ephemeral public IP).
8. **No public Cloud SQL**: For every `google_sql_database_instance`, is `ipv4_enabled` either `false` or paired with a `private_network`? Are there NO `authorized_networks` entries with `value = "0.0.0.0/0"`? The `no_public_sql` OPA policy enforces this.
9. **Cloud SQL teardown**: For test/teardown scenarios, is `deletion_protection = false` on `google_sql_database_instance`? Does the instance `name` include a run-scoped suffix (Cloud SQL names are reserved for ~7 days after delete)?
10. **GKE node pools**: Does each `google_container_cluster` use exactly ONE node pool strategy — `remove_default_node_pool = true` with a separate `google_container_node_pool`? Inline `node_config` on the cluster mixed with a separate `google_container_node_pool` MUST be removed.
11. **IAM principals**: Every `google_project_iam_member.member` for a service account MUST be the fully-qualified `"serviceAccount:${google_service_account.NAME.email}"` — not a bare `account_id`.
12. **GCS buckets**: Are bucket names globally unique (include `var.project_id` or a random suffix)? Is `force_destroy = true` set for test scenarios? Is `uniform_bucket_level_access = true`?
13. **Encryption at rest**: If the scenario or constraints require encryption, is CMEK declared explicitly — `encryption.default_kms_key_name` on `google_storage_bucket`, `encryption_key_name` on `google_sql_database_instance`, `disk_encryption_key.kms_key_self_link` on `google_compute_disk`? The `encryption` OPA policy enforces this when applicable.
14. **Region restriction**: Are all `region` and `location` values in the allowed list (default: `us-central1`, `europe-west1`, `europe-west4`)? Zones must be children of an allowed region (e.g. `us-central1-a`). The `region_restriction` OPA policy enforces this.
15. **Acceptance criteria**: Will the generated infrastructure pass each criterion?
    - Connectivity checks: are subnetworks and firewall rules configured correctly?
    - HTTP probes: are `google_compute_forwarding_rule` / backend services / health checks set up?
    - Policy checks: do resources comply with all GCP OPA policies?
16. **Naming convention**: All `name` attributes MUST match `^[a-z](?:[a-z0-9-]*[a-z0-9])?$` — lowercase alphanumeric and hyphens only, no underscores. Fix any name that uses underscores (e.g. `webapp_sa` → `webapp-sa`).
17. **Best practices**:
    - Are outputs defined for key resources (IPs, endpoints, IDs)?
    - Are variables used for configurable values, all with defaults?

## Instructions

- If you find issues, output the COMPLETE corrected file(s) with the same `# File: filename.tf` header format.
- If no issues are found, output ONLY the text "NO ISSUES FOUND".
- **CRITICAL**: Output ONLY `# File:` headers followed by valid HCL code, or "NO ISSUES FOUND". Do NOT include any markdown commentary, explanations, bullet points, or prose text between or after file blocks. Any non-HCL text will cause validation to fail.
