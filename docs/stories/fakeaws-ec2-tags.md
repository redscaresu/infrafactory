---
kind: code
status: ready
epic: fakeaws-step-one-surfaces
repo: fakeaws
depends_on: [fakeaws-sg-ip-permissions, fakeaws-instance-eni-and-ips, fakeaws-sg-ingress-validation]
touches: ["handlers/ec2.go", "handlers/ec2_test.go", "repository/ec2_tags.go (new)", "repository/ec2_tags_test.go (new)", "coverage_matrix.yaml", "examples/updates/update_vpc_network_tags/ (new)", "CHANGELOG.md"]
---

# Item 7: tags round-trip on every step-one resource (TagSpecification, CreateTags, DeleteTags, tagSet)

A tags table (resource id, key, value) in repository/ec2_tags.go, registered with prependResetTables. TagSpecification.N is persisted on CreateVpc, CreateSubnet, CreateInternetGateway, CreateRouteTable, CreateSecurityGroup and RunInstances (instance and network-interface). CreateTags and DeleteTags, today the 404 default (ec2.go:183-190), give an unknown id its typed NotFound code. tagSet is returned on DescribeVpcs, Subnets, InternetGateways, RouteTables, SecurityGroups and Instances. DescribeTags (:1915-1923) honours the resource-id, resource-type and key filters. Deleting a resource deletes its tags. The coverage_matrix aws_vpc row moves from updates_exempt ('VPC at v1 is immutable', coverage_matrix.yaml:150-151, false once tags change in place) to updates_dir_name update_vpc_network_tags.

**Done when:**
- Handler tests per resource type: TagSpecification appears in that type's Describe tagSet; CreateTags adds and DeleteTags removes (key-only and key+value); CreateTags on vpc-doesnotexist returns InvalidVpcID.NotFound; /mock/reset clears tags. Each fails today
- In the provider-smoke job, updates/update_vpc_network_tags (provider default_tags plus a resource tag that changes v1->v2 across vpc, subnet, igw, route table, SG and instance) passes
- coverage-audit passes with the aws_vpc row naming the updates dir
