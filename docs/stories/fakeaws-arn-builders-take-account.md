---
kind: code
repo: fakeaws
status: ready
epic: fakeaws-loaded-identity
depends_on: []
touches: [handlers/awsproto/arn.go, handlers/awsproto/awsproto.go, handlers/awsproto/awsproto_test.go, handlers/ec2.go, handlers/iam.go, handlers/sts.go, handlers/eks.go, handlers/rds.go, handlers/sqs.go, handlers/secretsmanager.go, handlers/dynamodb.go, handlers/arn_default_test.go]
---

# ARN builders take the account as a parameter; every caller still passes the default

A pure refactor of package awsproto. The output stays byte-identical.

The 21 builders that embed an account (IAM x4, EC2 x7, RDS x5, EKS x2, SQS, Secrets Manager,
DynamoDB; `handlers/awsproto/arn.go:20-145`) gain an account parameter, in one order:
- regional builders take `(region, account, ...)`, matching `BuildSSMParameterARN` (arn.go:156);
- IAM builders take `(account, name)`.

The S3 and Route53 builders carry no account and do not change. Every non-test call site passes
the `account` already in scope. The three sites with no `account` in scope (ec2.go:2071,
sqs.go:285, sts.go:58) pass `awsproto.FakeAccountID`. Use a parameter, never a package global:
one test process holds many Applications (testutil/testutil.go:35). The arn.go header (:15-16),
the FakeAccountID doc (awsproto.go:25-33) and the comment in awsproto_test.go:294-296 now call
it the default account, not the only one.

**Order:** this story edits ec2.go, iam.go, rds.go, dynamodb.go, sqs.go and secretsmanager.go,
which fakeaws-request-log-gap-markers also edits. This story goes first (a mechanical signature
change, ready now); gap-markers depends on it and rebases. It takes the first turn in the
cross-epic ec2.go chain: one ec2.go story open at a time.

**Done when:**
- `git grep -n FakeAccountID handlers/awsproto/arn.go` prints nothing.
- `go test ./...` passes. In awsproto_test.go, each changed assertion line differs from the
  original only by the added `awsproto.FakeAccountID` argument, and no expected string changes.
- A new table in awsproto_test.go, modelled on :246-284, asserts the exact full ARN for all 21
  builders under account 123456789012 in us-east-1. It uses equality, not "contains".
- New `handlers/arn_default_test.go` runs at the default account. It creates one resource per
  builder call site and asserts each full ARN string equals today's (e.g.
  `arn:aws:ec2:us-east-1:000000000000:vpc/vpc-...`). This covers the 15 builders that no handler
  test asserts today.
- Each new test is shown failing by a cp-backup break, recorded in the PR:
  - one builder ignores its account (awsproto_test fails);
  - region and account are swapped at one ec2.go call site (arn_default_test fails).
