---
kind: code
repo: fakeaws
status: ready
epic: fakeaws-loaded-identity
depends_on: [fakeaws-identity-store-and-flag, fakeaws-identity-iam-sts]
touches: [examples/identity/allowed_account_ids/main.tf, examples/identity_test.go]
---

# allowed_account_ids configures against a fakeaws started with -account, on its own process

The example goes outside the auto-discovered trees, at `examples/identity/allowed_account_ids/main.tf`. Every `examples/{working,misconfigured,updates}` directory runs in parallel against the one :8082 instance at the default account (examples/provider_smoke_test.go:61-85, AGENTS.md:91). The example has:
- provider 5.100.0 with `allowed_account_ids = ["123456789012"]`;
- endpoints from the environment only, as examples/working/env_endpoints does;
- `data.aws_caller_identity` with a precondition on account_id;
- one resource.

New `examples/identity_test.go` sits behind requireE2EGate and requireTofu and runs in exactly one SMOKE_SHARD (inShard). It builds cmd/fakeaws and starts it on a free loopback port with `-account 123456789012` and a temp `-db`. It waits for /healthz, then runs init, apply, `plan -detailed-exitcode` and destroy. It then starts a second process without `-account` and asserts that apply fails with the provider's allowed_account_ids error. t.Cleanup kills both processes. The shared smoke instance and the existing examples do not change.

**Done when:**
- With INFRAFACTORY_ENABLE_E2E=1 and tofu present, every step of the positive run exits 0 and plan reports no diff. The negative run's apply fails, and its output contains the provider's allowed account error text.
- Under SMOKE_SHARD=i/5 the test runs for exactly one i, and TestShardsPartitionTheSmokeRun still passes.
- The new test is shown failing by a cp-backup break that removes `-account` from the positive start, recorded in the PR.
