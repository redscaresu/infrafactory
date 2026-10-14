---
kind: code
status: blocked
blocked_by: [rehearsal-walls-adr, rehearsal-walls-env]
epic: rehearsal-walls
depends_on: [rehearsal-walls-adr, rehearsal-walls-env]
touches: [internal/cli/rehearse_walls_override.go, internal/cli/rehearse_walls_override_test.go, internal/cli/testdata/golden/walls/, internal/cli/rehearsal_assumptions_test.go, .github/fakeaws-e2e-tests, docs/decisions/0028-factual-claims-are-tests-not-comments.md]
risk: high
---

# The provider override's exact bytes, with endpoints keys parsed from `tofu providers schema -json` output

New file `internal/cli/rehearse_walls_override.go`.

`rehearsalEndpointKeys(schemaJSON []byte) ([]string, error)` returns the sorted attribute names of the aws provider's `endpoints` block_type. The provider is the single `provider_schemas` entry whose address is `<host>/hashicorp/aws` for a host in rehearsalLockHosts (the lock admits registry.terraform.io/hashicorp/aws with a warning, rehearse_lock.go:26, :65-66). Zero such entries or two is an error, as is a missing endpoints block, an empty key set, or one without `sts`. The existing test reader that hardcodes registry.opentofu.org (rehearse_gate_aws_schema_test.go:42) is not the model.

`rehearsalProviderOverride(fakeawsURL, region string, keys, aliases []string) []byte` emits one `provider "aws"` block for the default provider and one per alias, in sorted order. Each block holds `alias` (aliased blocks only), `access_key = rehearsalFakeAccessKeyID` and `secret_key = rehearsalFakeSecretAccessKey` (the constants rehearsal-walls-env defines), `skip_metadata_api_check = true`, and `endpoints {}` naming every key. A key in the modelled map (ec2, iam, eks, rds, sqs, dynamodb, secretsmanager, kms, route53, sts, ssm → the awsFakeawsPaths path, test_command.go:203-216) goes to fakeawsURL plus its path with `{region}` filled; every other key, s3 included, goes to rehearsalDeadProxy. The output is deterministic. A golden file pins the bytes for a fixed input (UPDATE_GOLDEN=1 to rewrite). No prepare flow change.

Two ADR-0028 assumption tests pin the tofu claims this rests on, added to rehearsal_assumptions_test.go, listed in .github/fakeaws-e2e-tests, and given rows in ADR-0028's 2026-10-11 amendment table:
- TestAssumption_ProvidersSchemaOffline: after a first init and a second init with -plugin-dir (the rehearsal's two inits), `tofu providers schema -json` under the second init's env (ReplaceEnv, HTTPS_PROXY and HTTP_PROXY at a recorder) succeeds, prints the aws endpoints block, and the recorder counts zero.
- TestAssumption_AliasedOverrideMerges: a root with `provider "aws" { alias = "west" }` and an override file holding an aliased block with endpoints at a closed port: an apply of a resource with `provider = aws.west` dies at the closed address and the recorder counts zero. A control without the aliased override block does not die there.

**Done when:**
- TestRehearsalProviderOverrideGolden matches testdata/golden/walls/provider_override.tf for a fixed input of 2 aliases and a key list holding ec2, s3, sts and elbv2. Mapping s3 to fakeaws, or dropping an alias block, turns it red.
- Outside the golden, for fakeawsURL=http://127.0.0.1:4566 and region=eu-west-1, a test asserts these literal lines: `ec2 = "http://127.0.0.1:4566/ec2/region/eu-west-1"`, `sts = "http://127.0.0.1:4566/sts"`, s3 and elbv2 equal to rehearsalDeadProxy, and no `{region}` anywhere in the output. Leaving `{region}` unfilled turns it red (cp-backup).
- A test asserts each block's access_key equals rehearsalWallsEnv(...)["AWS_ACCESS_KEY_ID"] and its secret_key equals AWS_SECRET_ACCESS_KEY there.
- The override parses with hclsyntax, and countEndpoints-style counting equals len(keys) for every block.
- rehearsalEndpointKeys refuses a schema JSON with no endpoints block, one whose endpoints lack sts, one with no hashicorp/aws entry, and one holding both registry.opentofu.org/hashicorp/aws and registry.terraform.io/hashicorp/aws. A trimmed fixture copied from the 5.100.0 schema returns its keys sorted, keyed under either host.
- A test asserts every modelled key's service id is in awsFakeawsPaths and S3 is not modelled. Adding "s3" to the modelled map turns it red.
- TestAssumption_ProvidersSchemaOffline and TestAssumption_AliasedOverrideMerges are in .github/fakeaws-e2e-tests (sorted) and pass; each is shown red by a cp-backup break (the schema call's proxy pointed at the recorder; the override's alias dropped).
- Checks before every stage in docs/epics/rehearsal-walls.md hold.

**From review (binding; overrides the scope above where they differ):**
- The schema entry is the single `<host>/hashicorp/aws` for a host in rehearsalLockHosts; zero or two refuse. Otherwise a registry.terraform.io lock passes here and fails in prepare (skeptic).
- Literal-line assertions sit outside the golden, because UPDATE_GOLDEN=1 certifies whatever the code makes (skeptic).
- The fake keys are rehearsalFakeAccessKeyID / rehearsalFakeSecretAccessKey from rehearsal-walls-env, which this story depends on; it defines no copy (skeptic, lead's choice).
- TestAssumption_ProvidersSchemaOffline and TestAssumption_AliasedOverrideMerges pin the two new tofu claims (critic, ADR-0028).
