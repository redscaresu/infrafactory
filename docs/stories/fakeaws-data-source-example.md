---
kind: code
repo: fakeaws
status: ready
epic: fakeaws-data-sources
depends_on: [fakeaws-subnet-overlap-refused, fakeaws-filters-vpc-network, fakeaws-filters-security-groups, fakeaws-images-owner-name-filters, fakeaws-describe-availability-zones]
touches: [examples/working/ec2_data_sources/main.tf, examples/filter_refused_test.go, examples/README.md, README.md, coverage_matrix.yaml, internal/audit/audit_test.go]
---

# A data-source example (VPC by tag, SG by name, AMI by owner and name, AZs) applies under 5.100.0, and a refused filter is proved end to end

**Example.** examples/working/ec2_data_sources, under hashicorp/aws 5.100.0. It creates two VPCs
with different Name tags and a same-named SG in each. It reads data.aws_vpc by tag,
data.aws_security_group by name and vpc_id, data.aws_ami with owners [amazon], name
al2023-ami-2023.*-x86_64 and most_recent, and data.aws_availability_zones with state available,
then creates a subnet in the first AZ name. The reads reference the created resources, so they are
deferred to apply. Postconditions fail the apply unless each read resolves to what it should.

**End to end.** examples/filter_refused_test.go proves the refusal from the real provider through
fakeaws to the request log; this is the epic's 'tested end to end'. The PR records the verbatim
`tofu plan` error text; infrafactory-filter-refusal-routes-to-mock-gaps checks its learner fixture
against it.

**Coverage matrix.** Rows for the data sources, each pointing at this example
(working_dir_name: ec2_data_sources), with integration_test_func_name naming the vpc-network,
security-groups and AZ regression tests, and misconfigured exempt with filter_refused_test.go as the
reason: `data.aws_vpc`, `data.aws_security_group` and `data.aws_availability_zones`. The aws_ami row
(:308-314) gets working_dir_name ec2_data_sources instead of working_exempt. If TestFullCoverageAudit
cannot key a `data.` row (its default test-name and dir derivation, internal/audit/audit_test.go:132),
this story teaches it to, with a test.

README.md's EC2 row (:39) and examples/README.md list the example. It follows
fakeaws-subnet-overlap-refused (both edit examples/README.md) and edits no ec2.go, AGENTS.md or
CHANGELOG.

**Done when:**
- working/ec2_data_sources passes provider-smoke through apply, the idempotency plan and destroy. Its
  postconditions assert: data.aws_vpc's id is the tagged VPC's, the SG id is the one in that VPC, the
  AMI id is the AL2023 fixture's, and there are three AZ names.
- A cp-backup break that removes tag support from the VPC table makes the example fail at apply,
  because two VPCs match; recorded in the PR.
- examples/filter_refused_test.go starts fakeaws with -admin-token and -record-requests. `tofu plan`
  of a data.aws_vpc with filter dhcp-options-id exits 1, and its output names dhcp-options-id,
  NotImplemented and `not modelled by fakeaws`. GET /mock/requests holds that DescribeVpcs with
  status 501 and class filter-refused. A cp-backup break of the marker turns the test red; recorded
  in the PR.
- TestFullCoverageAudit passes with the aws_ami row's working_exempt gone and the three `data.` rows
  present; a cp-backup break deleting the example directory turns it red, recorded in the PR.
