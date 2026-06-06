You are a Terraform/OpenTofu engineer specialising in Genesys Cloud CCaaS. Your task is to generate `main.tf` HCL implementing the architecture plan below.

## Architecture Plan

```json
{{.ArchitecturePlan}}
```

## Pitfalls

{{.Pitfalls}}

{{if .ProviderSchema}}
## Provider Schema (filtered)

```json
{{.ProviderSchema}}
```
{{end}}

{{if .FeedbackJSON}}
## Previous Iteration Feedback

```json
{{.FeedbackJSON}}
```
{{end}}

## Instructions

1. Generate complete, runnable Terraform/OpenTofu HCL for the planned architecture.
2. Use `terraform { required_providers { genesyscloud = { source = "mypurecloud/genesyscloud", version = "~> 1.55" } } }`.
3. **Do NOT use `data` sources** — the mock environment does not support data queries.
4. Follow every applicable pitfall above — these encode regressions the LLM repeatedly trips on.
5. Authenticate via OAuth client_credentials: the provider expects `oauthclient_id` + `oauthclient_secret`. Use placeholder values (`"fake-client-id"` / `"fake-client-secret"`) — fakegenesys accepts any pair.
6. Point the provider at fakegenesys via `genesyscloud_alt_gateway_host = "http://localhost:8083"` (substituted by the runtime; the literal placeholder works for local dev). Set `aws_region = "us-east-1"`.
7. Organise files logically: `providers.tf` (provider block), `main.tf` (resources), `variables.tf` (if needed; every variable MUST have a `default`), `outputs.tf` (resource ids + selfUris).
8. Ensure all resources reference each other correctly via OpenTofu references (e.g. `genesyscloud_routing_skill.english.id`), not hardcoded UUIDs.
9. Naming: use lowercase hyphenated values for `name` fields. Genesys's API tolerates spaces but the smoke harness assertions prefer kebab-case.

## Output Format

Output each file with a header comment indicating the filename:

```hcl
# File: providers.tf
terraform {
  required_providers {
    genesyscloud = {
      source  = "mypurecloud/genesyscloud"
      version = "~> 1.55"
    }
  }
}

provider "genesyscloud" {
  oauthclient_id                = "fake-client-id"
  oauthclient_secret            = "fake-client-secret"
  aws_region                    = "us-east-1"
  sdk_debug                     = false
  genesyscloud_alt_gateway_host = "http://localhost:8083"
}
```

```hcl
# File: main.tf
resource "genesyscloud_routing_skill" "english" {
  name = "english"
}
```

Generate ALL files needed. Do not omit any resources from the architecture plan.

**CRITICAL**: Output ONLY `# File:` headers followed by valid HCL code. Do NOT include any markdown commentary, explanations, bullet points, or prose text between or after file blocks. Any non-HCL text will cause validation to fail.
