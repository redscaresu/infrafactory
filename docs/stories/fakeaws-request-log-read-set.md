---
kind: code
repo: fakeaws
status: blocked
blocked_by: [prod-read-seam-adr-0041]
epic: fakeaws-rehearsal-instance
depends_on: [fakeaws-request-log-gap-markers, prod-read-seam-adr-0041]
touches: [handlers/recorder.go, handlers/recorder_test.go, AGENTS.md, concepts.md, CHANGELOG.md]
risk: high
---

# Request-log entries carry the ADR-0041 read-set fields {service, protocol, action, params}

The blocker is the `prod-read-seam-adr-0041` story ("account-read contract, recorded-read and read-set formats").

- **Body.** The recorder reads the body only for Query-RPC and X-Amz-Target requests, and restores `r.Body` byte-identical: ParseQueryRPC and ParseXAmzTarget drain it (awsproto/queryrpc.go:30-51, json.go:29-48). Never use r.ParseForm/FormValue. REST bodies are not read, so S3 PutObject uploads are never buffered.
- **Action.** Query-RPC (EC2, IAM, STS, RDS) takes Action from the form body. JSON (SSM, DynamoDB, Secrets Manager, KMS, SQS) takes X-Amz-Target. Params are as ADR-0041 defines them.
- **REST entries.** EKS, S3 and Route53 are recorded as ADR-0041 says. Where it is silent, an entry carries the method, the chi route pattern (read after the handler returns; chi v5.3.0 mux.go:63-91, context.go:123) and the raw query string, so S3 subresources sharing `/s3/{bucket}/` (s3.go:31-37, 84-105, 151-175) stay distinct.
- **Where params live.** Params stay in process memory, in GET /mock/requests only (token-gated by fakeaws-request-log). They never reach the db, the snapshot or /mock/state, whose audit stays `{service, protocol, action, status, class}`; this story fills protocol and action there.

**Done when:**
- `TestContract_request_log_read_set_entry`: EC2 DescribeVpcs, IAM GetRole, STS GetCallerIdentity, RDS DescribeDBInstances (form Action), SSM GetParameter and DynamoDB DescribeTable (X-Amz-Target) each record service, protocol, action and params as ADR-0041 defines them, and each asserts the wire status.
- A REST test: S3 GetBucketTagging and GetBucketPolicy on one bucket record two different entries; an EKS DescribeCluster entry carries the route pattern `/eks/region/{region}/clusters/{name}` (eks.go:32-35).
- Body restored, under RecordRequests: EC2 CreateVpc then DescribeVpcs returns the VPC with 200; IAM CreateRole then GetRole returns the role; SSM PutParameter then GetParameter returns the value byte-exact.
- Params stay in memory: after a PutParameter with a unique value, a GetParameter for a unique missing name and POST /mock/snapshot, neither string appears in the db file, `.snapshot` or the GET /mock/state body.
- Each new test is shown failing by a cp-backup break recorded in the PR, including one that drops the body restore (Query-RPC and JSON paths) and one that drops the raw query from REST entries.
- AGENTS.md line 151, concepts.md item 7 and CHANGELOG [Unreleased] are updated.
