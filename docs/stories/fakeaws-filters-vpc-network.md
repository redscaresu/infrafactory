---
kind: code
repo: fakeaws
status: ready
epic: fakeaws-data-sources
depends_on: [fakeaws-ec2-filters-never-ignored]
touches: [handlers/ec2_filters.go, handlers/ec2.go, handlers/ec2_test.go, handlers/regression_test.go, CHANGELOG.md]
---

# DescribeVpcs, DescribeSubnets, DescribeInternetGateways and DescribeRouteTables honour tags and their documented filters

Fill the four tables in ec2_filters.go. Each gets every AWS-documented name whose field the row
stores, plus the camelCase aliases the 5.100.0 data sources send; every other documented name
(dhcp-options-id, ipv6-*) stays refused by the evaluator, named. The PR cites, for each name, the EC2
API reference and the provider source.
- Vpcs: vpc-id, cidr, cidr-block-association.cidr-block, state, owner-id, is-default/isDefault
  (always false: fakeaws has no default VPC, repository/ec2.go:153-160), tags.
- Subnets: subnet-id, vpc-id, cidr-block/cidr/cidrBlock, availability-zone/availabilityZone,
  availability-zone-id/availabilityZoneId (the id the subnet XML emits, ec2.go:634), state,
  default-for-az/defaultForAz (false), map-public-ip-on-launch, owner-id, tags.
- InternetGateways: internet-gateway-id, attachment.vpc-id, attachment.state (available when
  attached), owner-id, tags.
- RouteTables: today's three plus route-table-id, association.subnet-id, tags.

These are the reads prod-read-seam-fakeaws-filtered-e2e and prod-read-seam-ec2-readers make.

**Done when:**
- With two VPCs, one tagged infrafactory-run-id, DescribeVpcs Filter tag-key=infrafactory-run-id
  returns exactly the tagged one. tag:Name, tag-value and cidr each select one of the two. A filter
  matching nothing answers 200 with an empty vpcSet.
- With three subnets over two VPCs, vpc-id plus availability-zone, and tag:Name alone, each return
  only the matching subnet; the alias cidrBlock answers like cidr-block.
- DescribeInternetGateways attachment.vpc-id returns only the IGW attached to that VPC;
  attachment.state=available excludes a detached IGW.
- DescribeRouteTables vpc-id plus tag:Name returns one table; DescribeVpcs Filter dhcp-options-id
  answers 501 naming it, classed filter-refused.
- A regression test in handlers/regression_test.go; CHANGELOG [Unreleased] lists the names.
- Each new test is shown failing by a cp-backup break (one table entry removed per Describe),
  recorded in the PR.
