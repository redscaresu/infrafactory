---
kind: code
status: ready
epic: prod-read-seam
depends_on: [prod-read-seam-wall, prod-read-seam-credentials]
touches: [docs/layer3/aws/reader-policy.json, internal/harness/aws_reader_policy_test.go, docs/decisions/0023-layer3-sealed-environment-and-orphan-verification.md, docs/operations.md]
risk: high
---

# reader-policy.json, tied to the reader table by a test; ADR-0023 amended; the runbook names the policy

**Policy.** `docs/layer3/aws/reader-policy.json` is Allow statements whose actions are exactly the reader table's rows: no wildcard, no NotAction, and none of the DryRun-table actions. It uses the REGION and ACCOUNT_ID placeholders as iam-policy.json does (aws_scope_policy_test.go:19-31). New `aws_reader_policy_test.go` mirrors readAWSPolicy (aws_scope_policy_test.go:85) without editing it.

**ADR-0023.** A dated amendment, following the 2026-08-23 precedent (ADR-0023:85): a credential that reaches prod is read-only and named to its account, and its policy is reader-policy.json, named in the runbook. "Present only for its read step" means the key file: the read-only IAM user and its key stay between rehearsals, the user supplies the key file for each read, and after each read the lead deletes the local key file, the raw recording and the pre-destroy state once the scrub has produced its fixtures.

**Runbook.** In docs/operations.md, after prod-read-seam-credentials' section: name the policy, the key-file lifecycle above, and that the SCP (scp.json:3-25) leaves the policy's iam:* rows unreachable in the Layer 3 account.

**Done when:**
- TestAWSReaderPolicyIsExactlyTheReaderTable: the policy's actions and the reader table's rows are the same set, and no DryRun-table action is in the policy. Mutated copies each fail it: adding ec2:CreateVpc, ec2:*, ssm:Put*, iam:*, a NotAction statement, one DryRun-table action (e.g. ec2:DeleteVpc), and dropping a row.
- A test fails unless ADR-0023 contains `reader-policy.json`, `read-only` and `key file`; TestADR0023AmendmentRecordsTheAWSImplementation passes unchanged.
- docs/operations.md names reader-policy.json and says the lead deletes the key file, the raw recording and the pre-destroy state after the scrub; `make doc-hygiene` passes.
- docs/layer3/aws/iam-policy.json, scp.json and aws_scope_policy_test.go are not in the diff.
- The PR records a cp-backup break that turns each new test red.
