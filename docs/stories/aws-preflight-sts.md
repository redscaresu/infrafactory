---
kind: code
status: ready
epic: aws-layer3-seal-and-dispatch
depends_on: [aws-seal, layer3-cloud-teardown-seams]
touches: ["internal/config/config.go", "internal/cli/runtime.go", "internal/cli/test_command.go", "internal/cli/aws_preflight.go (new)", "internal/cli/aws_preflight_test.go (new)", "internal/harness/aws_identity.go (new)", "internal/harness/aws_identity_test.go (new)", "docs/stories/aws-preflight-sts.md (delete)"]
risk: high
---

# AWS credential preflight: the sealed builder plus sts:GetCallerIdentity, refusing a wrong account or principal

internal/config/config.go gains an AWS Layer 3 block with region, account_id and principal_arn. Any empty value, or a region failing ValidAWSRegion, is refused. infrafactory.yaml is not edited: the values are the operator's, from aws-layer3-claim-sweep-reap's hand setup. RuntimeDependencies (runtime.go:108-125) gains the STS HTTP doer, whose default is an http.Client with a timeout; the endpoint is never read from config or env. New harness VerifyAWSIdentity(ctx, env, doer, endpoint, wantAccount, wantPrincipal) wraps the aws-seal client.

The aws arms from layer3-cloud-teardown-seams:
- assertSandboxCredentials(aws) runs AWSSealedEnv and then VerifyAWSIdentity with that same map, refusing unless Account == account_id and Arn == principal_arn.
- sandboxCommandEnvForProject(aws, scope) returns the map only when scope == account_id, and makes no call.

All local checks come before the STS call (ADR-0025 ordering). The pass detail (test_command.go:792) names the verified account. Tests inject aws-seal's loopback-only doer and an httptest STS.

**Done when:**
- Right account and principal: the preflight passes, the STS request carries the credential file's key, and sandboxCommandEnvForProject(aws, account_id) returns the six keys with the same key and zero requests
- A wrong account, a wrong principal, an STS 403 and malformed XML each refuse, naming what differed or failed
- A missing credential file, mode 0644, an empty account_id, an empty principal_arn, an empty region and the region "x@evil.example/" each refuse with zero STS requests
- sandboxCommandEnvForProject(aws) refuses an empty scope or a scope other than account_id
- sandbox_env_test.go, run_project_lifecycle_test.go (including TestNoProductionPathBuildsSandboxEnvWithoutARunProject), deploy_command_test.go and internal/config's tests pass unmodified
- The doc-hygiene CI check passes, with the tip commit carrying 'ADR: none — ADR-0023 credential-blast-radius preflight applied to AWS; aws-layer3-claim-sweep-reap records it'
