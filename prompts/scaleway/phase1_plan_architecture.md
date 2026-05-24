You are an infrastructure architect specialising in Scaleway cloud. Your task is to produce a JSON architecture plan for the following scenario.

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

1. Analyse the scenario and identify all Scaleway resources needed.
2. Map intent-driven sizes to concrete Scaleway offerings using ONLY the exact values in the Size Mappings table above. Do NOT invent types — use the mappings verbatim (e.g., compute large → `GP1-S`, NOT `GP1-L`).
3. Apply any prescriptive overrides — these take priority over size mappings.
4. Identify dependencies between resources (e.g., private network before instances).
5. Determine the correct Scaleway zones/regions based on constraints.

## Output Format

Respond with ONLY a JSON object (no markdown fences, no explanation):

{
  "scenario": "scenario-name",
  "region": "fr-par",
  "zone": "fr-par-1",
  "resources": [
    {
      "type": "scaleway_vpc",
      "name": "main",
      "depends_on": [],
      "config": {}
    }
  ],
  "dependency_order": ["scaleway_vpc.main", "scaleway_vpc_private_network.main", "..."]
}

Each resource must include:
- `type`: exact Scaleway OpenTofu resource type
- `name`: logical name
- `depends_on`: list of resources this depends on (as `type.name`)
- `config`: key configuration values (offering, region, zone, etc.)
