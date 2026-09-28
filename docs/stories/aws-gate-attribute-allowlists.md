---
kind: code
status: blocked
blocked_by: [layer3-gate-shared-rules]
epic: aws-layer3-gate
depends_on: [layer3-gate-shared-rules]
touches: ["internal/cli/layer3_aws_attrs.go (new)", "internal/cli/layer3_aws_attrs_test.go (new)", "docs/stories/aws-gate-attribute-allowlists.md (delete)"]
risk: high
---

# AWS gate, resource leg: attribute and nested-block allowlists for all nine types, cost bounds, standard in-region zones only, typed parent references, exactly one aws_instance and at most one aws_eip

New file internal/cli/layer3_aws_attrs.go holds one table in the layer3EnumBounds shape (layer3_hcl_shape.go:920-923). Anything unlisted is refused (HLD contradiction 14). A table entry is either an attribute entry or a nested-block entry. Attribute entries match only hclsyntax Attributes, and nested-block entries match only hclsyntax Blocks. So the attribute-as-blocks forms (ConfigModeAttr in 5.100.0: vpc_security_group.go:127, vpc_route_table.go:91) are unlisted attributes and are refused.

Rules:
- enum, numeric max and bool, with values resolved through layer3ResolveConstant (:991-1008); an unresolvable value is refused;
- typed reference: a bare <type>.<name>.id to that exact stack-local type, or a list of them;
- zone: matches ^<QuoteMeta(region)>[a-z]$, a standard zone letter, which rejects Local Zones and Wavelength;
- any: free, but still under the cloud-neutral rules;
- delegated: checked by aws-gate-user-data-and-ami.

Per type:
- aws_instance: ami (delegated); instance_type {t3.micro, t3.small}; subnet_id ref aws_subnet; vpc_security_group_ids refs aws_security_group; associate_public_ip_address bool; user_data (delegated); tags; and at most one root_block_device block, whose own allowlist is volume_size <= 20, volume_type gp3 and delete_on_termination true.
- aws_vpc: cidr_block, enable_dns_support, enable_dns_hostnames, instance_tenancy {default}, tags.
- aws_subnet: vpc_id ref aws_vpc, cidr_block, availability_zone (zone rule), map_public_ip_on_launch bool, tags.
- aws_internet_gateway: vpc_id ref aws_vpc, tags.
- aws_route_table: vpc_id ref aws_vpc, tags.
- aws_route: route_table_id ref aws_route_table, destination_cidr_block, gateway_id ref aws_internet_gateway.
- aws_route_table_association: subnet_id ref aws_subnet, route_table_id ref aws_route_table.
- aws_security_group: name, description, vpc_id ref aws_vpc, tags, and ingress and egress blocks holding only from_port, to_port, protocol, cidr_blocks, ipv6_cidr_blocks and description. Which ports may be public is the Rego policy's decision.
- aws_eip: domain {vpc}, instance ref aws_instance, tags.

depends_on is admitted everywhere. count, for_each, provider and lifecycle have their own rules.

The gate calls awsResourceProblems(block, file, varDefaults, region) per resource, and awsMultiplicityProblems(parsed map[string]*hclsyntax.Body from layer3ParseDir) for the stack: exactly one aws_instance, and at most one aws_eip, the HLD's two-address bound. An omitted root_block_device is bounded by awsAMIRootProblems (aws-gate-user-data-and-ami).

**Done when:**
- Admit: awsResourceProblems returns nothing for any resource in internal/e2e/testdata/aws-web-step-one/web-step-one.tf, nor for root_block_device {volume_size = 20, volume_type = "gp3", delete_on_termination = true} or aws_eip {domain = "vpc", instance = aws_instance.web.id}. awsMultiplicityProblems returns nothing for that fixture
- awsResourceProblems refuses each of these as an unlisted attribute, naming type, name and attribute: `ingress = [{... security_groups = ["sg-0123"], prefix_list_ids = ["pl-x"] ...}]` and `egress = [{...}]` on aws_security_group; `route = [{cidr_block = "0.0.0.0/0", gateway_id = aws_internet_gateway.main.id}]` on aws_route_table; `root_block_device = [{...}]` on aws_instance
- awsResourceProblems refuses these root_block_device fields as unlisted nested attributes: iops = 3001, throughput = 126, encrypted = true, kms_key_id, tags. It refuses these bound violations: volume_size 21, volume_type gp2, delete_on_termination = false, two root_block_device blocks, volume_size = var.v with no default
- awsResourceProblems refuses these on aws_instance, naming the attribute: tenancy, ebs_block_device, network_interface, iam_instance_profile, disable_api_termination (both true and false), user_data_base64, user_data_replace_on_change, metadata_options, instance_type t3.medium, instance_type = var.t with no default
- awsResourceProblems refuses these availability_zone values against region us-east-1: us-east-1-bos-1a (Local Zone), us-east-1-wl1-bos-wlz-1 (Wavelength), use1-az1 (a zone id), us-east-1 (the region itself), eu-west-1a, var.az with no default. It admits us-east-1a and us-east-1f
- awsResourceProblems refuses each of these: aws_vpc instance_tenancy dedicated and ipv4_ipam_pool_id; aws_subnet outpost_arn and customer_owned_ipv4_pool; aws_eip address, public_ipv4_pool, network_interface and domain standard; aws_route nat_gateway_id, vpc_peering_connection_id and instance_id; aws_route_table_association gateway_id; an ingress block holding prefix_list_ids or security_groups
- awsResourceProblems refuses these as typed-reference violations: the literals vpc-0123, subnet-0123, rtb-0123, igw-0123, sg-0123 and i-0123; var.vpc; vpc_id = aws_subnet.public.id; a ternary that mentions aws_vpc.main.id; aws_vpc.main.arn
- awsMultiplicityProblems refuses, naming the files: zero aws_instance, two aws_instance (also across two files), two aws_eip. awsResourceProblems refuses aws_launch_template and any other type that has no table entry
- doc-hygiene passes, and the tip commit carries 'ADR: none — attribute allowlists and cost bounds decided in HLD 2026-09-27 § The gate'
