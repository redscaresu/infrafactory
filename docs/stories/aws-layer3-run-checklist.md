---
kind: code
status: blocked
blocked_by: [aws-layer3-ami-resolve-wiring]
epic: aws-layer3-wiring-proof
depends_on: [aws-layer3-ami-resolve-wiring]
touches: ["docs/operations.md", "docs/layer3/coverage.md", "internal/cli/aws_scope_doc_test.go"]
---

# AWS run checklist in operations.md, held to the stage names by a test, and the stale aws-web-live coverage text

Mostly docs, but kind code: it adds Go assertions to aws_scope_doc_test.go that reference the
resolve stage constant from aws-layer3-ami-resolve-wiring.

Add `### Running aws-web-live` under `## Layer 3 (AWS)` in docs/operations.md, after
'Re-running the planted-leak proof' (:323) and before `## Sibling mocks` (:334), so
TestAWSScopeSetupRunbookNamesWhatTheCodeReads (aws_scope_doc_test.go:16-36), which cuts at the
next `\n## `, still covers it. Content:

- What to source: the key file and its mode, the config's aws block, and sandbox_deploy (off by
  default, infrafactory.yaml:81).
- What refuses, in two ordered phases, because generation and the gate (an LLM cost) sit between
  them: (1) before generation — credentials file, STS account and principal, the AMI resolve;
  (2) after the gate, before apply — credentials env, stamp, default VPC, a held claim naming its
  holder (aws_scope_lifecycle.go:31-53).
- What a failure prints: the claim-kept stage and the reap command.
- That the first real AMI resolve is denied until ssm:GetParameter on the AL2023 public path is
  granted. Link docs/layer3/aws/iam-policy.json (already cited, aws_scope_doc_test.go:29) and name
  the epic aws-web-live-on-real-aws in plain text as the owner of that widening — no link to an
  epic file or a future anchor, and no action list.

In docs/layer3/coverage.md, the aws-web-live row's blocker cell (:119) and the note under the
totals (:136-137) stop saying the AWS gate is unbuilt (#405 built it); the status cell stays
'runnable, unrun' (the flip is aws-web-live-on-real-aws's). Markdown links only; no undated counts
(AGENTS.md:132-133). Commit trailer: `ADR: none — docs`.

**Done when:**
- aws_scope_doc_test.go fails if the new subsection is missing, or omits any of these literals:
  `### Running aws-web-live`, the resolve stage constant's value, StageAWSScopeClaimKept's value,
  and reapCommand(config.DefaultPath, "scenarios/training/aws-web-live.yaml").
- `grep -n 'not yet built' docs/layer3/coverage.md` returns no aws-web-live line, and
  TestLayer3CoverageDocTotalsMatchItsTable passes with the totals line unchanged.
- The subsection contains no IAM action list: its only action name is ssm:GetParameter, in the
  sentence about the public parameter.
- `make doc-hygiene` passes.
