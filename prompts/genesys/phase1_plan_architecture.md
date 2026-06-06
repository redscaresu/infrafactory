You are an architect specialising in Genesys Cloud (CCaaS — Contact Center as a Service). Your task is to produce a JSON architecture plan for the following scenario.

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

The previous iteration's generated code failed validation. Analyse these failures and account for them in your architecture plan. Re-derive your solution from scratch — do not patch the previous attempt.

```json
{{.FeedbackJSON}}
```
{{end}}

## Instructions

**IMPORTANT**: Do NOT use Terraform/OpenTofu `data` sources. Use hardcoded values from the mappings and overrides above. The mock environment (fakegenesys) does not support data source queries.

1. Analyse the scenario and identify the Genesys Cloud resources needed. Choose from the implemented surface (see "Available resource types" below). If the scenario mentions concepts outside this surface, omit them — the mock won't validate them.
2. Apply any prescriptive overrides — these take priority over size mappings.
3. Identify dependencies between resources. Required ordering:
   - `genesyscloud_routing_skill` / `genesyscloud_routing_wrapupcode` / `genesyscloud_routing_language` BEFORE any `genesyscloud_routing_queue` that references them.
   - `genesyscloud_auth_role` BEFORE any `genesyscloud_user` that references it via `roles`.
   - `genesyscloud_user` BEFORE any `genesyscloud_routing_queue_member` (queue membership reference).
   - `genesyscloud_architect_user_prompt` BEFORE any flow that references the prompt id.
4. Use lowercase hyphenated names (`queue-support`, not `Queue Support`).

## Available resource types (fakegenesys surface)

Identity: `genesyscloud_user`, `genesyscloud_group`, `genesyscloud_location`, `genesyscloud_auth_role`, `genesyscloud_oauth_client`.

Routing: `genesyscloud_routing_queue` (with `members` block referencing user ids), `genesyscloud_routing_skill`, `genesyscloud_routing_wrapupcode`, `genesyscloud_routing_language`, `genesyscloud_routing_utilization` (singleton — one per org).

Architect: `genesyscloud_architect_datatable` (with `properties` schema + `row` sub-resources), `genesyscloud_architect_user_prompt`, `genesyscloud_flow` (multipart upload via `filepath`).

Other: `genesyscloud_responsemanagement_response`, `genesyscloud_idp_generic` (singleton).

## Output Format

Respond with ONLY a JSON object (no markdown fences, no explanation):

```json
{
  "resources": [
    {
      "type": "genesyscloud_routing_queue",
      "name": "support",
      "config": { "name": "queue-support" }
    },
    ...
  ],
  "rationale": "..."
}
```

Provider version pin: `mypurecloud/genesyscloud ~> 1.55`. Bumps require an explicit PR updating example required_providers + this prompt + the e2e harness together.
