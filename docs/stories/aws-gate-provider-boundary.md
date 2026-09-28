---
kind: code
status: ready
epic: aws-layer3-gate
depends_on: [layer3-gate-shared-rules]
touches: ["internal/cli/layer3_aws_provider.go (new)", "internal/cli/layer3_aws_provider_test.go (new)", "docs/stories/aws-gate-provider-boundary.md (delete)"]
risk: high
---

# AWS gate, provider leg (the region boundary's gate leg): terraform {} is allowlisted, and there is exactly one provider "aws" in the configured region, with only region, s3_use_path_style and infrafactory's default_tags, the exact hashicorp/aws pin, no alias and no provider =

New file internal/cli/layer3_aws_provider.go. awsProviderProblems(parsed, varDefaults, region) runs over every file; aws-gate-assembled wires it into the gate. It checks:
- Every terraform block, through layer3TerraformBlockProblems (layer3-gate-shared-rules). Every required_providers, through the pinned layer3ProviderSourceProblems. The only admitted entry is aws = {source hashicorp/aws, version harness.AWSProviderVersion}. Any other key in that object (such as configuration_aliases), any other provider, and a missing entry (layer3MissingProviderProblem) are refused. There is no second pin constant: generation writes harness.AWSProviderVersion (generate_command.go:361,417) and the e2e mirror installs it (internal/e2e/helpers.go:508).
- Exactly one provider block across all files, labelled exactly ["aws"], checked by layer3ProviderBlockProblems with safe attrs {region, s3_use_path_style} (ADR-0039 decision 3). region must be present and resolve to exactly the configured aws.region. An empty configured region refuses; it never defaults the way awsRegion does (test_command.go:222-227). s3_use_path_style must be literal true.
- The provider's nested blocks. layer3ProviderBlockProblems checks attributes only, so this function checks them. At most one default_tags is allowed, whose only attribute is tags: a literal object with exactly the key awsRunIDTagKey (generate_command.go:583) and a literal string value. endpoints, assume_role, ignore_tags and any other block are refused. The run-id key anywhere else in the stack is refused, as refuseAwsRunIDTag refuses it (generate_command.go:376-386).
- A provider meta-argument on any resource is refused.

**Done when:**
- Admit: awsProviderProblems returns nothing for the providers.tf that ensureAwsProviderWiring (generate_command.go:347) writes for us-east-1, with runID "20260928T000000Z" and with "". The test builds it on the e2e web-step-one fixture, and the same holds with terraform { required_version = ">= 1.6" } added
- awsProviderProblems refuses these through layer3TerraformBlockProblems, naming the terraform block: encryption with key_provider "external" (with a command), with method "external", and with key_provider "aws_kms"; provider_meta "aws"; backend "s3"; cloud; experiments; an unknown block
- awsProviderProblems refuses these, naming the offending item: zero provider "aws" blocks; two, in one file or in two; provider "scaleway"; a provider "aws" with no region; region eu-west-1 against configured us-east-1; region = var.r with no default; an empty configured region; s3_use_path_style = false; alias, access_key, secret_key, token, profile, shared_config_files, shared_credentials_files, skip_credentials_validation and assume_role (the last as an attribute and as a block); endpoints {}; ignore_tags {}; default_tags with a second key, with a non-literal value, or given twice; the run-id key in a resource's tags; provider = aws.x on a resource
- awsProviderProblems refuses these through the pinned layer3ProviderSourceProblems: source attacker/aws; version ~> 5.100, 5.99.0, or none; configuration_aliases; a random provider. It refuses a stack with no required_providers through layer3MissingProviderProblem
- doc-hygiene passes, and the tip commit carries 'ADR: none — AWS provider leg of the HLD 2026-09-27 gate under ADR-0023 and ADR-0039 decisions 3-4'
