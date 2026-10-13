---
kind: code
repo: fakeaws
status: ready
epic: fakeaws-loader
depends_on: [fakeaws-load-security-groups]
touches: [handlers/load_ec2_compute.go, handlers/load_ec2_compute_test.go, repository/load_ec2.go, handlers/ec2.go, handlers/ec2_test.go, repository/ec2_compute.go, repository/ec2.go, repository/repository.go, repository/testdata/schema.golden, testdata/ec2/DescribeNetworkInterfaces.xml, testdata/ec2/DescribeAddresses.xml, testdata/ec2/DescribeKeyPairs.xml]
---

# A network interface may have no instance; ENIs, Elastic IPs and key pairs load as recorded

ec2_network_interfaces.instance_id becomes nullable, keeping the cascade for attached ones
(repository/ec2_compute.go:54-64); dbSchemaVersion is bumped and the golden regenerated.

ec2ENIToXML (handlers/ec2.go:2345) renders an attachment only when attached, and gains
description, macAddress, privateDnsName, availabilityZone, requesterManaged, interfaceType,
ownerId, status, privateIpAddressesSet and association, all struct fields. Every ENI consumer
tolerates a missing instance: instanceENIs, the terminate path (:2913-2970) and the
DescribeNetworkInterfaces filters, where `attachment.instance-id` matches no standalone ENI.

Loaders for DescribeNetworkInterfaces, DescribeAddresses and DescribeKeyPairs, in
handlers/load_ec2_compute.go. Key pairs gain keyType and createTime (created_at from the body).
EIPs gain publicIpv4Pool, networkBorderGroup and the association ids (repository/ec2.go:125-137,
handlers/ec2.go:1241-1251, :2963-2975). An attached ENI references its instance, which
fakeaws-load-instances loads; until then it is refused as dangling. EIPs are proved on the
API-reference fixture only.

**Done when:**
- The three Describes, re-sent with the recorded params, answer bodies load_diff.go finds equal to
  the fixtures in both directions.
- A standalone ENI loads, and DescribeNetworkInterfaces answers it with no attachment element; an
  EIP associated with it answers its recorded association ids.
- A key pair answers keyType and createTime as recorded, and `KeyPairId.N` finds it.
- Mutate once: after ModifyNetworkInterfaceAttribute Description on the loaded ENI,
  DescribeNetworkInterfaces equals the recording except at description; after DisassociateAddress,
  DescribeAddresses equals it except at the association fields. Key pairs have no mutating action
  but tags.
- An ENI attached to an instance absent from the recording refuses, naming both.
- RunInstances, TerminateInstances and the existing ENI tests pass unchanged; a file at the
  previous schema version is refused.
