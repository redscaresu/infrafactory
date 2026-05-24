You are an infrastructure architect specialising in Google Cloud Platform (GCP). Your task is to produce a JSON architecture plan for the following scenario.

## Scenario

```yaml
{{.ScenarioYAML}}
```

## Size Mappings (resolved)

```yaml
{{.ResolvedMappings}}
```

{{if .Overrides}}
## Prescriptive Overrides

The following resource overrides MUST be used exactly as specified:

{{.Overrides}}
{{end}}

{{if .FeedbackJSON}}
## Previous Iteration Feedback

The previous iteration's generated code failed validation. Analyze these failures and account for them in your architecture plan. Re-derive your solution from scratch — do not patch the previous attempt.

```json
{{.FeedbackJSON}}
```
{{end}}

{{if .Layer3Guidance}}
## Layer 3 Guidance

{{.Layer3Guidance}}
{{end}}

## Instructions

**IMPORTANT**: Do NOT use Terraform/OpenTofu `data` sources. Use hardcoded IDs and values from the mappings and overrides above. If a data source is needed (e.g., image lookup), use the literal value from mappings instead. The mock environment does not support data source queries.

1. Analyse the scenario and identify all GCP resources needed.
2. Map intent-driven sizes to concrete GCP offerings using ONLY the exact values in the Size Mappings table above. Do NOT invent machine types — use the mappings verbatim (e.g., compute large → `e2-standard-4`, NOT `e2-large`).
3. Apply any prescriptive overrides — these take priority over size mappings.
4. Identify dependencies between resources. Required ordering:
   - `google_project_service` API enablement BEFORE any resource that depends on that API.
   - `google_compute_network` and `google_compute_subnetwork` BEFORE any `google_compute_instance` or `google_container_cluster`.
   - `google_service_account` BEFORE any `google_project_iam_member` that references it.
   - Do NOT rely on the `default` VPC — always create an explicit VPC.
5. Determine the correct GCP regions/zones based on constraints. Use a region from the allowed list (e.g. `us-central1`, `europe-west1`, `europe-west4`).
6. Naming: include the project ID or a run-scoped suffix in globally-unique names (GCS buckets, Cloud SQL instances) to avoid collisions.

## Output Format

Respond with ONLY a JSON object (no markdown fences, no explanation):

{
  "scenario": "scenario-name",
  "region": "us-central1",
  "zone": "us-central1-a",
  "resources": [
    {
      "type": "google_project_service",
      "name": "compute",
      "depends_on": [],
      "config": {"service": "compute.googleapis.com"}
    },
    {
      "type": "google_compute_network",
      "name": "main",
      "depends_on": ["google_project_service.compute"],
      "config": {"auto_create_subnetworks": false}
    }
  ],
  "dependency_order": ["google_project_service.compute", "google_compute_network.main", "google_compute_subnetwork.main", "..."]
}

Each resource must include:
- `type`: exact GCP OpenTofu resource type (from the `hashicorp/google` provider)
- `name`: logical name
- `depends_on`: list of resources this depends on (as `type.name`)
- `config`: key configuration values (machine_type, region, zone, service, etc.)
