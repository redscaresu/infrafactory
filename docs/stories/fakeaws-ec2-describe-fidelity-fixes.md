---
kind: code
repo: fakeaws
status: blocked
blocked_by: [fakeaws-filters-instances-enis-addresses-keys]
depends_on: [fakeaws-filters-instances-enis-addresses-keys]
touches: [handlers/ec2.go, handlers/ec2_test.go, repository/ec2.go, AGENTS.md, CHANGELOG.md]
risk: high
---

# Three EC2 Describe fidelity fixes found reviewing fakeaws#64

- ec2DescribeRouteTables drops the errors from ListRouteTableAssociations and ListRoutes, so a storage
  error answers 200 with an empty table (fail-open).
- DescribeSubnets answers an unknown SubnetId.N with 200 empty instead of InvalidSubnetID.NotFound.
- A repeated VpcId.N returns the same VPC twice.
- From fakeaws#65: CreateVpc creates no `default` security group, which every AWS VPC has, so `aws_default_security_group` cannot be answered and #65 refuses the lookup. Create that row in CreateVpc (and delete #65's rowless refusal), and refuse CreateSecurityGroup with GroupName=default as AWS does (reserved name).

**Done when:**
- A broken route-table association or route read answers 500 naming EC2, never 200 empty.
- DescribeSubnets with an unknown SubnetId.1 answers 400 InvalidSubnetID.NotFound naming the id.
- DescribeVpcs with VpcId.1=VpcId.2=<id> returns that VPC once.
- CreateVpc then DescribeSecurityGroups group-name=default answers that VPC's default group; CreateSecurityGroup GroupName=default answers the AWS error for a reserved name (mark the code UNVERIFIED if not from a recording).
- Each new test is shown failing by a cp-backup break recorded in the PR.
