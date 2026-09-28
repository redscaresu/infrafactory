---
kind: code
status: blocked
blocked_by: [aws-preflight-sts, fakeaws-sts-caller-identity]
epic: aws-layer3-seal-and-dispatch
depends_on: [aws-preflight-sts, fakeaws-sts-caller-identity]
touches: [".github/workflows/ci.yml", "internal/e2e/aws_preflight_fakeaws_test.go (new)", "docs/stories/aws-preflight-against-fakeaws-sts.md (delete)"]
risk: high
---

# BLOCKED on fakeaws-sts-caller-identity: the AWS identity check passes against fakeaws STS in the required CI job and refuses a different account

Not dispatchable until fakeaws-sts-caller-identity merges. That story is blocked on fakeaws-provider-exact-pin, which waits on fakeaws-smoke-in-ci (docs/stories/fakeaws-sts-caller-identity.md:3-4, fakeaws-provider-exact-pin.md:3-4).

New internal/e2e/aws_preflight_fakeaws_test.go uses SkipUnlessEnabled and StartFakeaws (internal/e2e/helpers.go:37-41,206-221). It calls VerifyAWSIdentity with an env from AWSSealedEnv, the loopback-only doer, and the endpoint <fakeaws>/sts. The expected identity is account 000000000000 and the fixed ARN fakeaws's sts contract documents. In .github/workflows/ci.yml, a step in the existing required test job clones redscaresu/fakeaws at a pinned 40-hex SHA to ../fakeaws and runs INFRAFACTORY_ENABLE_E2E=1 go test ./internal/e2e -run '^TestAWSPreflightAgainstFakeaws' -v. Because it is in the existing required job, the ruleset does not change. Bumping the SHA is a deliberate edit.

**Done when:**
- The CI step passes on the PR and fails when the test is skipped or absent: it requires '--- PASS: TestAWSPreflightAgainstFakeaws' in its output
- Against fakeaws, account 000000000000 with fakeaws's ARN passes. account_id 111111111111 refuses, naming both accounts. A different principal refuses
- The default `go test ./...` step still skips the test without the E2E gate
