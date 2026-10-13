---
kind: code
repo: fakeaws
status: ready
blocked_by: [fakeaws-load-volumes-d7]
epic: fakeaws-loader
depends_on: [fakeaws-load-volumes-d7]
touches: [handlers/load_read_back_test.go, testdata/load/step0-synthetic.json]
risk: high
---

# A read-back test loads each recorded read, re-sends every exchange and finds no difference, reporting paths only

handlers/load_read_back_test.go, TestLoadReadBack. For each recorded read in
testdata/load/*.json, or only the file FAKEAWS_LOAD_READ names, it starts the app in process
(`:memory:`, an admin token, -load-max-bytes) and POSTs /mock/load. It then re-sends each
exchange's request as the reader sent it (query: form params plus Action and Version; JSON 1.1:
X-Amz-Target and the body), and diffs each answer against the recorded body with load_diff.go,
in both directions.

- A NotFound exchange must answer the same code.
- A paged entry compares the union of its pages (MergePages) to the answer.
- The report names the read, the exchange index, the action and the path, never a body or a value:
  the file may be a real account's raw read (fakeaws-loader-real-read-back).

testdata/load/step0-synthetic.json has the shape of the step-0 read: the read set (VPC by tag, AZs,
SGs by tag, the absent-name SG lookup, the al2023 parameter), the reader's VPC follow-ups
(infrafactory internal/harness/aws_reader_ec2.go:111-189) with DescribeVolumes, and the
DescribeImages for the parameter's value (prod-read-seam-reader-parameter-image). It is built from
the testdata/ec2 and testdata/ssm fixtures, and its `.source` file names them.

**Done when:**
- step0-synthetic.json loads with no refusal, and every exchange reads back equal.
- A cp-backup break that renders subnet mapPublicIpOnLaunch as false fails it, naming
  DescribeSubnets and the path. A break that renders an extra element fails it too, naming that
  path.
- A copy of the fixture with SECRET-MARKER-123 planted in a differing value fails, and the report
  string asserted on does not contain SECRET-MARKER-123.
- FAKEAWS_LOAD_READ set to a missing or unreadable path fails; it is never a skip.
