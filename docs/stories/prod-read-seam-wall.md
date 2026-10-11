---
kind: code
status: ready
epic: prod-read-seam
depends_on: [prod-read-seam-adr-0041]
touches: [internal/harness/aws_reader_table.go, internal/harness/aws_reader_wall.go, internal/harness/aws_reader_wall_test.go, internal/harness/aws_reader_clients.go]
risk: high
---

# The reader table and the wall: one Go table, and a doer that refuses every request outside it before it leaves

**Reader table.** New `internal/harness/aws_reader_table.go` holds one Go var of `{service, protocol, action}` for the epic's EC2, IAM, SSM and STS reads, with each service's NotFound codes. It is separate from AWSSweepCollections (aws_sweep.go:82), so it never adds a sweep row.

**DryRun table.** A second var in the same file, derived in code from AWSReapSteps' non-Describe EC2Actions (aws_reap.go:68-78; HLD:390-391), never hand-listed and never part of the reader table. The policy is built from the reader table only.

**Wall.** New `aws_reader_wall.go` defines `*AWSReadWall`: a `Do(*http.Request)` around an inner doer, bound to one service. It reads each request into an ADR-0041 entry (Action from the form body for query, X-Amz-Target for json), restores the body byte-identical, and counts every request it passes on. It refuses: more than one Action value; more than one DryRun value; a DryRun other than exactly `true`; X-Amz-Target on a query service; Action on a json service; Action in the URL query. A DryRun-table action passes only with that single literal `DryRun=true`.

**Walled clients.** `aws_reader_clients.go` holds the only walled SDK constructors: EC2 (for the preflight) and STS (for the identity check). Each takes an aws.Config from the caller and a `*AWSReadWall`, refuses a nil wall (unlike NewAWSSTSClient, aws_client.go:41-44), sets `o.HTTPClient` to the wall and sets `aws.NopRetryer{}`, so the SDK never retries and read-core owns retry. Every other read goes through read-core's raw replay, so no SSM or IAM SDK client is built.

Also: Go types for the entry and the recorded read, and `ValidateAWSReadSet`.

**Done when:**
- Table-driven wall test over a recording inner doer. Refused with zero inner calls: EC2 CreateVpc; IAM CreateAccessKey (form body); STS AssumeRole; SSM PutParameter (X-Amz-Target); neither Action nor X-Amz-Target; Action in the URL query; `Action=DescribeVpcs&Action=CreateVpc`; DeleteVpc without DryRun, with `DryRun=false`, with `DryRun=True`, with `DryRun=1`, and with `DryRun=true&DryRun=false`; a query request that also carries X-Amz-Target; a json request with an Action field.
- Passed with one inner call each and the body byte-identical: DescribeVpcs, GetRole, GetCallerIdentity, GetParameter, and DeleteVpc with `DryRun=true`. Two passed requests and one refused give a call count of 2.
- No DryRun-table action is a reader-table row (asserted in the test).
- ValidateAWSReadSet, given a rest entry, EC2 CreateVpc and an unknown service, returns one error naming all three.
- Every ADR-0041 fenced JSON example parses into the Go types with DisallowUnknownFields. The test fails unless it found at least one read-set example and one recorded-read example.
- A behavioural test per walled constructor (EC2, STS) issues one call and asserts the wall's inner recording doer saw it and the count rose by one. A throttled answer gives exactly one inner call (no SDK retry). Each constructor refuses a nil wall.
- An AST test fails when a non-test file under internal/harness/aws_reader*.go or internal/cli/rehearse_read*.go references newAWSEC2Client, newAWSSSMClient, NewAWSSTSClient, VerifyAWSIdentity or AWSDoers, or calls any `*.NewFromConfig`/`*.New` outside aws_reader_clients.go; and when an options func in that file does not assign `o.HTTPClient` to its wall parameter.
- TestADR0040NamesEverySweepRowAndPrecedencePair, the Layer 3 policy tests and TestNoProductionCodeUsesTheAWSDefaultChain pass unchanged.
- The PR records a cp-backup break that turns each new test red.
