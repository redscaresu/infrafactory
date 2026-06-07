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
10. **`genesyscloud_oauth_client.authorized_grant_type` uses HYPHEN, not underscore.** Valid values: `"CODE"`, `"TOKEN"`, `"SAML2BEARER"`, `"SAML2-BEARER"`, `"PASSWORD"`, `"CLIENT-CREDENTIALS"`. Do NOT write `"CLIENT_CREDENTIALS"` (the underscore form is rejected — the provider lists only the hyphenated values). The Go/Python convention of underscores doesn't apply here.

10c. **Auto-answer attributes use ONLY `auto_answer_only`.** Do NOT write `auto_answer_on`, `auto_answer_number_of_calls`, or any other `auto_answer_*` variant on `genesyscloud_routing_queue`, `genesyscloud_routing_utilization`, or any other Genesys resource. The single canonical attribute is `auto_answer_only` (boolean). The provider rejects every other `auto_answer_*` name with "argument not expected".

10b. **`genesyscloud_user` does NOT accept a `roles { }` block.** To assign roles to a user, declare a SEPARATE `genesyscloud_user_roles` resource:

    ```hcl
    resource "genesyscloud_user" "agent" {
      name  = "user-agent"
      email = "agent@example.com"
    }

    resource "genesyscloud_user_roles" "agent_roles" {
      user_id = genesyscloud_user.agent.id
      roles {
        role_id = genesyscloud_auth_role.agent_role.id
      }
    }
    ```

    Same pattern applies to `genesyscloud_oauth_client.roles`: that block IS supported on `genesyscloud_oauth_client` but must reference a `role_id`, not be a flat list.

11. **`genesyscloud_flow` workdir asset (auto-placed by the harness).** When any of your generated HCL declares a `genesyscloud_flow` resource, infrafactory drops a default `flow.yaml` into the workdir BEFORE `tofu plan` runs (because the provider reads the file at plan time via `CustomizeDiff`). Set ONLY `filepath = "${path.module}/flow.yaml"` — do NOT write a `local_file` resource (timing won't work), and do NOT set `file_content_hash` (it's computed by the provider). Example:

    ```hcl
    resource "genesyscloud_flow" "ivr" {
      filepath = "${path.module}/flow.yaml"
    }
    ```

    If your scenario plans MULTIPLE flow resources, use the same `flow.yaml` for all of them — the harness only places one stub file.

12. **`genesyscloud_flow` deprecated `local_file` pattern (kept for reference only — use item 11 instead).** If for some reason you can't use the harness-placed `flow.yaml`, the inline-generate pattern below is technically possible but rarely works because tofu plan evaluates the file before apply creates it. Pattern:

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
