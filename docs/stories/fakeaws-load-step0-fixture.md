---
kind: code
repo: fakeaws
status: blocked
blocked_by: [fakeaws-loader-real-read-back, prod-read-seam-real-recording]
epic: fakeaws-loader
depends_on: [fakeaws-loader-docs, fakeaws-loader-real-read-back, prod-read-seam-real-recording]
touches: [testdata/load/step0-scrubbed.json, testdata/load/step0-scrubbed.json.source, handlers/load_fixture_leak_test.go]
risk: high
---

# The scrubbed step-0 recording is a fakeaws fixture that loads with no refusal and reads back equal

The real-fixture story, last of the fakeaws stories. Risk is high: a public repo gains data read
from a real account.

- Copy the scrubbed recording prod-read-seam-real-recording committed (infrafactory
  internal/e2e/testdata/aws-step0/) to testdata/load/step0-scrubbed.json, so TestLoadReadBack runs
  on it in CI. Its `.source` file names the infrafactory commit.
- A leak test fails if any 12-digit run in testdata/ is neither the scrub account
  (AWSScrubAccount, 111122223333, infrafactory internal/harness/aws_reader_scrub.go:26) nor on a
  named list of public owner ids, each with its AWS documentation cite (Amazon's image owner,
  137112412989, which the al2023 DescribeImages carries).
- If a scrub placeholder fails a loader check, file an infrafactory scrub story; never loosen the
  loader.

**Done when:**
- Current-EC2 Image members the model lacks (freeTierEligible, deregistrationProtection, sourceInstanceId, imageAllowed, an ebs kmsKeyId) are each modelled or ignore-listed with a v5.100.0 provider cite before the real recording loads; today any of them refuses the load by name (from fakeaws#90 review).
- Current-EC2 Volume and attachment members the model lacks (volume: availabilityZoneId, volumeArn, ownerId, sseType, fastRestored, sourceVolumeId, operator, volumeInitializationRate; attachment: ebsCardIndex, associatedResource, instanceOwningService) are each modelled or ignore-listed with a v5.100.0 provider cite before the real recording loads; today any of them refuses the load by name (from fakeaws#91 review).
- TestLoadReadBack passes on step0-scrubbed.json with zero refusals; the PR lists the per-type
  counts.
- The file's sha256 equals that of infrafactory's committed file at the commit the `.source` file
  and the PR name.
- A cp-backup copy of the fixture with 999988887777 planted fails the leak test.
