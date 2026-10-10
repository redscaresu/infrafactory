---
kind: code
status: ready
repo: fakeaws
depends_on: []
touches: [handlers/ec2.go, repository/ec2.go, handlers/regression_test.go]
---

# fakeaws serves AssociateAddress and DisassociateAddress, so an aws_eip attached to an instance applies and destroys

fakeaws has no `AssociateAddress` or `DisassociateAddress` case in its EC2 dispatch
(handlers/ec2.go, `// ----- EIP -----`: Allocate, Describe, Release and DescribeAddressesAttribute
only, at the infrafactory CI pin FAKEAWS_SHA ee7656a and on origin/main f4d20e5), so it answers
both with 404 ResourceNotFoundException. infrafactory allowlists `aws_eip` (infrafactory.yaml:192),
so a model may attach one to the instance; the Layer 2 apply then fails at
`aws_eip.web: associating EC2 EIP ... AssociateAddress ... 404`, a mock gap rather than an HCL
mistake. Seen 2026-10-10 while measuring the IAM policy's apply actions (PR #423): the Layer 2
run's HCL plus `resource "aws_eip" "web" { domain = "vpc", instance = aws_instance.web.id }`.

The provider (hashicorp/aws 5.100.0) sends `AssociateAddress` with `AllocationId` and `InstanceId`,
reads the association back through `DescribeAddresses` (`AssociationId`, `InstanceId`,
`NetworkInterfaceId`, `PrivateIpAddress`), and on destroy sends `DisassociateAddress` with
`AssociationId` before `ReleaseAddress`. Store the association on the EIP row; an instance's
association goes on its primary network interface, as EC2 does. An unknown allocation or instance
id is the EC2 error EC2 returns (`InvalidAllocationID.NotFound`, `InvalidInstanceID.NotFound`);
`ReleaseAddress` on an associated address is `InvalidIPAddress.InUse`, as EC2 does.

**Done when:**
- A regression test in fakeaws: AllocateAddress, RunInstances, AssociateAddress by InstanceId
  returns an `associationId`; DescribeAddresses shows it with the instance and its network
  interface; DisassociateAddress by that id clears it; ReleaseAddress then succeeds. Release while
  associated fails with `InvalidIPAddress.InUse`, and an unknown allocation id fails with
  `InvalidAllocationID.NotFound`.
- The aws-web-live Layer 2 HCL with an `aws_eip` that sets `instance` applies and destroys against
  fakeaws with the pinned provider, with no 404, and fakeaws CI passes on the PR.
