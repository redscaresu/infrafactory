---
kind: code
status: blocked
blocked_by: [prod-read-seam-ec2-readers]
epic: prod-read-seam
depends_on: [prod-read-seam-ec2-readers]
touches: [internal/harness/aws_reader_iam.go, internal/harness/aws_reader_iam_test.go, internal/harness/aws_reader_table.go, docs/layer3/aws/reader-policy.json, internal/e2e/aws_reader_iam_fakeaws_test.go]
---

# IAM readers: a role and its policies, through read-core's replay

For each role in a replayed GetRole exchange: ListRolePolicies, GetRolePolicy for each name, ListAttachedRolePolicies and ListInstanceProfilesForRole (about eight calls per role, HLD:236). Each is an ADR-0041 entry sent through read-core's raw replay to iam.amazonaws.com, signed for us-east-1, paged by IsTruncated/Marker under read-paging's rule.

The epic named a new service/iam client. Raw replay records the raw response bytes ADR-0041 needs, and an SDK client would deserialize them away, so this story adds no SDK client and go.mod stays as is.

Off the wedge's path: the CLI does not wait for it, and neither does the real recording's EC2/SSM/STS proof; only the epic's close does. Proved against fakeaws only, and unproved against AWS, because the SCP denies iam:* (scp.json:3-25; user, 2026-10-11).

**Done when:**
- Recording-fake test: a role with two inline policies over two ListRolePolicies pages, plus one attached policy, records every call. IsTruncated=true without a Marker fails the read, naming the role.
- Every IAM request on the wire goes to iam.amazonaws.com and is signed for us-east-1.
- internal/e2e against fakeaws at FAKEAWS_SHA: a created role's GetRole and its follow-ups are recorded.
- go.mod and go.sum are not in the diff; TestNoProductionCodeUsesTheAWSDefaultChain, prod-read-seam-wall's AST test and TestAWSReaderPolicyIsExactlyTheReaderTable pass.
- The PR states that the IAM readers are proved against fakeaws only, and records a cp-backup break that turns each new test red.
