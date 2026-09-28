# ADR-0013: Cross-Repo E2E Testing and GCP Multi-Cloud Support

## Status
Proposed

## Context
Two architectural decisions span multiple slices and affect long-term project structure:

1. **Cross-repo E2E testing (Slice 33)**: Infrafactory depends on mockway for mock deploy validation, but the two repos are tested independently. Bugs that span both (field naming mismatches like `ip_id` vs `ip_ids`, missing response fields, port conventions) are only caught by manual `infrafactory run` invocations. There is no automated regression net for cross-repo integration.

2. **GCP multi-cloud support (Slice 36)**: The codebase is Scaleway-first but the architecture was designed to be cloud-agnostic. Adding GCP support validates that the `cloud` field in scenario YAML, the per-provider pitfalls system (`pitfalls/{cloud}.yaml`), the prompt template selection, and the topology derivation layer actually work for a second cloud provider.

## Decision

### Cross-repo E2E strategy

- E2E tests live in infrafactory under `internal/e2e/`.
- Tests build and start mockway from source at `../mockway` (relative to infrafactory repo root). This assumes the developer has both repos checked out as siblings.
- A `TestMain` helper compiles mockway, starts the binary, waits for health check (`GET /mock/state`), and tears down after all tests complete.
- Tests invoke `infrafactory run` as a subprocess (not imported as a library) to test the full CLI path.
- Tests are gated by a build tag or env var (`INFRAFACTORY_E2E=1`) so they don't run in normal `go test ./...`.

### GCP multi-cloud approach

- Prompt templates are organized per cloud: `prompts/scaleway/` (existing, moved) and `prompts/gcp/` (new). The generator selects the template directory based on `scenario.cloud`.
- Topology derivation in `DeriveTopology` dispatches by cloud provider. GCP derivation uses the same pattern as Scaleway but maps GCP resource types (compute instances, forwarding rules, Cloud SQL, GKE).
- Pitfalls are already per-provider (`pitfalls/gcp.yaml`). No structural change needed.
- GCP mock server is fakegcp (separate repo, same pattern as mockway). Provider URL injection uses `GOOGLE_API_ENDPOINT` or equivalent provider override.
- Scenario schema adds GCP resource types as a new `cloud: gcp` branch. The JSON Schema uses conditional validation (`if cloud=gcp then resources must match GCP schema`).

## Consequences

**Benefits**:
- Cross-repo E2E catches integration bugs that unit tests in either repo miss.
- GCP support validates the cloud-agnostic architecture with a real second provider.
- Per-cloud prompt templates allow provider-specific guidance without conditional logic in templates.
- The pattern established by GCP makes adding AWS/Azure straightforward.

**Tradeoffs**:
- E2E tests require both repos checked out as siblings — CI needs a multi-repo checkout step.
- E2E tests are slower (compile + start mockway + full run loop) — gated behind opt-in flag.
- GCP prompt templates duplicate some structure from Scaleway templates — acceptable since provider-specific guidance dominates.
- fakegcp must exist and be sufficiently complete before GCP scenarios can pass.

### AWS note (aws-web-step-one-e2e)

`TestE2E_AWSWebStepOne` (`internal/e2e/aws_web_step_one_test.go`) is the AWS analogue of the
cross-repo pattern above, run against fakeaws instead of mockway: a fixed step-one fixture (VPC,
subnet, IGW, route table, route, association, one security group, one EC2 instance running the
scenario's `service:` block) is applied, converged and destroyed on fakeaws in CI, with
`http_probe compute:80` derived from fakeaws's live EC2 state
(`internal/harness/topology_derive_aws.go`). It additionally reads back fakeaws's
`DescribeInstanceAttribute(userData)` for the applied instance and compares it byte for byte
against the renderer's golden script — the bytes that reached the instance, not the file
generation wrote to disk. `TestE2E_AWSWebStepOneUnattachedGroup` is the fail-closed
counterpart: the same fixture with the ingress rule moved to a security group the instance never
attaches, asserting the run fails on `http_probe` with the unattached-group diagnostic rather
than reading "reachable" from a group with no effect. Both run in CI against fakeaws pinned to a
fixed commit (`.github/workflows/ci.yml`'s `FAKEAWS_SHA`), alongside the other AWS e2e tests.
