---
kind: code
status: ready
epic: prod-read-seam
depends_on: [prod-read-seam-identity-preflight, prod-read-seam-reader-policy]
touches: [internal/harness/aws_reader.go, internal/harness/aws_reader_test.go, internal/harness/aws_reader_table.go, internal/e2e/aws_reader_fakeaws_test.go]
risk: high
---

# The read: validate the read set, prove the identity, run the preflight, replay each entry verbatim, write the recorded read

The planned read-core was too big for one PR, so it is split in two: this story (orchestration and replay), then prod-read-seam-read-paging (paging and throttling).

New `aws_reader.go` defines `ReadAWSAccount(ctx, input)`, which returns an ADR-0041 recorded read. Order: validate the read set (no request is sent if that fails), check the identity, run the preflight, then replay each entry.

**Replay.** Each entry becomes a raw HTTP request, signed with aws/signer/v4 (already in go.mod), sent through its service's `*AWSReadWall`. An explicit map gives each service its endpoint and signing region; IAM is global: `iam.amazonaws.com`, signed for `us-east-1`. No SDK client and no retryer sit on the replay path, so IAM replay needs no service/iam. Query params go into the form body verbatim; json params are the body verbatim.

**Answers.** A NotFound-class code named for that service in the reader table (InvalidVpcID.NotFound, InvalidGroup.NotFound, InvalidKeyPair.NotFound, ParameterNotFound, NoSuchEntity) is recorded with its status, so a lookup that finds nothing is still an answer. Any other non-2xx fails, naming the entry. Until read-paging lands, any truncation marker (EC2 nextToken, SSM NextToken, IAM IsTruncated=true) fails the read, naming the entry, so nothing is cut short silently.

**Input.** The identity check and the preflight are input fields; a nil one refuses before any request. calls is the sum of every wall's count, identity and preflight included. Duration and timestamp come from an injected clock.

**Done when:**
- Recording-doer tests: replayed EC2 Filter.1.Name, Filter.1.Value.1, GroupName.1 and Owner.1 reach the wire form verbatim; an SSM GetParameter entry's params equal the JSON body on the wire. Each request carries a SigV4 Authorization header for its service and signing region; an IAM entry goes to iam.amazonaws.com signed for us-east-1.
- ReadAWSAccount with a nil identity check, or a nil preflight, refuses with nothing on the wire.
- An out-of-table entry leaves the wire empty; a wrong account leaves only the GetCallerIdentity request; a failing preflight leaves no read-set request on the wire.
- A NotFound-class answer is recorded with status 400; an AccessDenied fails the read, naming the entry; a response carrying a truncation marker fails, naming the entry.
- In a mixed read over STS, EC2, SSM and IAM recording doers, calls equals their summed request count, identity and preflight included.
- With a fake clock advanced between start and end, timestamp equals the start time and duration_ms is non-zero and equals the advance.
- go.mod is not in the diff.
- internal/e2e: against fakeaws at FAKEAWS_SHA, through the wall over NewLoopbackOnlyHTTPClient (aws_client.go:74), GetCallerIdentity, an SSM GetParameter and an unfiltered DescribeVpcs record the expected exchanges. fakeaws does not model DryRun, so the e2e passes a non-nil stub preflight.
- The PR records a cp-backup break that turns each new test red.
