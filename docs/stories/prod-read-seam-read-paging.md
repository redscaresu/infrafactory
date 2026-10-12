---
kind: code
status: ready
epic: prod-read-seam
depends_on: [prod-read-seam-read-core]
touches: [internal/harness/aws_reader.go, internal/harness/aws_reader_paging.go, internal/harness/aws_reader_paging_test.go, internal/harness/aws_reader_table.go, internal/harness/testdata/aws_reader_pages/]
risk: high
---

# The read pages to the end and backs off on throttling, failing closed on both

Split from prod-read-seam-read-core, which fails the read on any truncation marker and any throttle. This story replaces those refusals with paging and bounded retry.

**Paging.** Lists page to the end per protocol: EC2 nextToken, IAM IsTruncated/Marker. Each page is its own exchange. awsPages' seen-token rule applies (aws_sweep.go:184-205): a repeated token fails the read, naming the entry, and so does IsTruncated=true without a Marker (ADR-0040 rule 10). The reader table has no paged SSM action, so a json response with a non-empty NextToken keeps failing the read, naming the entry.

**Throttling.** The throttle codes are named per service in the reader table: EC2 `RequestLimitExceeded`, SSM `ThrottlingException`, IAM and STS `Throttling`. A throttled request backs off on the injected clock and gives up after a bounded number of attempts. Every attempt counts in calls.

**Fixtures.** Paged test bodies are real-shaped for each protocol (EC2 XML with `<nextToken>`, IAM XML with `<IsTruncated>` and `<Marker>`, SSM JSON with `NextToken`), copied from AWS's documented response shape; the PR cites each source. A paged action used in a test is a row in the reader table.

**Done when:**
- A three-page EC2 list (e.g. DescribeInstances) and a two-page IAM list (e.g. ListRolePolicies) record every page as its own exchange.
- A repeated nextToken fails the read, naming the entry; IsTruncated=true without a Marker fails, naming the entry; an SSM response with a NextToken fails, naming the entry.
- A test fails if a paged action used in these tests is not a reader-table row.
- Throttling then OK succeeds with calls=2, one row per service code (RequestLimitExceeded, ThrottlingException, Throttling). An unnamed code (e.g. SlowDown on EC2) is not retried and fails, naming the entry.
- Throttling every time fails after the bound, naming the entry, in under 5s of wall time on the fake clock.
- In every test, calls equals the recording doers' summed request count.
- TestAWSReaderPolicyIsExactlyTheReaderTable passes; the PR records a cp-backup break that turns each new test red.
