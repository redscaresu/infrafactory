---
kind: code
repo: fakeaws
status: blocked
blocked_by: [fakeaws-vpc-dns-attribute-rules]
depends_on: [fakeaws-vpc-dns-attribute-rules]
touches: [handlers/ec2.go, handlers/ec2_test.go, CHANGELOG.md]
---

# CreateVpc makes the VPC's main route table, as AWS does

Real AWS gives every new VPC a main route table, with a main association and the local route, and
a subnet with no explicit association uses it. fakeaws CreateVpc makes none, so a tofu-built estate
that relies on the main or default table (`aws_default_route_table`, or no explicit association)
reads as unrouted in infrafactory's topology derivation. The generator then gets a false
"unreachable" failure and adds explicit associations real AWS does not need. Found in /code-review
of infrafactory #574.

**Done when:**
- `go test ./handlers/ -run 'TestEC2_CreateVpc_MainRouteTable' -v` passes: after CreateVpc, DescribeRouteTables filtered by `vpc-id` returns one table with an association `Main: true` and a local route for the VPC CIDR; `association.main=true` filtering finds it.
- `aws_default_route_table` adopting that table (by `default_route_table_id` from DescribeVpcs) applies and plans with no diff in a provider smoke example.
- Deleting the VPC deletes its main table; DeleteRouteTable on a main table answers DependencyViolation as AWS does.
- Each new case is shown failing with the creation removed (cp backup).
