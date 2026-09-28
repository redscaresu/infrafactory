---
kind: code
status: ready
epic: fakeaws-step-one-surfaces
repo: fakeaws
depends_on: [fakeaws-subnet-and-instance-attributes]
touches: ["handlers/ec2.go", "handlers/ec2_test.go", "handlers/regression_test.go", "examples/misconfigured/instance_unknown_ami/ (new)", "README.md", "CHANGELOG.md"]
---

# Item 10 (+ item 9's fixture): RunInstances refuses unknown AMIs; an AL2023 fixture exists

Delete ensureAMIExists (ec2.go:1774-1794) and its call and comment (:1427-1433). An unknown ImageId returns 400 InvalidAMIID.NotFound through awsproto.WriteServiceError. Add an AL2023 fixture to ec2AMIFixtures (:1768-1772): name al2023-ami-2023.*-kernel-6.1-x86_64, an id distinct from ami-0abcd1234 (amzn2), exported as a constant for SSM. Invert TestRegressionRunInstancesAutoSeedsUnknownAMI (regression_test.go:679-727), citing HLD:285-288. Add examples/misconfigured/instance_unknown_ami (ami-0c55b159cbfafe1f0). README.md:39 drops 'AMI auto-seed'. The CHANGELOG entry states the reversal and that LLM sweeps writing a documentation AMI now fail at Layer 2 until aws-layer-neutral-hcl's resolver lands.

**Done when:**
- Regression test: ami-0c55b159cbfafe1f0 returns InvalidAMIID.NotFound and DescribeImages still does not know it; ami-0abcd1234 and the AL2023 fixture return 200. Fails if the auto-seed returns
- DescribeImages in two regions lists the AL2023 fixture, with a name starting al2023-ami- and an id different from ami-0abcd1234
- In the provider-smoke job, instance_unknown_ami passes (expected.txt InvalidAMIID.NotFound) with knownRed unchanged
- grep -n 'auto-seed' README.md finds nothing
