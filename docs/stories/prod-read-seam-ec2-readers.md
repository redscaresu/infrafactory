---
kind: code
status: blocked
blocked_by: [prod-read-seam-read-paging]
epic: prod-read-seam
depends_on: [prod-read-seam-read-paging]
touches: [internal/harness/aws_reader_ec2.go, internal/harness/aws_reader_ec2_test.go, internal/harness/aws_reader_table.go, docs/layer3/aws/reader-policy.json, internal/e2e/aws_reader_ec2_fakeaws_test.go]
---

# EC2 readers: each touched VPC's surroundings, the follow-ups, the AZs and the name lookups

New `aws_reader_ec2.go`, which ReadAWSAccount calls after the replay. Each follow-up is an ADR-0041 entry sent through read-core's replay, so it is signed, paged, throttled and recorded the same way.

**Surroundings.** For every VPC id in any replayed exchange: subnets, SGs, SG rules (DescribeSecurityGroupRules), route tables, internet gateways, network interfaces, instances, addresses and key pairs, each filtered to that VPC where the API allows.

**Follow-ups.** DescribeVpcAttribute (enableDnsSupport, enableDnsHostnames); per instance, DescribeInstanceAttribute with userData recorded raw, plus the attributes the 5.100.0 provider reads (cited in the PR). DescribeAvailabilityZones runs once. Images are read only when the read set names them, by replay.

**Name lookups**, as read-set entries the replay answers:
- Key pair by name: DescribeKeyPairs `KeyName.1`.
- SG within a VPC: DescribeSecurityGroups with `Filter.N` group-name plus vpc-id or tag-key. Never `GroupName.N`, which does not find a non-default-VPC SG.
- IAM role: GetRole `RoleName`. Parameter: GetParameter `Name`.

If a row is missing, this PR adds it to both aws_reader_table.go and reader-policy.json; the tie test holds them together.

**Done when:**
- Recording-fake test with two touched VPCs and an untouched third: both touched VPCs get their surroundings, the third gets none, and every surroundings call carries its VPC filter verbatim.
- Every instance gets DescribeInstanceAttribute userData, recorded raw; every VPC gets both attribute calls; the AZs are read once.
- A two-page DescribeInstances is recorded whole; a repeated nextToken fails the read, naming the call.
- A key-pair lookup sends `KeyName.1` and records the match. An SG lookup sends group-name and vpc-id filters verbatim, sends no `GroupName.N`, and returns only the SG in that VPC when a same-named SG exists in another VPC.
- One NotFound row per lookup, each recorded with its status: IAM role `NoSuchEntity`, key pair `InvalidKeyPair.NotFound`, parameter `ParameterNotFound`, SG by absent `GroupId.1` `InvalidGroup.NotFound`. An SG filter lookup that matches nothing records 200 with no groups.
- internal/e2e against fakeaws at FAKEAWS_SHA records every surroundings and follow-up row that fakeaws answers at the pin. The PR lists each row the pin refuses (e.g. the vpc-id filter on SGs, DescribeAvailabilityZones) as owned by prod-read-seam-fakeaws-filtered-e2e.
- TestAWSReaderPolicyIsExactlyTheReaderTable passes; the PR records a cp-backup break that turns each new test red.
