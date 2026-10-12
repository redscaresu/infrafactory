---
kind: code
repo: fakeaws
status: ready
epic: fakeaws-data-sources
depends_on: [fakeaws-filters-instances-enis-addresses-keys]
touches: [handlers/ec2_filters.go, handlers/ec2.go, handlers/ec2_test.go, handlers/regression_test.go, CHANGELOG.md]
---

# DescribeImages honours Owner.N and name filters with wildcards; filters on fields AMI rows do not store are refused

Filters: name (wildcards cross `/`), image-id, owner-id, owner-alias, virtualization-type,
root-device-name, state (available), image-type (machine), tags. Owner.N keeps account ids, `self`
(the caller) and `amazon` (ec2.go:2737-2745). AMI rows have no field for architecture, platform,
block-device-mapping.* and the rest (repository/ec2_compute.go:143-150), so those filters stay
refused, named. ExecutableBy.N stays refused through the one writer (fakeaws-ec2-filters-never-ignored
put it there). IncludeDeprecated and IncludeDisabled are accepted with that reason in a comment:
fakeaws has no deprecated or disabled image. Rewrite the comment 'data.aws_ami is NOT supported'
(ec2.go:2711-2712).

Owner amazon matches the fixtures' literal OwnerID `amazon`. On a loaded database the fixtures are
suppressed, and a loaded Amazon image has a numeric owner, so Owner.1=amazon matches nothing there:
an honest empty answer, not an invented one. Matching it is fakeaws-loader's AMI widening.

**Done when:**
- Owner.1=amazon plus name=al2023-ami-2023.*-x86_64 returns exactly the AL2023 fixture
  (AL2023AMIID). name=ubuntu/images/* with Owner.1=099720109477 returns both Ubuntu fixtures.
- TestRegressionSweepDescribeImagesOwnerSelf passes unchanged; Owner.1=self plus a name filter
  returns no image.
- Filter architecture=x86_64 and ExecutableBy.1=all each answer 501 naming it, classed filter-refused.
- `grep -n 'data.aws_ami is NOT supported' handlers/ec2.go` prints nothing. A regression test is in
  handlers/regression_test.go, and CHANGELOG has an entry.
- Each new test is shown failing by a cp-backup break (the name entry removed; a `*` that stops at
  `/`), recorded in the PR.
