---
kind: code
repo: fakeaws
status: blocked
blocked_by: [fakeaws-images-owner-name-filters, fakeaws-ami-fixtures-suppressed-when-loaded]
epic: fakeaws-data-sources
depends_on: [fakeaws-images-owner-name-filters, fakeaws-ami-fixtures-suppressed-when-loaded]
touches: [handlers/ec2.go, handlers/ec2_filters.go, handlers/ec2_test.go, handlers/regression_test.go, repository/ec2.go, repository/ec2_test.go, AGENTS.md, CHANGELOG.md]
risk: high
---

# DescribeAvailabilityZones: a fixed a-c set for an unloaded database, the AZ table's rows for a loaded one

Add a new arm to handleEC2; today the action falls to the default arm and answers 404
(ec2.go:190-195).

**Unloaded** database: answers `<region>a`-`c` for the request region (ec2.go:42). Each zone has
state available, opt-in-status opt-in-not-required, zone-type availability-zone, and region, group
and network-border-group set to the region. Real sets differ (us-east-1 has six zones,
ap-northeast-1 has no `b`); CHANGELOG says the a-c set is not AWS's.

**Loaded** database: answers the AZ table's rows for (account, region), read through
fakeaws-ami-fixtures-suppressed-when-loaded's loaded reader, never a second flag. No rows means an
empty set marked empty-collection (falling back to the fixed set would invent zone ids for a real
account; classing it answered-from-rows would hide it). A failed loaded-flag read answers 500,
never the fixed set.

**Zone ids agree on both paths.** One helper gives a zone's id, used by this arm and by the subnet
XML (ec2.go:634). Unloaded it keeps today's `<zone>-az1`. Loaded it returns the AZ row's stored
zone_id for that zone name; with no row for the zone, the subnet's availabilityZoneId is empty,
never the fabricated id. The helper reads the region's rows once per Describe.

**Table.** New `availability_zones` in repository/ec2.go, with zone_name, zone_id, state,
opt_in_status, zone_type, group_name, network_border_group. Registered in the migrations and
prependResetTables but not in emptyExclusions (repository/identity.go:19-34), because a row is
account data. Its insert is fakeaws-loader's API; here only tests call it.

**Params.** Filters through the evaluator: zone-name, zone-id, state, region-name, opt-in-status,
zone-type, group-name, network-border-group. ZoneName.N and ZoneId.N are honoured; an unknown one
answers 400 InvalidParameterValue. AllAvailabilityZones is honoured: absent or false leaves out
rows whose opt-in-status is not-opted-in, as AWS does; true includes them.

Docstring: `CRITICAL[ec2-az-loaded-never-fixed-set]`. **The unknown-zone code is unverified:**
InvalidParameterValue for an unknown ZoneName.N/ZoneId.N comes from the EC2 API reference, not a
recording; the handler and its test carry `UNVERIFIED(aws-code): InvalidParameterValue from <doc URL>; no recording`,
and fakeaws-data-sources-pin records it in the deltas file.

**Done when:**
- Unloaded: us-east-1 answers us-east-1a, b and c with the fields above, and eu-west-2 answers
  eu-west-2a-c. The zoneId of us-east-1a equals the availabilityZoneId DescribeSubnets returns for a
  subnet there.
- Filter state=available (data.aws_availability_zones' default), zone-name and zone-id each select
  zones. ZoneName.1=us-east-1z answers 400 InvalidParameterValue naming it, with the UNVERIFIED marker.
- TestContract_ec2_az_loaded_never_fixed_set: a database marked loaded with two stored us-east-1
  rows answers exactly those two in us-east-1, with their stored zone ids, and a subnet in one of
  those zones reports that stored zone id as its availabilityZoneId. eu-west-2 answers an empty set
  classed empty-collection. With the identity table's loaded column dropped through
  Repository().DB() (`ALTER TABLE identity DROP COLUMN loaded`), so AccountID still reads and the
  request reaches the arm, the call answers 500 and returns no fixed zone.
- A loaded row with opt-in-status not-opted-in is absent without AllAvailabilityZones and present
  with AllAvailabilityZones=true.
- Reset empties the table; restore and reopen keep its rows. SetIdentity refuses an account change
  on a database whose only row is an AZ row.
- DescribeAvailabilityZones is added to TestContract_ec2_describe_filter_never_ignored's list, and
  that test fails until it is. `git grep -n loaded repository/` shows only the identity column and
  fakeaws-ami-fixtures-suppressed-when-loaded's reader and setter.
- AGENTS.md lists the id and the contract audits pass. A regression test is in
  handlers/regression_test.go, and CHANGELOG says the a-c set is not AWS's per-region set.
- Each new test is shown failing by a cp-backup break, recorded in the PR: the loaded branch falls
  back to the fixed set; the reader's error is swallowed and the fixed set returned; the subnet XML
  keeps `-az1` on a loaded database; AllAvailabilityZones ignored.
