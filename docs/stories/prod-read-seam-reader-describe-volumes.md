---
kind: code
repo: infrafactory
status: ready
epic: prod-read-seam
depends_on: []
touches: [internal/harness/aws_reader_table.go, internal/harness/aws_reader_ec2.go, internal/harness/aws_reader_ec2_test.go, internal/harness/aws_reader_scrub_test.go, docs/layer3/aws/reader-policy.json, internal/e2e/aws_reader_ec2_fakeaws_test.go, internal/e2e/aws_reader_step0_fakeaws_test.go, docs/operations.md]
risk: high
---

# The reader reads each instance's volumes: DescribeVolumes joins the reader table, the policy and the VPC follow-ups

aws_instance's read calls DescribeVolumes by the instance's volume ids (terraform-provider-aws
v5.100.0 internal/service/ec2/ec2_instance.go:2364-2392, readBlockDevicesFromInstance), so a
loaded instance cannot refresh without its volumes. Nothing the reader reads holds them: the
reader table, reader-policy.json and the follow-ups have no DescribeVolumes
(internal/harness/aws_reader_table.go:45-55, docs/layer3/aws/reader-policy.json:14-27,
internal/harness/aws_reader_ec2.go:111-188). fakeaws-loader's D7 root volume needs a recorded
source.

- AWSReaderTable gains DescribeVolumes, with NotFound code InvalidVolume.NotFound, and
  reader-policy.json gains `ec2:DescribeVolumes`. TestAWSReaderPolicyIsExactlyTheReaderTable ties
  the two.
- readEC2VPC gains one DescribeVolumes filtered by `attachment.instance-id` to the VPC's
  instances, sent only when there are any.
- At the current FAKEAWS_SHA, fakeaws refuses that filter (a filtered DescribeVolumes goes to
  ec2DescribeNone, 501). So TestAWSReadEC2AgainstFakeaws, which runs an instance, expects
  `DescribeVolumes attachment.instance-id` refused. TestAWSReadStep0AgainstFakeaws seeds no
  instance and keeps its expectation, so the shared fakeawsPinRefusedRows
  (internal/e2e/aws_reader_ec2_fakeaws_test.go:40) splits into one list per test.
  fakeaws-loader-pin empties both.
- docs/operations.md § The reader policy lists DescribeVolumes, and says a policy attached before
  this merge must be written again (the re-attach block, :507).

This story merges before the user creates the reader user: prod-read-seam-reader-user depends on
it, so the user attaches the policy once, already holding ec2:DescribeVolumes.
prod-read-seam-real-recording and fakeaws-loader-real-read-back depend on it too.

Known gap, same class, not fixed here: for burstable types aws_instance's read also calls
DescribeInstanceCreditSpecifications (v5.100.0 ec2_instance.go:1459-1462), which the reader never
reads. fakeaws-loader-pin records it in deltas.md.

**Done when:**
- TestAWSReaderPolicyIsExactlyTheReaderTable passes with `ec2:DescribeVolumes`, and fails on a
  copy of the policy without it.
- A harness test: a VPC with two instances reads one DescribeVolumes with
  `attachment.instance-id` Value.1 and Value.2, and a VPC with none sends no DescribeVolumes.
- The scrub test, given a DescribeVolumes exchange holding a KMS key ARN in the real account,
  writes no account id. This is a regression pin (scrubAWSRead already replaces the account
  everywhere, aws_reader_scrub.go:153-155), not proof of the new read.
- CI 'AWS against fakeaws' passes at the current FAKEAWS_SHA: TestAWSReadEC2AgainstFakeaws names
  `DescribeVolumes attachment.instance-id` as refused, and TestAWSReadStep0AgainstFakeaws's
  expectation is unchanged.
- docs/operations.md § The reader policy lists DescribeVolumes and says a policy attached before
  this merge must be written again.
- prod-read-seam-reader-user's depends_on names this story (the board PR that scoped it set it).
