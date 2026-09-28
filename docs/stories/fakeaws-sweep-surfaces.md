---
kind: code
status: ready
epic: aws-layer3-claim-sweep-reap
repo: fakeaws
depends_on: []
touches: ["handlers/ec2.go", "handlers/ec2_test.go", "coverage_matrix.yaml", "CHANGELOG.md"]
risk: high
---

# Every EC2 Describe the AWS scope sweep calls answers on fakeaws, in the real wire shape

Close each fakeaws gap that stops a whole-scope sweep. (1) An unfiltered DescribeAddresses lists every EIP in the account and region. Today it describes nothing without AllocationId.N (handlers/ec2.go:1026-1031). (2) DescribeSecurityGroups with no GroupId.N lists every group. Today it answers 409 (ec2.go:1300-1303). (3) DescribeImages with Owner.1=self returns only caller-owned images, so none of the canonical fixtures, and it accepts IncludeDisabled=true. Today Owner is ignored (ec2.go:2382-2416). ImageId.N lookups are unchanged. (4) New DescribeVolumes, DescribeNatGateways, DescribeSnapshots (Owner.1=self) and DescribeLaunchTemplates answer the real result shapes. They return empty, since fakeaws creates none, and refuse an unmodelled Filter.N rather than ignoring it (the ec2.go:2081-2092 convention). (5) An unfiltered DescribeNetworkInterfaces does not list a terminated instance's ENI.

**Done when:**
- Through aws-sdk-go-v2 against fakeaws, an unfiltered DescribeAddresses lists an address made by AllocateAddress. At the previous HEAD it lists nothing, and the regression test fails there.
- An unfiltered DescribeSecurityGroups lists a created group. At the previous HEAD it answers 409.
- DescribeImages Owners=[self] IncludeDisabled=true returns zero images while the fixtures exist. DescribeImages ImageIds=[<AL2023 fixture>] still returns that fixture.
- DescribeVolumes, DescribeNatGateways, DescribeSnapshots OwnerIds=[self] and DescribeLaunchTemplates each decode through the SDK to an empty list. Each refuses an unmodelled Filter.N with 501 or 409, and a test fails if the filter is ignored.
- After RunInstances then TerminateInstances, an unfiltered DescribeNetworkInterfaces omits the instance's ENI, and DescribeInstances reports the instance as terminated.
- Each surface has a coverage_matrix.yaml row and a regression test. The contract audit and smoke examples pass, and CHANGELOG.md names the surfaces.
