---
kind: code
status: blocked
blocked_by: [fakeaws-instance-eni-and-ips]
epic: fakeaws-step-one-surfaces
repo: fakeaws
depends_on: [fakeaws-instance-eni-and-ips]
touches: ["handlers/ec2.go", "handlers/ec2_test.go", "repository/ec2.go", "repository/ec2_test.go", "repository/ec2_compute.go", "repository/ec2_compute_test.go", "coverage_matrix.yaml", "examples/updates/update_subnet_map_public_ip/ (new)", "examples/known_red_test.go", "CHANGELOG.md"]
---

# Items 5 + 6: ModifySubnetAttribute persists MapPublicIpOnLaunch; RunInstances persists UserData

MapPublicIpOnLaunch.Value replaces the no-op (ec2.go:63-70); other attributes keep the documented no-op. EC2Subnet gains the field (repository/ec2.go:160-169) and DescribeSubnets echoes mapPublicIpOnLaunch. A top-level launch into such a subnet gets a public IP and association from the story-F allocator. RunInstances persists UserData (repository/ec2_compute.go:85-96) and DescribeInstanceAttribute userData returns it (today "", ec2.go:1888-1889). The coverage_matrix aws_subnet row moves from updates_exempt, whose 'Subnet immutable' reason becomes false (coverage_matrix.yaml:158-159), to updates_dir_name. basic_instance leaves knownRed.

**Done when:**
- Handler tests: MapPublicIpOnLaunch true then false flips DescribeSubnets; a top-level launch into a flagged subnet has ipAddress and one into an unflagged subnet does not; UserData base64 round-trips and a missing UserData returns an empty value. Each fails on today's code
- In the provider-smoke job, examples/updates/update_subnet_map_public_ip (v1 false, v2 true) passes (today the provider's wait taints the subnet, .swarm/moto/plan-inline-fakeaws-true.log:83-91), and basic_instance passes with basic_instance deleted from knownRed
- coverage-audit passes with the aws_subnet row naming the updates dir
