You are reviewing AWS Terraform/OpenTofu HCL for correctness against the scenario and known pitfalls.

## Scenario

```yaml
{{.ScenarioYAML}}
```

## Architecture Plan

```json
{{.ArchitecturePlan}}
```

## Generated HCL

```hcl
{{.GeneratedFiles}}
```

## Pitfalls

{{.Pitfalls}}

{{if .FeedbackJSON}}
## Previous Iteration Feedback

```json
{{.FeedbackJSON}}
```
{{end}}

## Instructions

Review the generated HCL against:

1. The scenario's required resources — every resource named in the plan must appear.
2. The pitfalls list — flag any rule the HCL violates.
3. AWS-specific correctness:
   - VPC + subnet exist before any compute, database, or kubernetes resource.
   - IAM role + instance profile chain is correct (profile bridges role to instance).
   - DB subnet group exists before any RDS instance in a custom VPC.
   - Security group rules use `aws_security_group_rule` resources (NOT inline `ingress` / `egress` blocks) when the same SG references itself, to avoid the cycle that inline blocks create.
   - S3 bucket names include an account-synthetic or run-scoped suffix (S3's name space is global).
4. Provider version pin matches `hashicorp/aws ~> 5.70`.

## Output Format

Respond with ONLY a JSON object:

```json
{
  "passes": true | false,
  "findings": [
    {"severity": "error" | "warning", "message": "..."}
  ],
  "summary": "..."
}
```

If `passes` is `false`, the runtime will trigger another generate-iteration with these findings as feedback.
