---
kind: code
status: ready
epic: fakeaws-step-one-surfaces
repo: fakeaws
depends_on: [fakeaws-smoke-in-ci]
touches: ["handlers/ec2.go", "handlers/ec2_test.go", "examples/working/security_group_full_shape/main.tf (new)", "CHANGELOG.md"]
risk: high
---

# Item 8: security groups keep and export the whole ip_permissions shape, ingress and egress

The permission type and XML (ec2.go:974-1003) and parseIpPermissions (:1016-1062) carry IpRanges (CidrIp, Description), Ipv6Ranges, UserIdGroupPairs (GroupId, UserId, Description) and PrefixListIds, in both directions, from inline blocks and from Authorize*/Revoke*. Revoke removes only the matching entry. gatherEC2StateReal (:1965-2030) exports ip_permissions and ip_permissions_egress per group, with snake_case AWS keys (from_port, to_port, ip_protocol, ip_ranges[{cidr_ip, description}], ipv6_ranges, user_id_group_pairs, prefix_list_ids), which the ingress policy's deny_state reads. No validation here. The new example keeps the explicit endpoints block and pins 5.100.0.

**Done when:**
- Handler test: tcp 22 from ::/0 appears in DescribeSecurityGroups ipv6Ranges and in /mock/state; an SG-to-SG rule round-trips as a UserIdGroupPair; a pl- rule round-trips; egress -1 0.0.0.0/0 is in ip_permissions_egress; revoking ::/0 leaves the IPv4 rule. Each assertion fails on today's ec2.go
- A golden test pins the exact /mock/state permission JSON keys
- A CRITICAL[...] docstring in ec2.go, paired with its TestContract_* in ec2_test.go, passes contract_audit_test
- examples/working/security_group_full_shape (ipv6 ingress, SG-reference rule, allow-all egress) passes in the provider-smoke job
