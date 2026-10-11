---
kind: code
status: ready
epic: rehearsal-assumption-tests
depends_on: []
touches: [internal/cli/rehearsal_assumptions_test.go, internal/e2e/helpers.go, .github/fakeaws-e2e-tests]
risk: high
---

# Claims (a) and (g): the endpoints override keeps apply off prod; a later user override wins; a root override misses a child module's provider

Create `internal/cli/rehearsal_assumptions_test.go`, `package cli_test` (it imports internal/e2e; package cli's TestMain lives at aws_ami_resolve_test.go:27, so no TestMain here). The file also holds a recording listener (counts accepted connections and logs CONNECT hosts) that later stories reuse.

**Shared rules for every test in the file.**
- Every child env is an explicit list (PATH, HOME=t.TempDir(), SealNetwork's TF_CLI_CONFIG_FILE, the proxy at the recorder, AWS_REGION, an AKIA…EXAMPLE fake key pair), never os.Environ().
- No AWS_ENDPOINT_URL* is set, except in g2, which sets AWS_ENDPOINT_URL_STS on purpose to test precedence.
- Every fixture's provider block sets `max_retries = 1`, and every tofu call runs under exec.CommandContext with a stated deadline (2m). Measured: without this the SDK retries for more than 10 minutes.
- The provider is `harness.AWSProviderVersion`; failure messages carry `tofu version`.
- Doc comments state the claim, cite ADR-0028 and an HLD section heading (not line numbers), and name the relying epic.

**Fix the shared mirror first.** `e2e.CleanupAWSMirror` deletes the mirror the binary shares, and `awsMirrorOnce` never refills it (internal/e2e/helpers.go:532, :595-599). Make it reset `awsMirrorOnce` and `awsMirrorDir`, so the next SealNetwork mirrors again. Each new test calls `t.Cleanup(e2e.CleanupAWSMirror)`.

**(a) TestAssumption_EndpointsOverrideKeepsApplyOffProd**, relied on by rehearsal-walls. The override names every key of the `endpoints` block, from `tofu providers schema -json` at that version, all at a closed loopback port. The test asserts the generated key count equals the schema's (303 at 5.100.0). Two cases:
- **STS:** credential validation on. `tofu apply` on `aws_caller_identity` exits non-zero naming the closed port; the recorder counts 0.
- **EC2:** `skip_credentials_validation = true` and `skip_requesting_account_id = true`, plus an EC2 data source. DescribeVpcs is sent and dies at the closed port; the recorder counts 0.
- **Control:** an in-test run with `sts` omitted gets a recorder count of 1 or more.
- **Alias warnings:** naming every key gives about 39 "only one of … should be set" warnings. Log them with `tofu version`; fail if they become errors.

**(g1) TestAssumption_UserOverrideMergesAfterOurs**, relied on by rehearsal-prepare-gate. Our override (named so it sorts before `zz_`; the name is stated) routes to the closed port. A user `zz_override.tf` sets `endpoints { sts = <listener B> }`, and B receives the STS request.

**(g2) TestAssumption_RootOverrideMissesChildModuleProvider**, relied on by rehearsal-prepare-gate. The root has `provider "aws" {}` and the override goes to recorder A. A child module declares its own `provider "aws"` and a data source, routed by AWS_ENDPOINT_URL_STS to recorder B. B receives the child's request; A counts 0.

**If tofu contradicts a claim,** the test asserts what tofu does and the PR says the claim is false. Append the names to `.github/fakeaws-e2e-tests` in C-locale order. Tip commit: `ADR: none — assumption tests only; ADR-0028 lists them in rehearsal-assumptions-adr`.

**Done when:**
- With INFRAFACTORY_ENABLE_E2E=1, one `go test ./internal/cli -run '^(TestE2E_AWSLayer2SeedsTheResolvedAMI|TestAssumption_EndpointsOverrideKeepsApplyOffProd|TestAssumption_UserOverrideMergesAfterOurs|TestAssumption_RootOverrideMissesChildModuleProvider)$' -count=1 -v` prints four `--- PASS` lines and finishes in under 5 minutes. The PR's aws-e2e log has `--- PASS: <name> ` for each new test.
- The PR body records one cp-backup break per case, each with its FAIL output and the restore by cp:
  - (a) STS case: `sts` removed from the key list
  - (a) EC2 case: `ec2` removed (the recorder logs CONNECT ec2.<region>.amazonaws.com)
  - (g1): zz_override.tf renamed to sort before ours
  - (g2): the root override's endpoints copied into the child provider
- `LC_ALL=C sort -c <(grep -v '^#' .github/fakeaws-e2e-tests)` exits 0.
- `grep -n 'os.Environ' internal/cli/rehearsal_assumptions_test.go` prints nothing; gitleaks and the pre-commit secret scan are green on the fake key literal.
- `make doc-hygiene` passes.
