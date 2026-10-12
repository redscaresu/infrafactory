---
kind: code
status: ready
epic: prod-read-seam
depends_on: [prod-read-seam-wall, prod-read-seam-credentials]
touches: [internal/harness/aws_reader_identity.go, internal/harness/aws_reader_identity_test.go, internal/harness/aws_reader_preflight.go, internal/harness/aws_reader_preflight_test.go]
risk: high
---

# Identity first, with the principal optional; then the EC2 DryRun preflight, which refuses a key that can write

**Identity.** New `aws_reader_identity.go` defines VerifyAWSReadIdentity, run through the walled STS client. The account is mandatory; an empty principal skips only the ARN comparison. VerifyAWSIdentity (aws_identity.go:17-37) goes on refusing an empty principal, and its callers (aws_preflight.go:36-52, aws_reap.go:25) are unchanged. The ARN match is exact, so an assumed-role key needs its assumed-role ARN as --principal, and the mismatch error says so.

**Preflight.** New `aws_reader_preflight.go` sends every DryRun-table row once, with `DryRun=true`, through the walled EC2 client. Each row has a placeholder builder whose input carries the action's identifying parameter with a syntactically valid placeholder id: AllocationId or AssociationId, LaunchTemplateId, KeyPairId, GroupId plus one IpPermissions entry, and so on. A row with no builder refuses the preflight, naming the row; it is never skipped. An answer passes only when it is a smithy.APIError whose ErrorCode() is exactly `UnauthorizedOperation`. A transport error, a non-API error or any other code refuses.

**PR statement.** "IAM and SSM writes are stopped in-process by the wall at runtime. reader-policy.json, tied to the table by a test, is the policy the user attaches. DryRun is EC2-only."

**Done when:**
- Identity table over a recording doer: a wrong account, a wrong principal and an empty account are refused; an empty principal with the right account passes, and so do the right account and principal. Each refusal leaves exactly one request on the wire, except the empty account, which leaves none.
- The principal-mismatch error mentions the assumed-role ARN form (`arn:aws:sts::<account>:assumed-role/...`) and --principal.
- Preflight: the set of actions sent equals the DryRun-table row set, one request each, checked on the recording doer. A table holding an extra row with no builder refuses, naming that row, before or without sending it.
- Each placeholder request carries its identifying parameter, asserted on the wire. The PR lists each action's placeholder input.
- Preflight table: all-UnauthorizedOperation passes. Each of these refuses, naming the action: a DryRunOperation answer, a 200, InvalidParameterValue, AccessDenied, a 403 with AuthFailure, and a transport error. The inner doer saw only `DryRun=true` requests.
- aws_identity.go and its tests are not in the diff.
- The PR carries the statement above and records a cp-backup break that turns each new test red.
