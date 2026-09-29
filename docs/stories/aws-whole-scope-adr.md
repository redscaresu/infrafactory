---
kind: docs
status: ready
epic: aws-layer3-claim-sweep-reap
depends_on: [aws-scope-run-failure-path, aws-scope-planted-leak-proof]
touches: ["docs/decisions/0040-aws-whole-scope-ownership.md (new)", "docs/decisions/0023-layer3-sealed-environment-and-orphan-verification.md", "docs/decisions/0025-run-project-created-before-the-apply.md", "docs/decisions/README.md", "internal/harness/aws_scope_adr_test.go (new)", "docs/epics/aws-layer3-claim-sweep-reap.md (delete)", "docs/hld/2026-09-27-aws-web-stack.md", "docs/stories/aws-layer3-gate-lift.md", "docs/epics/aws-layer3-wiring-proof.md", "docs/epics/aws-web-live-on-real-aws.md", "STATUS.md"]
risk: high
---

# ADR-0040 records whole-scope ownership for AWS and ADR-0023 records the third implementation. This story closes the epic and carries the unproven legs into the lift

It adds new file docs/decisions/0040-aws-whole-scope-ownership.md, which records: the claim as a compare-and-swap with a per-process holder; retry recognition; release by value, with its Get-then-Delete ceiling; takeover only by name, refusing a live local holder; the claim taken after the stamp and default-VPC checks, and released only by awsReleaseAfterCleanSweep; one take and one release per test execution; the run ending when the claim is kept; the stamp, which only the admin can write; the collection table; a settle that re-polls the whole scope; token-cycle and 403 failing closed; ReapAWSScope's delete-site gate; the report-only SSM row; the IAM policy tied to the sweep by a test; the SCP as the boundary preflight cannot assert, with its exemption; tags annotating and never gating; and the proof's date and PR. This is the only story that edits ADR-0023's record. It adds one dated amendment: AWS is the third implementation, with rules 1-2 by aws-layer3-seal-and-dispatch (#288-#303), 3-4 by ADR-0040 and 5 by aws-layer3-gate, and ADR-0025 is not carried over. It adds a one-line note to ADR-0025 and runs `make adr-index`. Closing the epic: delete the epic file; mark it done in the HLD's Epics list (HLD:473). In aws-layer3-gate-lift.md: drop the two satisfied blockers, reuse Deps.AWSEC2 (line 24), route its gate signature change through Deps.Layer3HCLGate, and name the unproven legs from the proof PR. Add those legs to docs/epics/aws-web-live-on-real-aws.md's Done. Amend aws-layer3-wiring-proof.md:10's claim order. Update STATUS.md.

**Done when:**
- internal/harness/aws_scope_adr_test.go fails unless ADR-0040 names every row of the sweep table and every precedence pair, and unless ADR-0023's amendment names ADR-0040, aws-layer3-seal-and-dispatch and 'ADR-0025 is not carried over'. Adding a table row without updating the ADR fails it.
- `make adr-index` leaves no diff, and make doc-hygiene passes.
- The epic file is gone and the link test passes. aws-layer3-gate-lift.md no longer lists aws-layer3-claim-sweep-reap or operator:planted-leak-proof in blocked_by, and names each unproven leg. aws-web-live-on-real-aws.md's Done lists those legs.
