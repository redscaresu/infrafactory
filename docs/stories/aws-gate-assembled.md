---
kind: code
status: blocked
blocked_by: [aws-gate-provider-boundary, aws-gate-attribute-allowlists, aws-gate-user-data-and-ami, aws-layer3-allowlist-entries]
epic: aws-layer3-gate
depends_on: [aws-gate-provider-boundary, aws-gate-attribute-allowlists, aws-gate-user-data-and-ami, aws-layer3-allowlist-entries]
touches: ["internal/cli/layer3_aws_hcl_shape.go (new)", "internal/cli/layer3_aws_hcl_shape_test.go (new)", "internal/cli/layer3_aws_parity_test.go (new)", "internal/cli/layer3_aws_policy_lockstep_test.go (new)", "docs/stories/aws-gate-assembled.md (delete)"]
risk: high
---

# validateAWSLayer3HCLShape assembles the AWS gate and admits web_step_one as infrafactory writes it. Every Scaleway gate rule, table entry and map entry has an AWS row: applied with a fixture through the driver, or declined with a reason

New file internal/cli/layer3_aws_hcl_shape.go adds validateAWSLayer3HCLShape(outputDir, awsGateInputs{AllowedResourceTypes, Region, AMI awsResolvedAMI, UserData}). The checks run in this order:
- layer3ParseDir;
- per block: layer3TopLevelBlockProblem, layer3NestedProblems, layer3FunctionCallProblems with awsUserDataExempt, and layer3UndestroyableProblems;
- per resource: layer3ResourceTypeProblem("aws_"), layer3MultiplicityProblems, awsResourceProblems, awsInstanceUserDataProblems and awsAMIProblems;
- for the stack: awsProviderProblems, awsMultiplicityProblems and awsAMIRootProblems;
- last, awsUserDataFileProblem.
The problems are sorted and wrapped in ErrLayer3RefusesConfiguration. Zero-valued inputs refuse. layer3PreflightHCLForCloud still refuses aws.

Parity test (S156d), new file layer3_aws_parity_test.go. Every source below needs a row. An applied row is an HCL mutation of the admitted stack that the driver refuses. A declined row carries a non-empty reason.
- Every func in layer3_hcl_shape.go whose name ends in Problem or Problems, found with go/parser. After layer3-gate-shared-rules this includes the former inline rules layer3TopLevelBlockProblem, layer3ResourceTypeProblem, layer3ProjectCountProblem, layer3MissingProviderProblem, layer3ProviderBlockProblems, layer3TerraformBlockProblems and layer3ProviderVersionProblem, and that story's AST guard keeps new inline rules out of the drivers.
- layer3UnreadableConfigExt and layer3ParseDir.
- Every key of layer3DeniedNestedBlocks, and every non-admitted top-level block tofu 1.12 accepts: data, module, import, moved, removed, check, ephemeral.
- Every entry of layer3NumericBounds, layer3EnumBounds and layer3NestedCostBounds, ranged in the test.

Named rows:
- Applied: the missing-required_providers rule → AWS without required_providers. layer3CostProblems and layer3EnumBounds' instance type → instance_type t3.medium. layer3NestedCostProblems and layer3NestedCostBounds → root volume_size 21 and root iops. layer3ParentBindingProblems → a literal vpc_id. layer3ProviderVersionProblem → 5.99.0.
- Declined: layer3ProjectCountProblem and layer3ContainmentProblems' project_id rule (the AWS scope is the account; the credential bounds it, and UnplacedAWSResources checks it after apply). layer3EnumBounds' scaleway_lb (aws_lb is not allowlisted at step one). layer3UndestroyableResourceProblems (a Scaleway standardisation; aws_network_interface is not allowlisted). layer3InlinePrivateNetworkProblems (AWS attaches through subnet_id and vpc_security_group_ids, which are typed references).

Also:
- The attribute table (layer3_aws_attrs.go) and the aws_ entries of config.Default() are the same set.
- New layer3_aws_policy_lockstep_test.go evaluates policies/aws/vpc_required.rego:13-19, the way vpc_required_lockstep_test.go evaluates the Scaleway policy. The admitted aws_instance passes, and the attribute the policy demands, subnet_id, is on aws_instance's table entry.

**Done when:**
- Admit: validateAWSLayer3HCLShape passes a directory built by production functions, with Region us-east-1, AMI {harness.AWSLayer2AMI, {8, gp3, true}} and config.Default()'s allowlist. The directory is the e2e web-step-one.tf, run through ensureAwsProviderWiring with a run id and through placeAWSUserData with renderAWSUserData's script for web-step-one.yaml
- validateAWSLayer3HCLShape refuses each of these, naming the offending item, through the rule func the row names: an allowlisted scaleway_instance_server (layer3ResourceTypeProblem); aws_launch_template (layer3ResourceTypeProblem); data "aws_ami", module and import (layer3TopLevelBlockProblem); count and for_each (layer3MultiplicityProblems); provisioner, connection, lifecycle and dynamic "ingress" (layer3NestedProblems); terraform { encryption { key_provider "external" ... } } (layer3TerraformBlockProblems via awsProviderProblems); file("/proc/self/environ") in tags (layer3FunctionCallProblems); an unguarded aws_instance.web.ipv6_addresses[0] in an output (layer3UndestroyableProblems); terraform.tfvars, x.tf.json, x.tofu and an unparseable .tf (layer3ParseDir); `ingress = [{...}]` (awsResourceProblems); a changed ami (awsAMIProblems); AMI root {30, gp3, true} (awsAMIRootProblems); a tampered infrafactory-user-data.sh (awsUserDataFileProblem)
- validateAWSLayer3HCLShape refuses the admitted stack given an empty Region, an empty AMI ID, a zero-value AMI Root, nil UserData, or an empty allowlist
- The parity checker fails when a fixture source adds a func without a row, when a layer3DeniedNestedBlocks key or bounds entry has no row, when an applied row's mutation is admitted, and when a declined row's reason is empty
- Dropping subnet_id from aws_instance's table entry fails the AWS lockstep test. The table-versus-allowlist test fails when either side gains a type
- doc-hygiene passes, and the tip commit carries 'ADR: none — assembles the HLD 2026-09-27 AWS gate under ADR-0023, with parity per S156d'
