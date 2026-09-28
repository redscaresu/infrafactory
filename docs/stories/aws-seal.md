---
kind: code
status: ready
epic: aws-layer3-seal-and-dispatch
depends_on: []
touches: ["go.mod", "go.sum", "internal/harness/sandbox_deploy.go", "internal/harness/aws_sealed_env.go (new)", "internal/harness/aws_sealed_env_test.go (new)", "internal/harness/aws_client.go (new)", "internal/harness/aws_client_test.go (new)", "internal/harness/aws_sdk_audit_test.go (new)", "internal/cli/aws_sealed_env_exec_test.go (new)", "docs/stories/aws-seal.md (delete)"]
risk: high
---

# The AWS seal: Layer 3 strips AWS_*, one builder seals the env from a mode-0600 file, and the in-process STS client is built from that map alone

Strip: append "AWS_*" to harness.SandboxStripEnv (internal/harness/sandbox_deploy.go:32-48). TF_VAR_* and TF_CLI_ARGS* stay (:39-46). exec_runner.go is not edited: envKeyMatches already takes a trailing * (:86-97), and the strip remains per-command (:143).

Builder (new internal/harness/aws_sealed_env.go): AWSSealedEnv(credFile, region). The credential file is $HOME/.config/infrafactory/layer3-aws.env, beside layer3.env (docs/operations.md:17). It is KEY=VALUE and holds only AWS_ACCESS_KEY_ID and AWS_SECRET_ACCESS_KEY. The builder refuses when the file is missing, is not a regular file, has any group or other permission bit, has an empty key, has an unknown key, or when the region fails ValidAWSRegion (^[a-z]{2}(-[a-z]+)+-[0-9]+$). It returns exactly AWS_ACCESS_KEY_ID, AWS_SECRET_ACCESS_KEY, AWS_REGION, AWS_EC2_METADATA_DISABLED=true, AWS_SHARED_CREDENTIALS_FILE and AWS_CONFIG_FILE. The last two are constant absolute paths that no user, root included, can create (e.g. children of /dev/null). It never reads the process env.

Client (new internal/harness/aws_client.go): add github.com/aws/aws-sdk-go-v2 (aws, credentials, service/sts). The config module is imported only by _test.go files. The client builds aws.Config by hand from the map, with static credentials. Its STS endpoint is https://sts.<validated region>.amazonaws.com unless a test endpoint argument is given, and the HTTP doer is injected through sts.Options.HTTPClient. The constructor's godoc explains why not LoadDefaultConfig (it reads AWS_ENDPOINT_URL_*, ~/.aws/config services and IMDS), and aws-layer-neutral-hcl's ADR quotes it.

Tests use an injected doer whose DialContext refuses non-loopback addresses, never a proxy env var: ProxyFromEnvironment caches once per process and skips loopback. Licence: Apache-2.0, like LICENSE:1-3.

**Done when:**
- Subprocess test through execCommandRunner: inherited AWS_ENDPOINT_URL_EC2, AWS_PROFILE, AWS_ACCESS_KEY_ID=PLANTED and TF_VAR_volume_size are unset in the child, which sees the builder's key; HTTP_PROXY, HTTPS_PROXY and SSL_CERT_FILE pass through. It fails if AWS_* leaves SandboxStripEnv
- A subprocess run with no StripEnv (Layer 2's shape) still sees AWS_ENDPOINT_URL_EC2, so the test fails if the strip becomes global at exec_runner.go:143
- The builder's keys are exactly the six, and AWS_EC2_METADATA_DISABLED is "true". Each file path passes filepath.IsAbs, os.Stat on it fails, and os.MkdirAll on its parent fails
- Refusal table: a missing file, a directory, mode 0644, mode 0640, an empty key, AWS_SESSION_TOKEN, and the regions "", "x@evil.example/", "us-east-1.evil.com" and "US-EAST-1" each refuse, naming the reason. Mode 0600 with us-east-1, eu-west-2 or us-gov-west-1 succeeds
- Default-endpoint test: a client built from the builder's map, with no endpoint argument and a capturing doer that never dials, sends GetCallerIdentity to https://sts.us-east-1.amazonaws.com/ with the file's key in the SigV4 Credential. The planted AWS_ENDPOINT_URL_STS, AWS_ENDPOINT_URL, AWS_PROFILE and a HOME .aws/config services entry pointing at a trap loopback server are all ignored: the trap counts zero
- With the loopback-only doer, a loopback STS returns the identity, and a request to 203.0.113.1 fails in the dialer
- A malformed region is refused by the client constructor before any request (the capturing doer counts zero)
- Chain test (_test.go only): with the process env reduced to the sealed map and the planted HOME files present, the SDK default chain (config.LoadDefaultConfig, which terraform-provider-aws builds on) loads without error and resolves the map's key and region, not the planted profile. This proves the sentinel paths read as absent
- AST audit: go/parser over non-test .go files under cmd/ and internal/ fails on an import of github.com/aws/aws-sdk-go-v2/config or on any selector named LoadDefaultConfig, naming the file. Fixture cases show both are caught and that a godoc comment is not
- govulncheck (.github/workflows/ci.yml:158-188) passes. TestSandboxEnvNeverInheritsMockwayURL, TestSandboxHarnessesDeclareStripEnv, cloud_parity_test.go and sandbox_destroy_test.go pass unmodified
- The doc-hygiene CI check passes, with the tip commit carrying 'ADR: none — ADR-0023 rules 1-2 for AWS; aws-layer3-claim-sweep-reap records them in ADR-0023 and aws-layer-neutral-hcl records the SDK'
