---
kind: code
repo: fakeaws
status: blocked
blocked_by: [fakeaws-identity-sweep-and-docs]
depends_on: [fakeaws-ami-fixtures-suppressed-when-loaded, fakeaws-identity-sweep-and-docs]
touches: [handlers/admin.go, handlers/ec2.go, handlers/iam.go, handlers/ssm.go, handlers/s3.go, handlers/eks.go, handlers/rds.go, handlers/dynamodb.go, handlers/sqs.go, handlers/secretsmanager.go, handlers/kms.go, handlers/route53.go, handlers/admin_test.go]
risk: high
---

# GET /mock/state fails closed when a table cannot be read

Found in fakeaws#56's review: every `gather*StateReal` discards its repository read error
(`params, _ := app.repo.ListSSMParameters(...)`, and the same in s3, eks, rds, dynamodb, ec2 and
iam). A table that fails to read shows in GET /mock/state as an empty block, so infrafactory's
orphan and destroy checks pass a run that leaked. Each gather returns its error, collectState
stops at the first one, and writeState answers 500 naming the service, never a partial state.

Also fix the stale comment beside ScheduleKeyDeletion that says /mock/reset purges KMS keys
(kmsStore is process-global and survives reset; that gap stays recorded, not fixed here).

**Done when:**
- A test per gather drops or corrupts its table through Repository().DB() and GET /mock/state
  answers 500 naming that service, with no `resources` block in the body.
- With every table readable, /mock/state is byte-identical to before (existing golden tests pass
  with no assertion line changed).
- A source-scan test fails when a non-test gather discards a repository error with `_`.
- Each new test is shown failing by a cp-backup break, recorded in the PR.
