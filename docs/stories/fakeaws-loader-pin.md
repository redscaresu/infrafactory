---
kind: chore
repo: infrafactory
status: blocked
blocked_by: [fakeaws-load-step0-fixture, prod-read-seam-reader-describe-volumes]
epic: fakeaws-loader
depends_on: [fakeaws-load-step0-fixture, prod-read-seam-reader-describe-volumes]
touches: [.github/workflows/ci.yml, internal/cli/rehearse_fakeaws_bin.go, internal/e2e/aws_reader_ec2_fakeaws_test.go, internal/e2e/aws_reader_step0_fakeaws_test.go, docs/operations.md, docs/layer3/real-vs-mock-deltas.md, docs/stories/fakeaws-known-red-examples.md]
---

# Pin FAKEAWS_SHA to the fakeaws main holding the loader, change only the e2e expectations the bump breaks, and record what stays open in deltas.md

Bump FAKEAWS_SHA (.github/workflows/ci.yml:229) to a fakeaws main holding every fakeaws story of
this epic.
If rehearsalFakeawsPin exists, move it with FAKEAWS_SHA.

**E2e: only what the bump breaks.** CI runs internal/e2e at the PR's FAKEAWS_SHA, so an
expectation the bump breaks cannot land apart from the bump without a red merge; the epic's Areas
name these files for that reason. Expected: the pin-refused rows (split per test by
prod-read-seam-reader-describe-volumes) become empty in TestAWSReadEC2AgainstFakeaws
(`DescribeSecurityGroupRules group-id`, `DescribeVolumes attachment.instance-id`) and in
TestAWSReadStep0AgainstFakeaws (`DescribeSecurityGroupRules group-id`). Any other expectation the
bump breaks is changed here, listed in the PR with the line it fixes. No new test and no new
assertion: those are fakeaws-loader-e2e's.

**Docs.**
- docs/operations.md `### The fakeaws instance` (:567-596) gains /mock/load, -load-max-bytes and
  what a loaded db answers; its empty-collection row (:592) names DescribeVolumes only for an
  unloaded db.
- docs/layer3/real-vs-mock-deltas.md D7 (:218) becomes closed for a loaded instance and open for
  an applied one (RunInstances makes no root volume), and adds that CreateVpc makes no main route
  table. The open gaps the loader leaves are recorded there or in a new section, each with its
  owner:
  - The copy holds the AMI a public parameter names only when the read holds its DescribeImages
    (prod-read-seam-reader-parameter-image). Any other AMI an instance launched from is answered
    under unresolved, and RunInstances with it on the copy answers InvalidAMIID.NotFound.
  - DescribeInstanceCreditSpecifications (aws_instance's read, burstable types, v5.100.0
    ec2_instance.go:1459-1462) is not read. Owner: larger-estate.
  - A volume's snapshotId and KMS key load as unresolved. Owner: larger-estate (snapshots).
- docs/stories/fakeaws-known-red-examples.md drops update_security_group_rules.

**Done when:**
- `git -C ../fakeaws merge-base --is-ancestor <sha> <FAKEAWS_SHA>` exits 0 for every fakeaws merge
  of this epic.
- CI 'AWS against fakeaws' is green with .github/fakeaws-e2e-tests unchanged, and each test name
  prints its PASS line.
- `git diff main -- internal/e2e` touches only expectation lines the bump broke, each listed in the
  PR.
- D7 says fakeaws answers DescribeVolumes empty only for an applied instance, and names the missing
  main route table; the AMI, credit-specification and volume-reference gaps are each recorded with
  an owner. The operations.md empty-collection row names an unloaded db.
- `grep -n update_security_group_rules docs/stories/fakeaws-known-red-examples.md` prints nothing,
  and `make doc-hygiene` passes.
