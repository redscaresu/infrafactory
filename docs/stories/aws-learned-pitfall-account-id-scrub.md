---
kind: code
status: ready
epic: aws-web-live-on-real-aws
depends_on: []
touches: [internal/generator/redaction.go, internal/generator/redaction_test.go, internal/generator/pitfalls_learn.go, internal/generator/policy_gap.go, internal/generator/policy_gap_test.go]
risk: high
---

# No learned pitfall, avoid-ledger entry or policy-gap row is written with a 12-digit AWS account id

Learned rules copy real failure text verbatim (internal/generator/pitfalls_learn.go:526-535), and
redaction.go has no rule for account ids. A real AccessDenied naming
`arn:aws:iam::<acct>:user/...` would land in public pitfalls/aws.yaml. Two written files are
published:
- pitfalls/*.yaml and the avoid ledgers, all marshalled by writePitfallsFile
  (pitfalls_learn.go:936; every caller in pitfalls_learn.go, pitfalls_retire.go and
  pitfalls_avoid_retire.go goes through it);
- docs/policy-gaps.md, written by AppendPolicyGap (internal/generator/policy_gap.go:192-258) with
  the failure detail verbatim. It is not gitignored, so a run that writes it can commit it.

Add one new function, `scrubAccountIDs`, in redaction.go, and call it at those two sinks only.
AppendMockGap (pitfalls_learn.go:188) may call it too, cheaply, but docs/mock-gaps.md is gitignored
(.gitignore:62), so it guards nothing published and no test is needed for it. Do NOT fold the rule
into redactTransportDetail or RedactSecretLikeText (redaction.go:15-48): they serve transport and
log redaction at other call sites, and nothing here pins that behaviour. The scrub replaces every
standalone run of exactly 12 digits with `ACCOUNT_ID`, the placeholder iam-policy.json already uses
(internal/harness/aws_scope_policy_test.go:28-29, refused at :61). It fails closed on the class
(any 12 digits), not on a list of ARN shapes. isDuplicate's word-share check still dedups a raw
candidate against a scrubbed entry.

Out of scope: avoid shapes (WriteAvoidShape, avoid_shape.go:165-180). They hold HCL cuts written by
an operator-run command (pitfalls_avoid_check.go:86), not failure text, so no real error message
reaches them. Commit trailer: `ADR: none — redaction at the write sinks`.

**Done when:**
- A generator test appends a learned aws_ pitfall whose rule holds
  `arn:aws:iam::123456789012:user/x` and `account 123456789012`. The written pitfalls/aws.yaml has
  no 12-digit run and has ACCOUNT_ID twice. The same check passes for an AppendLivePitfall entry
  and an avoid-ledger write.
- An AppendPolicyGap case writes a gap whose detail holds the same two strings; docs/policy-gaps.md
  then has no 12-digit run and has ACCOUNT_ID.
- Deleting the scrub from writePitfallsFile, and separately from AppendPolicyGap (cp backup,
  restore by cp), each fails a test; the PR records both.
- Thirteen digits and eleven digits are left unchanged, and a test pins both.
- `go test -tags noui ./internal/generator/... ./internal/cli/...` and `make doc-hygiene` pass.
