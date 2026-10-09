---
kind: code
status: blocked
blocked_by: [fakeaws-admin-seed-image]
epic: aws-web-live-on-real-aws
depends_on: [fakeaws-admin-seed-image]
touches: [internal/cli/test_command.go, internal/cli/aws_layer2_ami_seed.go, internal/cli/aws_layer2_ami_seed_test.go, internal/e2e/aws_layer2_ami_seed_fakeaws_test.go, .github/workflows/ci.yml]
risk: high
---

# An aws Layer 3 command seeds its resolved AL2023 id into fakeaws before every Layer 2 deploy

Today no aws Layer 3 `test` or `run` can reach the real apply. With sandbox_deploy on, generation
writes the id resolved from real SSM into the HCL (awsAMIForGeneration,
internal/cli/generate_command.go:716-728), and the gate requires exactly that literal
(awsAMIProblems, internal/cli/layer3_aws_user_data_gate.go:83-95, fed from
internal/cli/aws_post_apply.go:24). The Layer 2 mock deploy runs first, on fakeaws
(internal/cli/test_command.go:776), and fakeaws refuses any image it has not seeded with
`InvalidAMIID.NotFound`. Nothing in infrafactory seeds it. The unit tests miss this because they
use `harness.AWSLayer2AMI` as the "resolved" id (internal/cli/layer3_aws_hcl_shape_test.go:54-56).
Turning mock_deploy off is no workaround: `test` then returns before the sandbox.

Fix: when the scenario's cloud is aws and `runtime.AWSLayer3AMI` is set, POST that id with its
resolved root device name to fakeaws's `/mock/images` (fakeaws-admin-seed-image) immediately before
`runtime.Deps.MockDeploy.Run`, in the region and account the Layer 2 deploy uses. It goes there, not
once per command, because `run` resets every mock before each iteration
(internal/cli/run_command.go:128), and a reset drops the seed. A failed seed fails stage
`mock_deploy` with a detail naming the seed, and nothing is applied. Do NOT rewrite the ami to the
fixture for the mock: the mock then applies different HCL from the one the real apply gets, and
fidelity is lost exactly where it is being measured. Layer 3 off, or another cloud, seeds nothing.
Bump FAKEAWS_SHA in ci.yml to the fakeaws commit that adds the endpoint. Commit trailer:
`ADR: none — seeds the mock with the run's own resolved id; no contract change`.

**Done when:**
- An e2e test against a real fakeaws at the bumped FAKEAWS_SHA runs the AWS gate and then the
  Layer 2 mock deploy over the step-one HCL with a non-fixture AL2023-shaped id
  (`ami-0123456789abcdef0`, root `/dev/xvda`): the gate returns no problems and the mock apply
  succeeds. With the seed call removed (cp backup, restore by cp) the same test fails on
  `InvalidAMIID.NotFound`; the PR records it.
- A unit test drives `run` over two iterations with a reset between them and asserts the seed is
  posted before each mock deploy, after each reset.
- A failing seed endpoint fails stage `mock_deploy` and no `tofu apply` runs; Layer 3 off posts
  nothing.
- `go test -tags noui ./internal/cli/... ./internal/e2e/...` (with the fakeaws e2e at the pin) and
  `make doc-hygiene` pass.
