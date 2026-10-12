---
kind: code
repo: fakeaws
status: blocked
blocked_by: [fakeaws-filters-vpc-network, fakeaws-ami-fixtures-suppressed-when-loaded]
epic: fakeaws-data-sources
depends_on: [fakeaws-filters-vpc-network, fakeaws-ami-fixtures-suppressed-when-loaded]
touches: [handlers/ec2_filters.go, handlers/ec2.go, handlers/ec2_test.go, handlers/coverage_boost_test.go, handlers/regression_test.go, AGENTS.md, CHANGELOG.md]
---

# DescribeSecurityGroups honours Filter.N, and models GroupName.N as AWS does where the rows can say so

**Filters.** group-id, group-name, vpc-id, description, owner-id, tags. ip-permission.* and
egress.ip-permission.* stay refused, named.

**GroupName.N.** In AWS, GroupName.N resolves only in the default VPC. Whether a default VPC exists
must come from rows, and fakeaws stores no is-default field (repository/ec2.go:153-160):
- **Unloaded database:** fakeaws never creates a default VPC (CreateSecurityGroup requires VpcId,
  ec2.go:1556-1560), so the rows prove there is none. Every GroupName.N answers 400
  VPCIdNotSpecified, classed answered-from-rows. It never matches a group. This matches the read
  seam's rule 'never GroupName.N' (docs/stories/prod-read-seam-ec2-readers.md:20).
- **Loaded database** (read through fakeaws-ami-fixtures-suppressed-when-loaded's loaded reader,
  never a second flag): a real account usually has a default VPC the rows cannot show, so
  GroupName.N stays on the filter-refused writer (501 NotImplemented, filter-refused), until
  fakeaws-loader stores is_default and extends this.

`MUST[ec2-sg-group-name-default-vpc-only]` goes on the handler. **The code is unverified.**
VPCIdNotSpecified and its message come from the EC2 API reference, not a recording. The docstring
and the test each carry `UNVERIFIED(aws-code): VPCIdNotSpecified from <doc URL>; no recording`, and
fakeaws-data-sources-pin records it in docs/layer3/real-vs-mock-deltas.md, owned by the next real-AWS
recording.

**Done when:**
- With a same-named SG in VPCs A and B, Filter group-name=web plus vpc-id=A returns only A's SG.
- Filter group-name=absent plus tag-key=infrafactory-run-id answers 200 with an empty
  securityGroupInfo (prod-read-seam-fakeaws-filtered-e2e's absent-name lookup). Filter tag-key alone
  returns only the tagged SG.
- TestContract_ec2_sg_group_name_default_vpc_only: on an unloaded database, GroupName.1 of an
  existing SG answers 400 VPCIdNotSpecified and is classed answered-from-rows under RecordRequests;
  on a database marked loaded, GroupName.1 answers 501 NotImplemented naming `GroupName.N` and is
  classed filter-refused. Both carry the UNVERIFIED marker.
- The group-name=default case at coverage_boost_test.go:349-351 expects 200 with an empty
  securityGroupInfo (fakeaws creates no SG named default).
- AGENTS.md lists the id, the contract audits pass, a regression test is in
  handlers/regression_test.go, and CHANGELOG has an entry.
- Each new test is shown failing by a cp-backup break, recorded in the PR: one table entry removed;
  the loaded check dropped (the loaded case turns red).
