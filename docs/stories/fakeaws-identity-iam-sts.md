---
kind: code
repo: fakeaws
status: blocked
blocked_by: [fakeaws-arn-builders-take-account, fakeaws-identity-store-and-flag]
epic: fakeaws-loaded-identity
depends_on: [fakeaws-arn-builders-take-account, fakeaws-identity-store-and-flag]
touches: [handlers/iam.go, handlers/sts.go, handlers/iam_test.go, handlers/sts_test.go]
risk: high
---

# IAM and STS answer the stored account

**IAM.** In iam.go, handleIAM's `const account` (iam.go:43) and gatherIAMStateReal's (:220) become reads of `AccountID()`. Every IAM ARN and row key uses that value.

**STS.** GetCallerIdentity answers the stored id in both `Account` and `Arn` (sts.go:56-61). The provider's allowed_account_ids check reads the Arn; `aws_caller_identity` reads Account.

**Read errors.** If the identity read fails, the handler answers 500 InternalFailure in the service's error shape. It never falls back to the default. The gather can treat the error like its other repo calls, because collectState has already failed GET /mock/state on it.

**Contract.** The contract id `sts-caller-identity-fake-account` stays, since coverage_matrix.yaml:474, sts_test.go:34 and AGENTS.md:151 reference it. Only its body changes: "the stored account, 000000000000 when none".

**Order:** fakeaws-request-log-gap-markers also edits handleIAM (the default arm, iam.go:210-212). This story comes after it through arn-builders and rebases onto it; the hunks differ.

**Done when:**
- In an app started with 123456789012:
  - GetCallerIdentity answers Account 123456789012 and Arn `arn:aws:iam::123456789012:user/fakeaws`.
  - The CreateRole, CreatePolicy, CreateInstanceProfile and CreateUser ARNs carry 123456789012.
  - GET /mock/state iam lists those rows with those ARNs.
- With the identity table dropped through Repository().DB():
  - GetCallerIdentity answers 500, and the body has no 000000000000.
  - CreateRole answers 500 InternalFailure, and no role row is written.
- TestContract_sts_caller_identity_fake_account passes with its assertions unchanged. coverage_matrix.yaml is untouched, and TestFullCoverageAudit passes.
- `git grep -n -e FakeAccountID -e 000000000000 handlers/iam.go handlers/sts.go` prints nothing.
- Each new test is shown failing by a cp-backup break, recorded in the PR:
  - sts.go reverted to the constant;
  - handleIAM ignoring the read error.
