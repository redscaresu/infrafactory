---
kind: code
status: ready
epic: aws-layer-neutral-hcl
depends_on: []
touches: ["internal/harness/topology_derive.go", "internal/harness/topology_derive_aws.go (new)", "internal/harness/topology_derive_aws_test.go (new)", "internal/harness/testdata/topology/aws/ (new: captures + capture HCL)", "docs/stories/aws-compute-http-probe.md (delete)"]
---

# deriveTopologyAWS emits compute:<port> only when the instance's own group, its IPv4 public address and an IGW route make it reachable

Replace the stub deriveTopologyAWS (internal/harness/topology_derive.go:114-124) with a deriver in a new topology_derive_aws.go. It reads the /mock/state ec2 export on fakeaws origin/main (handlers/ec2.go:2783-2950): instances, security_groups, route_tables, routes, route_table_associations, internet_gateways. It emits one key compute:<port> for every port named by any security group's single-port tcp ingress rule with a public IPv4 source, 0.0.0.0/0 in ip_ranges. A key is true only if some instance meets all three conditions. (1) The port is admitted by a group in that instance's OWN vpc_security_group_ids. (2) public_ip is non-empty; it is IPv4, and the HLD probe reads aws_instance.public_ip (docs/hld/2026-09-27-aws-web-stack.md:284-285). (3) The subnet's explicitly associated route table has a 0.0.0.0/0 route whose gateway_id is an internet gateway of the same VPC; the export has no main-table flag. Every false key gets a diagnostic naming its first failing condition. diagnostics["compute"] covers the rest: an IPv6-only ::/0 rule, which the IPv4 probe cannot use; a port range or protocol -1 rule (ponytail: ranges not derived); no public rule at all. There is no EIP branch, because fakeaws has no AssociateAddress (none in handlers/ec2.go) and HLD step one uses associate_public_ip_address. Connectivity stays empty. Scenario outcomes do not change: none of the 11 scenarios/training/aws-*.yaml declares http_probe, and EvaluateTopology runs only when topologyChecks is non-empty (internal/cli/test_command.go:1428). Every fixture is a trimmed /mock/state capture from applying a small HCL variant against fakeaws >= 5cf7693; the capture HCL and command sit beside the fixture. No ADR: internal/harness is not a decision path.

**Done when:**
- The positive capture derives {"compute:80": true}: one instance with public_ip set, an attached group admitting tcp 80 from 0.0.0.0/0, and the subnet associated with a table routing 0.0.0.0/0 to the VPC's IGW
- Each negative capture flips one condition. Each derives compute:80 false with its own diagnostic, asserted by string: the rule is on an unattached group; no 0.0.0.0/0 route to an IGW; no route table association; associate_public_ip_address = false; the rule's source is 10.0.0.0/16; the rule's only source is ::/0 (the diagnostic names IPv6-only ingress)
- A capture whose only rule is 80-90 or protocol -1 emits no compute key, and diagnostics["compute"] names the reason
- An aws-instance-shaped capture (instance, attached group, no public rule) emits no compute key and a diagnostics["compute"] reason. No existing scenario's outcome changes, because EvaluateTopology is gated on topologyChecks (test_command.go:1428)
- Mutation: dropping the route check, dropping the public_ip check, or accepting ::/0 fails at least one named negative. Checking every group in the VPC instead of the instance's own also fails one
- TestDetectCloud and the existing Scaleway and GCP topology tests pass unmodified
