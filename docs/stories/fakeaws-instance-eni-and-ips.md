---
kind: code
status: blocked
blocked_by: [fakeaws-sg-ip-permissions]
epic: fakeaws-step-one-surfaces
repo: fakeaws
depends_on: [fakeaws-sg-ip-permissions]
touches: ["handlers/ec2.go", "handlers/ec2_test.go", "repository/ec2_compute.go", "repository/ec2_compute_test.go", "examples/working/instance_public_ip/main.tf (new)", "CHANGELOG.md"]
risk: high
---

# Item 4 (+ item 8 instance IPs): RunInstances via NetworkInterface.N.*, a primary ENI, public/private IPs

RunInstances accepts NetworkInterface.N.{DeviceIndex,SubnetId,SecurityGroupId.M,AssociatePublicIpAddress,DeleteOnTermination} with no top-level SubnetId (today 409, ec2.go:1417-1425). Both forms go through one function doing the subnet lookup and the SG-VPC check (ec2.go:1444-1459), with the same error codes. Top-level SubnetId plus NetworkInterface.1.SubnetId returns InvalidParameterCombination. Each instance persists a primary ENI in a new table (prependResetTables) with a distinct private IP in the subnet CIDR, sourceDestCheck true, and an association.publicIp only when AssociatePublicIpAddress=true; otherwise there is no association element, because the provider sets associate_public_ip_address = Association != nil and the attribute is ForceNew (terraform-provider-aws ec2_instance.go:107-111,3358). The provider also reads source_dest_check from the ENI (:3354-3356). ec2InstanceXML (:1307-1315) gains ipAddress, privateIpAddress, sourceDestCheck and networkInterfaceSet, with groupSet taken from the primary ENI (moto's bug, .swarm/research/moto.md:317-331, not inherited). DescribeNetworkInterfaces (:71-79) returns ENIs, filtered on network-interface-id and attachment.instance-id. ModifyNetworkInterfaceAttribute Groups (with the SG-VPC check) updates the ENI and so the instance groupSet; the provider sends SG changes there (ec2_instance.go:1587-1613). TerminateInstances deletes the ENI. /mock/state gains ec2.network_interfaces, and each instance gets public_ip and private_ip.

**Done when:**
- Handler test: NetworkInterface.1.* only (subnet, two SGs, AssociatePublicIpAddress=true) returns 200 (409 today); DescribeInstances has a non-empty ipAddress equal to networkInterfaceSet[0].association.publicIp, a privateIpAddress inside the subnet CIDR, and an instance groupSet equal to the two SGs, pinned by a CRITICAL[...]/TestContract pair in ec2_test.go
- Negative controls: with AssociatePublicIpAddress=false, and with the top-level SubnetId form, there is no ipAddress and no association element; two instances in one subnet get distinct private IPs
- The NIC path with an SG from another VPC, or an unknown subnet, fails exactly as the top-level path does; SubnetId plus NetworkInterface.1.SubnetId returns InvalidParameterCombination
- ModifyNetworkInterfaceAttribute Groups changes the instance groupSet, and an SG from another VPC is refused
- After TerminateInstances, an unfiltered DescribeNetworkInterfaces and /mock/state ec2.network_interfaces are both empty; /mock/state instance public_ip and private_ip equal DescribeInstances'
- examples/working/instance_public_ip (associate_public_ip_address = true, no user_data, explicit endpoints block) passes in the provider-smoke job
