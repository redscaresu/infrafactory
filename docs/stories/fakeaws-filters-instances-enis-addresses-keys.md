---
kind: code
repo: fakeaws
status: blocked
blocked_by: [fakeaws-filters-security-groups]
epic: fakeaws-data-sources
depends_on: [fakeaws-filters-security-groups]
touches: [handlers/ec2_filters.go, handlers/ec2.go, handlers/ec2_test.go, handlers/regression_test.go, CHANGELOG.md]
---

# DescribeInstances, DescribeNetworkInterfaces, DescribeAddresses and DescribeKeyPairs honour tags and their documented filters; DescribeInstances answers one region

These are the vpc-id surroundings prod-read-seam-ec2-readers reads. Every name not listed stays
refused by the evaluator, named.
- **Instances:** instance-id, instance-state-name, instance-type, image-id, subnet-id, vpc-id,
  private-ip-address / ip-address (from the primary ENI), availability-zone (the subnet's),
  owner-id, tags. The unfiltered list stops being account-wide (ec2.go:2301-2305): a filtered data
  source must not return another region's instance with 200. InstanceId.N is already region-scoped
  (:2286-2296).
- **NetworkInterfaces:** today's four plus vpc-id, private-ip-address, attachment.attachment-id,
  owner-id, tags.
- **Addresses:** association-id plus allocation-id, domain, public-ip, instance-id,
  network-interface-id, private-ip-address, tags. PublicIp.N is honoured instead of refused (:1174);
  an unknown address answers 400 InvalidAddress.NotFound.
- **KeyPairs:** KeyName.N as today; KeyPairId.N honoured instead of refused (unknown answers
  InvalidKeyPair.NotFound); filters key-name, key-pair-id, fingerprint, tags.

**Unverified codes.** InvalidAddress.NotFound for an unknown PublicIp.N and InvalidKeyPair.NotFound
for an unknown KeyPairId.N come from the EC2 API reference, not a recording. Each handler site and
its test carry `UNVERIFIED(aws-code): <code> from <doc URL>; no recording`; fakeaws-data-sources-pin
records them in the deltas file.

**Done when:**
- With instances in VPC A, VPC B and eu-west-2: DescribeInstances vpc-id=A returns only A's;
  instance-state-name=running excludes a terminated one; an unfiltered call in us-east-1 omits the
  eu-west-2 instance.
- DescribeNetworkInterfaces vpc-id=A returns only A's ENI.
- DescribeAddresses PublicIp.1 returns that address, and an unknown one answers 400
  InvalidAddress.NotFound. The PublicIps case in TestRegressionSweepDescribeAddressesListsEveryAddress
  changes to expect this, and the PR names the change.
- DescribeKeyPairs KeyPairId.1 and Filter key-name each return the key; an unknown KeyPairId answers
  InvalidKeyPair.NotFound.
- `git grep -n 'UNVERIFIED(aws-code)' handlers/` shows both codes at their handler and test.
- A regression test in handlers/regression_test.go; CHANGELOG has an entry naming the region change.
- Each new test is shown failing by a cp-backup break, recorded in the PR (one table entry removed;
  the region scope dropped from the unfiltered list).
