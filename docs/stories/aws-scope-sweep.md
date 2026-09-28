---
kind: code
status: ready
epic: aws-layer3-claim-sweep-reap
depends_on: [fakeaws-sweep-surfaces, aws-scope-claim]
touches: ["internal/harness/aws_sweep.go (new)", "internal/harness/aws_sweep_test.go (new)", "internal/e2e/aws_sweep_fakeaws_test.go (new)", ".github/workflows/ci.yml"]
risk: high
---

# The enumerated AWS scope sweep over every HLD collection. A bounded settle loop re-polls the whole scope, pagination fails on a token cycle, and a 403 fails closed

New file internal/harness/aws_sweep.go: SweepAWSScope(ctx, env, ec2 and ssm doers, endpoints, sleeper), over one exported collection table in aws.region (HLD:147-150). The rows: instances unless terminated; volumes; network interfaces; Elastic IPs; security groups unless GroupName=default; VPCs, all of them, the default included; subnets; internet gateways; route tables unless an association is Main; NAT gateways unless deleted; key pairs; images (Owners=self, IncludeDisabled=true); snapshots (OwnerIds=self); launch templates; and one SSM row, parameters other than the claim and the stamp. Settle: while any item in any collection is shutting-down, deleting or detaching, the whole sweep re-runs. It is bounded by a named constant, and at the bound it fails naming every remaining item and its state. Pagination: each collection keeps a set of the NextTokens it has seen, and any token seen before fails the sweep naming the collection. The sweep does not use the SDK paginator's StopOnDuplicateToken, which silently ends pagination. A stray's run-id tag (#340) is printed beside it and never gates. FAKEAWS_SHA is bumped to the fakeaws-sweep-surfaces merge.

**Done when:**
- Table test, fake doer, one row per collection. A planted item fails naming the collection and the id. A stray on page 2 is named; for Elastic IPs and key pairs, whose API does not paginate, the test instead asserts that no MaxResults is sent. A 403 (UnauthorizedOperation or AccessDeniedException) fails naming the collection and never reports clean. A token cycle A→B→A fails naming the collection.
- Each exclusion has a test that fails when the exclusion is removed: a terminated instance, a VPC's default group, a main route table, a deleted NAT gateway, the stamp, the claim. The default VPC is a stray.
- Cross-collection settle. On poll 1 an instance is shutting-down, and its ENI and root volume are in-use. On poll 2 all three are gone. The sweep passes, and each of the collection's Describe calls was sent twice. At the bound, the sweep fails naming each remaining item and its state.
- DescribeImages is sent with IncludeDisabled=true and Owner.1=self. A test fails on the request body when either is dropped.
- One tagged and one untagged stray are both named.
- TestAWSSweepAgainstFakeaws (internal/e2e): empty fakeaws is clean. A VPC, subnet, security group, key pair, Elastic IP, instance and /other/x parameter created through fakeaws are each named. The test is in the ci.yml list, the SHA is bumped, and every test already listed still PASSes.
