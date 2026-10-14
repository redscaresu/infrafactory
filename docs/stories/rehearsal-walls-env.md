---
kind: code
status: blocked
blocked_by: [rehearsal-walls-adr]
epic: rehearsal-walls
depends_on: [rehearsal-walls-adr]
touches: [internal/cli/rehearse_walls_env.go, internal/cli/rehearse_walls_env_test.go]
risk: high
---

# rehearsalWallsEnv: the replace-mode AWS environment every tofu step after prepare runs under

New file `internal/cli/rehearse_walls_env.go` with `rehearsalWallsEnv(rehearsal, fakeawsURL, region string) map[string]string`. It also defines the two fake-key constants every wall shares, `rehearsalFakeAccessKeyID` and `rehearsalFakeSecretAccessKey`, with fixed values distinct from the AKIAIOSFODNN7EXAMPLE pair the fail-closed tests plant in the shell. rehearsal-walls-provider-override uses these same constants and defines none of its own.

The keys are exactly: HOME=<id>/home and TF_DATA_DIR=<id>/data; AWS_ACCESS_KEY_ID=rehearsalFakeAccessKeyID and AWS_SECRET_ACCESS_KEY=rehearsalFakeSecretAccessKey; AWS_REGION=region; AWS_EC2_METADATA_DISABLED=true; AWS_CONFIG_FILE and AWS_SHARED_CREDENTIALS_FILE, the harness sealed paths; AWS_ENDPOINT_URL and AWS_ENDPOINT_URL_S3 at rehearsalDeadProxy; AWS_ENDPOINT_URL_<SVC> for every awsFakeawsPaths entry except S3, at fakeawsURL plus its path with {region} filled; HTTPS_PROXY and HTTP_PROXY at rehearsalDeadProxy. There is no NO_PROXY, and no other region variable.

It reads awsFakeawsPaths (internal/cli/test_command.go:203-216) and edits nothing in test_command.go: the split ADR-0039 asks for is already done (awsLayer2Env, test_command.go:234-257). There is no production caller yet; rehearsal-passthrough and the fail-closed tests use it.

**Done when:**
- TestRehearsalWallsEnvExactKeys pins the exact map for a fixed input. Adding AWS_DEFAULT_REGION, NO_PROXY or any SCW_/GOOGLE_/CLOUDSDK_/GENESYS key, or pointing S3 at fakeaws, turns it red (cp-backup).
- A test asserts AWS_REGION equals the region passed and that no other key contains REGION, except the AWS_ENDPOINT_URL_* keys; no value contains `{region}`.
- A test asserts AWS_ACCESS_KEY_ID equals rehearsalFakeAccessKeyID and AWS_SECRET_ACCESS_KEY equals rehearsalFakeSecretAccessKey, and that neither equals the AKIAIOSFODNN7EXAMPLE pair.
- `git diff origin/main -- internal/cli/test_command.go internal/cli/generate_command.go internal/cli/layer3_aws_provider.go` is empty, and TestCloudEnvPinsTheLayer2AWSEnv, TestLayer2AndLayer3AWSEnvsAreInverses and the aws_provider_wiring tests pass unchanged.
- Checks before every stage in docs/epics/rehearsal-walls.md hold.

**From review (binding; overrides the scope above where they differ):**
- This story owns rehearsalFakeAccessKeyID and rehearsalFakeSecretAccessKey, named once. Two stories in one package each defining their own would compile and could drift apart (skeptic of provider-override; the lead chose env as owner, and provider-override depends on this story).
