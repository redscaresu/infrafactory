---
kind: code
repo: fakeaws
status: blocked
blocked_by: [fakeaws-filters-instances-enis-addresses-keys]
depends_on: [fakeaws-filters-instances-enis-addresses-keys]
touches: [handlers/ec2.go, handlers/ec2_test.go, CHANGELOG.md]
risk: high
---

# Three EC2 Describe fidelity fixes found reviewing fakeaws#64

- ec2DescribeRouteTables drops the errors from ListRouteTableAssociations and ListRoutes, so a storage
  error answers 200 with an empty table (fail-open).
- DescribeSubnets answers an unknown SubnetId.N with 200 empty instead of InvalidSubnetID.NotFound.
- A repeated VpcId.N returns the same VPC twice.

**Done when:**
- A broken route-table association or route read answers 500 naming EC2, never 200 empty.
- DescribeSubnets with an unknown SubnetId.1 answers 400 InvalidSubnetID.NotFound naming the id.
- DescribeVpcs with VpcId.1=VpcId.2=<id> returns that VPC once.
- Each new test is shown failing by a cp-backup break recorded in the PR.
