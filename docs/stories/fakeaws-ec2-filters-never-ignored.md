---
kind: code
repo: fakeaws
status: ready
epic: fakeaws-data-sources
depends_on: [fakeaws-ec2-filter-engine, fakeaws-request-log-gap-markers, fakeaws-ami-fixtures-suppressed-when-loaded, fakeaws-state-gathers-fail-closed, fakeaws-request-log-read-set, fakeaws-identity-sweep-and-docs]
touches: [handlers/ec2.go, handlers/ec2_filters.go, handlers/ec2_filters_test.go, handlers/ec2_test.go, handlers/coverage_boost_test.go, handlers/recorder_test.go, coverage_matrix.yaml, AGENTS.md, concepts.md, CHANGELOG.md]
risk: high
---

# No EC2 Describe ignores a param: every Describe goes through the evaluator, with one refusal site at 501 marked filter-refused

Wire fakeaws-ec2-filter-engine's evaluator into every Describe arm of handleEC2
(handlers/ec2.go:54-188). Each arm declares the params it reads; every other param except Action,
Version, MaxResults and NextToken goes to the one writer, named. Filter.N keeps only the names
honoured today: RouteTables' 3 (:878-894), NetworkInterfaces' 4 (:2354-2359), Addresses'
association-id (:1177-1185), Tags' 4 (:3157-3162) and Subnets' vpc-id, which now reads every
Value.M (today only Value.1, :494-502). Each of those arms matches through the evaluator; no inline
matcher survives.

Vpcs (:282-325), InternetGateways (:701), Instances, KeyPairs (:2635) and Images ignore Filter.N
silently today. After this story they, InstanceTypes, SecurityGroups and the four none-sets
(:2770-2776) refuse every Filter.N until the resource-group stories add names. Params still refused
that are not Filter.N go through the same writer: GroupName.N, PublicIp.N, DescribeImages
ExecutableBy.N, DescribeKeyPairs KeyPairId.N, AssociateAddress AllowReassociation (:1268-1270).
DescribeSnapshots OwnerId.N and DescribeImages Owner.N and IncludeDisabled stay read (the sweep
sends them). A scalar the provider sends that an arm cannot honour is either honoured here or
accepted with the reason in a comment (it cannot change the answer); provider-smoke finds them, and
none goes to a known-red entry. refuseUnmodelled (:2781-2792), the inline allowlist loops and
ec2Filters are deleted.

The writer calls `markClass(w, classFilterRefused)` (recorder.go:139) and is the only EC2 caller.
This lands after fakeaws-request-log-gap-markers, so this story updates
TestContract_request_log_filter_refused: EC2 sites move to 501, the marker stays, SSM sites are
unchanged. The empty-collection marker in ec2DescribeNone stays after the filter check. No flag: the
default wire changes on purpose (409 to 501; ignored params refused or honoured), and CHANGELOG lists
each change. infrafactory sends no SDK Filters (internal/harness/aws_sweep.go:220-462), and the SDK
v2 retryer does not retry 501.

Contracts, each with its TestContract_ over the wire and listed in AGENTS.md:155:
`MUST[ec2-filter-and-across-or-within]`, `MUST[ec2-filter-wildcard-crosses-slash]` and
`MUST[ec2-filter-refusal-names-every-name-sorted]` on ec2_filters.go (the last pins the message
phrase `not modelled by fakeaws`), and `MUST[ec2-describe-filter-never-ignored]` on handleEC2.

**Done when:**
- TestContract_ec2_describe_filter_never_ignored holds a list, written in the test, of the 19
  Describe names in ec2.go:59-181 at fakeaws 1993583 (all four from the multi-name arm at :149),
  16 after its exemptions DescribeVpcAttribute, DescribeInstanceAttribute and
  DescribeAddressesAttribute, which take no Filter in the AWS API. It scans every `case "Describe…"` arm, every name on the line, and fails if
  the scan finds a name not in the list or misses one that is, and fails if an exemption no longer
  appears in the source. It sends each listed arm `Filter.1.Name=fakeaws-no-such-filter&Filter.1.Value.1=x`,
  then `FakeawsNoSuchParam.1=x`; each answers 501 NotImplemented whose message names the probe, and
  under RecordRequests it is classed filter-refused, not unimplemented-501.
- Through a wired handler: DescribeRouteTables with Filter.1 vpc-id=A and Filter.2 vpc-id=B returns
  nothing; one Filter.N holding Value.1=A and Value.2=B returns both. DescribeSubnets with vpc-id
  Value.1 matching nothing and Value.2 matching returns the subnet.
- The three engine MUST tests and TestContract_ec2_describe_filter_never_ignored pass, AGENTS.md
  lists all four ids, and the contract audits pass.
- The tests that pin 409 change to 501, named in the PR: assertRefused (ec2_test.go:1616-1623),
  ec2_test.go:1069-1072, :1516 (DescribeTags tag:Name), :1836 (DescribeRouteTables
  route.gateway-id) and coverage_boost_test.go:349-351. The other RouteTables, NetworkInterfaces,
  Addresses and Tags tests pass unchanged.
- TestContract_request_log_filter_refused asserts 501 and filter-refused at every EC2 site, and
  still 501 at the SSM sites. `git grep -n classFilterRefused handlers/ec2.go` prints only the writer.
- `git grep -n -e 'func ec2Filters' -e 'func refuseUnmodelled' -e 'not supported by fakeaws' handlers/`
  prints nothing.
- The sweep's reads still answer: DescribeImages Owner.1=self with IncludeDisabled lists no fixture
  (TestRegressionSweepDescribeImagesOwnerSelf unchanged), and DescribeSnapshots OwnerId.1=self
  answers 200 empty. DescribeImages ExecutableBy.1=all answers 501 naming `ExecutableBy.N`.
- The five 'refused 409' reasons in coverage_matrix.yaml (:320-356) say 501.
  `git grep -n 409 AGENTS.md concepts.md coverage_matrix.yaml` shows no filter-refusal line.
  CHANGELOG [Unreleased] names the 409→501 change and each Describe and param that stopped being
  ignored.
- provider-smoke passes with examples/known_red_test.go unchanged.
- Each new test is shown failing by a cp-backup break, recorded in the PR: the marker call removed;
  one Describe unwired; the status put back to 409; a merged-OR matcher restored in
  DescribeRouteTables; the scan regex reading only the first name on a case line.
