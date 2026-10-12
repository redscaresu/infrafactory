---
kind: code
repo: fakeaws
status: blocked
blocked_by: [fakeaws-state-gathers-fail-closed]
depends_on: [fakeaws-ami-fixtures-suppressed-when-loaded, fakeaws-state-gathers-fail-closed]
touches: [handlers/ec2.go, handlers/ec2_test.go, repository/ec2_compute.go, AGENTS.md, CHANGELOG.md]
risk: high
---

# DescribeInstanceAttribute answers from rows, and an unknown instance is NotFound

Found in fakeaws#57's review. DescribeInstanceAttribute returns hard-coded values:
`sourceDestCheck` answers `true` even after ModifyInstanceAttribute stored `false` on the ENI, and
disableApiTermination, disableApiStop, ebsOptimized and instanceInitiatedShutdownBehavior are
constants. An unknown InstanceId answers 200 instead of InvalidInstanceID.NotFound. The rehearsal
reads instance attributes (prod-read-seam's EC2 follow-ups, then fakeaws-loader), so a constant
here is drift the copy cannot show.

Store each attribute the provider can set (RunInstances or ModifyInstanceAttribute) on the instance
or its primary ENI, answer DescribeInstanceAttribute from that row, and refuse an attribute fakeaws
does not model with the filter-refused / not-modelled marker the request log uses (never a silent
default).

**Done when:**
- ModifyInstanceAttribute SourceDestCheck=false, then DescribeInstanceAttribute sourceDestCheck,
  answers false; the same round trip holds for disableApiTermination, disableApiStop and
  instanceInitiatedShutdownBehavior.
- DescribeInstanceAttribute on an unknown InstanceId answers 400 InvalidInstanceID.NotFound.
- An attribute fakeaws does not store is refused with the request log's not-modelled class, not
  a constant.
- RunInstances defaults stay byte-identical when nothing is set (existing tests unchanged).
- A TestContract_ id, CRITICAL/MUST docstring and AGENTS.md "Current contracts" entry.
- Each new test is shown failing by a cp-backup break recorded in the PR.
