# gcp-full-stack — `google_service_networking_connection` escape (S84)

Date: 2026-06-03
Slice: S84 (timeboxed investigation, ~30 min)
Outcome: root cause + fix path identified → proceed to S85 with LLM-side pitfall.

## Symptom

S81's sweep had gcp-full-stack `repair_budget_exhausted` after 5 iterations. Iter 4's failure detail:

```
Error: Failed to find Service Networking Connection
err: Failed to retrieve project, pid: infrafactory-test
err: googleapi: Error 401: ... "reason": "ACCESS_TOKEN_TYPE_UNSUPPORTED"
"method": "google.cloudresourcemanager.v1.Projects.GetProject"
"service": "cloudresourcemanager.googleapis.com"
```

`google_service_networking_connection.private_vpc_connection`'s Read flow does an internal `retrieveProject` call to resolve the project NUMBER (the SN connection API needs the project number, not the project ID, as the parent path). That internal call escaped to real cloud despite `cloud_resource_manager_custom_endpoint = "http://127.0.0.1:8081/v1/"` AND `resource_manager_v3_custom_endpoint = "http://127.0.0.1:8081/"` being set.

## Ruled out

1. **No per-resource provider block / alias.** Iter 4 `network.tf` line 33-37 has the SNC resource with standard arguments (`network`, `service`, `reserved_peering_ranges`) — no `provider = google.alias` attribute or alternative provider source.
2. **fakegcp's v1 + v3 GetProject handlers both work when probed directly.** `curl -H "Authorization: Bearer fake-token" http://127.0.0.1:8081/v1/projects/infrafactory-test` returns 200 with a valid Project shape; same for `/v3/projects/infrafactory-test`. fakegcp is not the issue.
3. **The `"reason": "ACCESS_TOKEN_TYPE_UNSUPPORTED"` is GCP-specific.** fakegcp's 401 responses use `"reason": "required"`. The error came from real `cloudresourcemanager.googleapis.com`, not fakegcp — confirming the request escaped.
4. **No missing override.** Iter 4 `providers.tf` sets every relevant `*_custom_endpoint` including `service_networking_custom_endpoint`, `resource_manager_v3_custom_endpoint`, and `cloud_resource_manager_custom_endpoint`. The block also disables IAM batching (the prior workaround for an analogous escape).

## Root cause

`terraform-provider-google` v5's `servicenetworking` package constructs its own internal `cloudresourcemanager` HTTP client to do the `retrieveProject` (project-id → project-number) lookup. That client does NOT honor the `cloud_resource_manager_custom_endpoint` setting from the global provider config — it uses the hardcoded `cloudresourcemanager.googleapis.com` host.

This is the same architectural shape as the IAM batching escape already worked around in providers.tf (`batching { enable_batching = false }`): the provider has multiple internal clients for cloudresourcemanager and not all of them route through the override.

For IAM, the workaround knob existed (`enable_batching = false`). For servicenetworking, **no equivalent provider knob exists** — there's no way to make the internal client honor the override.

## Critical comparison: gcp-cloud-sql passed

Same sweep, `gcp-cloud-sql` passed in iter 2 / 372s using a different shape:

```hcl
resource "google_sql_database_instance" "postgres" {
  ip_configuration {
    private_network = google_compute_network.main.id  # ← direct attachment
  }
}
```

**No `google_service_networking_connection` at all.** In real GCP, Cloud SQL private IP REQUIRES the SNC (it's the prerequisite that establishes the VPC peering). But **fakegcp accepts the SQL instance with `private_network` attached directly** — the mock isn't strict about the SNC prerequisite.

This is the gap: the LLM, applying real-GCP knowledge, generates SNC for any private-network Cloud SQL. But fakegcp neither needs nor honors it.

## Fix path → S85

**LLM-side pitfall.** Land a `learned` (M91-permitted) pitfall in `pitfalls/gcp.yaml` instructing the LLM:
- Do NOT use `google_service_networking_connection` to enable private Cloud SQL on fakegcp. Its Read flow escapes to real `cloudresourcemanager.googleapis.com` via an internal client that does not honor `cloud_resource_manager_custom_endpoint`.
- Instead set `ip_configuration { private_network = google_compute_network.<name>.id }` directly on `google_sql_database_instance`. fakegcp accepts this and routes through normal overrides.

Validation: re-run gcp-full-stack with the pitfall in place. Target: target_reached in ≤ 4 iters.

## Alternative paths considered + rejected

- **fakegcp-side fix.** Not possible — the request never reaches fakegcp.
- **infrafactory-side fix in generate_command.go.** No additional `*_custom_endpoint` to add; the escape is in a provider-internal client. The only knob would be to intercept DNS for `cloudresourcemanager.googleapis.com` (e.g. /etc/hosts), which is invasive and unportable.
- **Use Cloud SQL with public IP.** Violates the scenario's `private_network: true` requirement; loses test coverage.

## Notes for future

If the v5 provider ever ships an `enable_internal_client_endpoint_overrides = true` knob (or v6 fixes the architecture), this pitfall can be retired via the N11 protocol.

Related to ADR-0015 § "2026-06-02, S78 — GCP escape resource carve-out" — that arc already handled the routing side (N3 routes these failures to ExtractLearnedPitfall instead of mock-gaps). This investigation closes the LLM-side education that the carve-out depends on.
