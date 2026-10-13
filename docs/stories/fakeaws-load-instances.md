---
kind: code
repo: fakeaws
status: ready
epic: fakeaws-loader
depends_on: [fakeaws-load-network-interfaces]
touches: [handlers/load_ec2_compute.go, handlers/load_ec2_compute_test.go, repository/load_ec2.go, handlers/ec2.go, handlers/ec2_test.go, repository/ec2_compute.go, testdata/ec2/DescribeInstances.xml, testdata/ec2/DescribeInstanceAttribute-userData.xml, testdata/ec2/DescribeInstanceAttribute-disableApiTermination.xml, testdata/ec2/DescribeInstanceAttribute-disableApiStop.xml, testdata/ec2/DescribeInstanceAttribute-instanceInitiatedShutdownBehavior.xml]
---

# Instances load with the shape terraform-provider-aws 5.100.0 reads and their recorded DescribeInstanceAttribute values

A loader for DescribeInstances and DescribeInstanceAttribute. The reader reads
instanceInitiatedShutdownBehavior, disableApiStop, disableApiTermination and userData (infrafactory
internal/harness/aws_reader_ec2.go:17-21).

Widen ec2InstanceXML (handlers/ec2.go:2205-2223, :2317-2343) to what 5.100.0 reads, as struct
fields: launchTime, placement, privateDnsName, dnsName, architecture, rootDeviceName/Type,
blockDeviceMapping, monitoring, hypervisor, virtualizationType, clientToken, metadataOptions,
vpcId, cpuOptions, capacityReservationSpecification, hibernationOptions, enclaveOptions,
maintenanceOptions, privateDnsNameOptions, platformDetails, usageOperation, bootMode. Fields it
never reads go on the ignore list, each with a cite. iamInstanceProfile answers the recorded
`AIPA…` id; today it answers the name (:2329).

- The attribute values are stored as recorded, userData byte-exact.
- The primary ENI in networkInterfaceSet must agree with its DescribeNetworkInterfaces item, or the
  load is refused naming the field.
- imageId, and an iamInstanceProfile with no IAM exchange, are references the reader never reads.
- keyName must resolve to a loaded key pair.
- A blockDeviceMapping EBS volume must resolve to a loaded volume (fakeaws-load-volumes-d7), or
  the load is refused. Until that story lands, only an instance with no block device mapping loads.

**Done when:**
- An instance with no block device mapping loads, and DescribeInstances by id and by vpc-id answers
  bodies load_diff.go finds equal to the fixture in both directions, launchTime and privateDnsName
  included.
- DescribeInstanceAttribute answers each of the four as recorded; a userData with a trailing
  newline and a CR reads back byte-exact.
- iamInstanceProfile's id answers the recorded `AIPA…` id.
- RunInstances with a loaded instance profile whose recorded ARN has a path answers that stored
  ARN, not one built from the name (handlers/ec2.go builds it today; found reviewing fakeaws#82).
- Mutate once: after ModifyInstanceAttribute DisableApiTermination=true on the loaded instance,
  DescribeInstances equals the recording exactly, and DescribeInstanceAttribute changes only
  disableApiTermination; userData is still byte-exact.
- An instance whose blockDeviceMapping names vol-0abc, with no DescribeVolumes exchange, refuses
  naming the instance and vol-0abc.
- A primary ENI whose privateIpAddress differs between the two exchanges refuses naming both; a
  keyName absent from the recording refuses; an absent imageId is answered under unresolved.
