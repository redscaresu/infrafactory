---
kind: code
repo: infrafactory
status: blocked
blocked_by: [prod-read-seam-reader-describe-volumes]
epic: prod-read-seam
depends_on: [prod-read-seam-reader-describe-volumes]
touches: [internal/harness/aws_reader.go, internal/harness/aws_reader_ssm.go, internal/harness/aws_reader_ssm_test.go, internal/harness/aws_reader_scrub.go, internal/harness/aws_reader_scrub_test.go, internal/e2e/aws_reader_step0_fakeaws_test.go]
---

# A public AMI parameter the reader reads is followed by the image it names: DescribeImages ImageId=<value>

The step-0 read set reads the al2023 parameter, whose value is an AMI id, but never that image
(internal/e2e/testdata/aws-step0/read-set.json). A loaded fakeaws suppresses its fixture AMIs
(fakeaws handlers/ec2.go:3805, admin.go:216), so on the copy RunInstances with that AMI answers
InvalidAMIID.NotFound (fakeaws handlers/ec2.go:2389). fakeaws-load-ssm-parameters reports the AMI
under unresolved until a read holds it.

- A new follow-up, beside readEC2 and readIAM (aws_reader.go:110), in aws_reader_ssm.go: for each
  replayed ssm GetParameter answered 200 whose name starts `/aws/service/` and whose Value is an
  AMI id (`^ami-[0-9a-f]+$`), one DescribeImages with `ImageId.1=<Value>`. Values are deduped,
  and nothing is sent when there are none. DescribeImages is already in the reader table and the
  policy, so neither changes.
- The image's imageOwnerId is Amazon's account (137112412989), which the committed-fixture check
  refuses as a 12-digit number other than the synthetic id (aws_reader_scrub.go:558-567). The check
  admits a 12-digit number only as a DescribeImages item's imageOwnerId, and only when it is on a
  named list of public image owner ids, each with its AWS documentation cite. The list holds
  Amazon's id alone. Anywhere else, the same number is still refused.

prod-read-seam-real-recording waits on this story, so the step-0 recording holds the image and the
loaded copy can launch from it.

**Done when:**
- A harness test: a GetParameter of `/aws/service/ami-amazon-linux-latest/al2023-ami-kernel-default-x86_64`
  answering `ami-0abc` is followed by exactly one DescribeImages with `ImageId.1=ami-0abc`; a
  private parameter, a public non-AMI value and a ParameterNotFound send none; two parameters with
  the same value send one.
- CheckScrubbedRead passes a recording whose DescribeImages item carries imageOwnerId
  137112412989. It fails a cp-backup copy with 137112412989 planted in a DescribeVpcs body, and a
  copy whose imageOwnerId is 999988887777.
- TestAWSReadStep0AgainstFakeaws records a DescribeImages `ImageId.1=<the parameter's value>`
  exchange answered 200 holding that image id, and CI 'AWS against fakeaws' passes at the current
  FAKEAWS_SHA.
