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
6. **Do NOT add a custom-endpoint attribute to the provider block.** The `mypurecloud/genesyscloud` provider does NOT accept HCL endpoint overrides. infrafactory sets `GENESYSCLOUD_GATEWAY_{PROTOCOL,HOST,PORT}` env vars at tofu invocation time to redirect every API call at fakegenesys. The provider block only needs `oauthclient_id`, `oauthclient_secret`, and `aws_region`.
7. Organise files logically: `providers.tf` (provider block), `main.tf` (resources), `variables.tf` (if needed; every variable MUST have a `default`), `outputs.tf` (resource ids + selfUris).
8. Ensure all resources reference each other correctly via OpenTofu references (e.g. `genesyscloud_routing_skill.english.id`), not hardcoded UUIDs.
9. Naming: use lowercase hyphenated values for `name` fields. Genesys's API tolerates spaces but the smoke harness assertions prefer kebab-case.
10. **`genesyscloud_flow` requires a YAML file ON DISK** that you generate inline via a `local_file` resource. The provider rejects `file_content_hash` as unconfigurable, and the smoke harness does not pre-place any files. Pattern:

    ```hcl
    terraform {
      required_providers {
        genesyscloud = { source = "mypurecloud/genesyscloud", version = "~> 1.55" }
        local        = { source = "hashicorp/local",        version = "~> 2.5"  }
      }
    }

    resource "local_file" "ivr_flow_yaml" {
      filename = "${path.module}/ivr_flow.yaml"
      content  = <<-EOT
        inboundCall:
          name: ivr-lookup
          startUpRef: "/inboundCall/menus/menu[main]"
          defaultLanguage: en-us
          supportedLanguages:
            en-us: { defaultLanguageSkill: { noValue: true } }
          menus:
            - menu:
                name: main
                refId: main
                audio:
                  defaultAudio: { tts: "Welcome." }
                choices: []
      EOT
    }

    resource "genesyscloud_flow" "ivr" {
      filepath          = local_file.ivr_flow_yaml.filename
      file_content_hash = filesha256(local_file.ivr_flow_yaml.filename)
      depends_on        = [local_file.ivr_flow_yaml]
    }
    ```

    Do NOT reference a bare filename (`filepath = "flow.yaml"`) — the file will not exist. Always generate the YAML inline via `local_file`. The flow type must match the scenario (`inboundCall`, `inboundChat`, `outbound`, etc.) — read the architecture plan carefully. `file_content_hash` IS configurable when supplied as `filesha256(...)`; the earlier-version error came from passing the wrong literal value.

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
  oauthclient_id     = "fake-client-id"
  oauthclient_secret = "fake-client-secret"
  aws_region         = "us-east-1"
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
