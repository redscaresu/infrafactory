---
kind: code
repo: fakeaws
status: ready
epic: fakeaws-data-sources
depends_on: [fakeaws-describe-availability-zones]
touches: [handlers/ec2.go, repository/ec2.go, repository/ec2_test.go, handlers/ec2_test.go, handlers/regression_test.go, examples/misconfigured/subnet_cidr_overlap/main.tf, examples/misconfigured/subnet_cidr_overlap/expected.txt, examples/README.md, AGENTS.md, CHANGELOG.md]
---

# CreateSubnet refuses a CIDR overlapping a sibling subnet with InvalidSubnet.Conflict (D8)

Siblings are the subnets of the same VPC; overlap is netip.Prefix.Overlaps.

repository CreateSubnet (repository/ec2.go:327-340) runs GetVPC then a bare r.db.Exec; there is no
transaction today. Wrap it in a new `r.db.Begin` tx (as repository/ec2.go:881 and
ec2_compute.go:191 do). The VPC lookup, a SELECT of the VPC's sibling cidr_blocks and the INSERT all
go through tx, never r.db: calling r.GetVPC or r.ListSubnets while the tx is open deadlocks under
SetMaxOpenConns(1) (repository/repository.go:104-107; ec2_compute.go:190). An overlap returns a new
sentinel, `repository.ErrSubnetConflict`, never models.ErrConflict (which WriteAWSError writes as
409 ConflictException). The handler maps it to 400 InvalidSubnet.Conflict,
`The CIDR '<cidr>' conflicts with another subnet`. Delete the comment 'Overlap with sibling subnets
is not checked' (ec2.go:469).

Thirteen examples create subnets; each uses distinct /24s or one subnet per VPC, as do the handler
and repository tests, so none should break. Any test whose CIDR changes is named in the PR.

Docstring: `CRITICAL[ec2-subnet-overlap-refused]`. **The code and message are unverified:** D8's
recorded refusal was InvalidSubnet.Range; InvalidSubnet.Conflict and its message come from the EC2
API reference. The docstring and the test carry `UNVERIFIED(aws-code): InvalidSubnet.Conflict from <doc URL>; no recording`,
and fakeaws-data-sources-pin records it in the deltas file.

fakeaws-data-source-example follows this story rather than running beside it: both edit
examples/README.md.

**Done when:**
- Creating 10.0.1.0/24 and then 10.0.1.128/25 in one VPC: the second create answers 400
  InvalidSubnet.Conflict naming 10.0.1.128/25 and stores nothing. The same /25 in another VPC
  succeeds, and so does 10.0.2.0/24. After DeleteSubnet, the /24 can be created again.
- In repository/ec2_test.go, 8 goroutines x 50 rounds, each round on a fresh VPC, create one CIDR:
  each round has exactly one success and seven ErrSubnetConflict.
- examples/misconfigured/subnet_cidr_overlap, with expected.txt InvalidSubnet.Conflict, passes
  provider-smoke, and examples/README.md lists it.
- TestContract_ec2_subnet_overlap_refused passes with the UNVERIFIED marker, AGENTS.md lists it,
  and the contract audits pass. Existing tests pass. CHANGELOG has an entry.
- Each new test is shown failing by a cp-backup break, recorded in the PR: the check removed; the
  check moved outside the tx through r.db (the concurrency test turns red); the sentinel replaced by
  models.ErrConflict (the handler test turns red on 409).
