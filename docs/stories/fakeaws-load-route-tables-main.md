---
kind: code
repo: fakeaws
status: ready
epic: fakeaws-loader
depends_on: [fakeaws-load-vpc-subnet-igw-az, fakeaws-load-ssm-parameters]
touches: [handlers/load_ec2_routes.go, handlers/load_ec2_routes_test.go, repository/load_ec2.go, handlers/ec2.go, handlers/ec2_test.go, repository/ec2.go, repository/ec2_test.go, repository/repository.go, repository/testdata/schema.golden, testdata/ec2/DescribeRouteTables.xml]
---

# Route tables load with their main association, local route and recorded association ids

A loader for DescribeRouteTables in handlers/load_ec2_routes.go.

A main association has `main=true` and no subnet, which ec2_route_table_associations cannot hold
(subnet_id NOT NULL, UNIQUE per subnet, repository/ec2.go:88-97). subnet_id becomes nullable, a
`main` column is added, and a partial unique index allows one main association per VPC.
dbSchemaVersion is bumped and the golden regenerated (after fakeaws-load-ssm-parameters' bump,
hence the dependency).

The XML gains association main, associationState and gatewayId, plus each route's origin and state
(handlers/ec2.go:1054-1075); new route and association fields are struct fields. The local route
loads as recorded. Every association consumer tolerates a subnet-less row: the DeleteSubnet
cascade, DisassociateRouteTable and the `association.*` filters. CreateVpc still makes no main
route table; fakeaws-loader-pin records that in D7.

**Done when:**
- DescribeRouteTables, re-sent with the recorded params, answers a body load_diff.go finds equal to
  the fixture in both directions.
- A recorded main route table answers association main=true with no subnetId and its `rtbassoc-`
  id as recorded, and filter `association.main=true` selects it.
- A second main association for one VPC refuses the load naming the VPC.
- Mutate once: after CreateRoute on a loaded route table, DescribeRouteTables equals the recording
  except for the new routeSet item.
- DisassociateRouteTable on the main association is refused (an `UNVERIFIED(aws-code)` code from
  the EC2 API reference), and the association stays.
- Subnet associations, DeleteSubnet and the existing route-table tests pass unchanged; a file at the
  previous schema version is refused.
