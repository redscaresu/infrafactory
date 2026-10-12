---
kind: code
repo: fakeaws
status: blocked
blocked_by: [fakeaws-load-exchange-rules]
epic: fakeaws-loader
depends_on: [fakeaws-load-exchange-rules]
touches: [handlers/load_ec2_network.go, handlers/load_ec2_network_test.go, repository/load_ec2.go, repository/load_ec2_test.go, handlers/ec2.go, handlers/ec2_test.go, repository/ec2.go, testdata/ec2/DescribeVpcs.xml, testdata/ec2/DescribeVpcAttribute-enableDnsSupport.xml, testdata/ec2/DescribeVpcAttribute-enableDnsHostnames.xml, testdata/ec2/DescribeSubnets.xml, testdata/ec2/DescribeInternetGateways.xml, testdata/ec2/DescribeAvailabilityZones.xml]
---

# VPCs and both DNS attributes, subnets, internet gateways and AZs load as recorded, tags included, and answer exactly as recorded

Loaders for DescribeVpcs, DescribeVpcAttribute (enableDnsSupport and enableDnsHostnames),
DescribeSubnets, DescribeInternetGateways and DescribeAvailabilityZones, in
handlers/load_ec2_network.go, writing through repository/load_ec2.go.

Widen the XML to the real shape:
- VPC: isDefault, dhcpOptionsId, instanceTenancy, ownerId, the cidrBlockAssociationSet ids and
  states, ipv6CidrBlockAssociationSet. Today these are hard-coded or synthesised
  (handlers/ec2.go:740-759, :313-314, :964-975).
- Subnet: subnetArn, ownerId, mapPublicIpOnLaunch, availableIpAddressCount, defaultForAz,
  privateDnsNameOptionsOnLaunch, and the rest the 5.100.0 provider reads.
- IGW: ownerId and the attachment state.
- AZ: the table's fields (repository/ec2.go:138-148). A field the table cannot hold is refused by
  the rendering diff, or ignore-listed with a cite.

New fields are struct fields, stored in each row's data JSON; no column changes. Every tagSet
loads into ec2_tags. Both VPC attributes are stored per VPC: DescribeVpcAttribute answers them,
and ModifyVpcAttribute persists either one (today both are constants, handlers/ec2.go:395-428). A
created VPC stores today's answers (true, true); its AWS default is out of scope
(fakeaws-vpc-dns-hostnames-default). dhcpOptionsId is a reference the reader never reads. A
subnet's recorded subnetArn is stored, not derived. Fixtures: testdata/ec2/<Action>.xml from the
EC2 API reference.

**Done when:**
- The fixtures load. DescribeVpcs, both DescribeVpcAttribute calls, DescribeSubnets,
  DescribeInternetGateways and DescribeAvailabilityZones, re-sent with the recorded params, answer
  bodies load_diff.go finds equal to the fixtures in both directions.
- A VPC recorded with isDefault true, dhcpOptionsId dopt-0abc, enableDnsSupport false and
  enableDnsHostnames false answers all four. After ModifyVpcAttribute EnableDnsSupport=true,
  DescribeVpcAttribute enableDnsSupport answers true and enableDnsHostnames still false; after
  EnableDnsHostnames=true, it answers true.
- Mutate once: after ModifySubnetAttribute MapPublicIpOnLaunch on a loaded subnet, DescribeSubnets
  equals the recording except at mapPublicIpOnLaunch; after ModifyVpcAttribute, DescribeVpcs
  equals it exactly; after DetachInternetGateway, DescribeInternetGateways equals it except at the
  attachment. AZs have no mutating action.
- A recorded subnetArn that differs from BuildEC2SubnetARN's answers as recorded.
- Planting `<fooBar><x>1</x></fooBar>` under vpcSet/item refuses the load naming DescribeVpcs and
  `vpcSet/item[0]/fooBar/x`, and nothing is stored.
- Filter `tag-key=infrafactory-run-id` selects the loaded tagged VPC; `tag:Name` selects its
  subnet.
- A subnet whose vpcId is not in the recording is refused naming both ids.
- A VPC and subnet created after the load answer as before, and ec2_test.go passes unchanged.
