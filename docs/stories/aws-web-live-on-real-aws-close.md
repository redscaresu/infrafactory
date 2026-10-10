---
kind: lead
status: blocked
blocked_by: [aws-web-live-real-run, aws-layer3-iam-measured, aws-layer3-claim-legs, aws-web-live-holdout-mutation, aws-learned-pitfall-from-real-aws, aws-probe-window-measured]
epic: aws-web-live-on-real-aws
depends_on: [aws-web-live-real-run, aws-layer3-iam-measured, aws-layer3-claim-legs, aws-web-live-holdout-mutation, aws-learned-pitfall-from-real-aws, aws-probe-window-measured]
touches: [AGENTS.md, CONCEPT.md, STATUS.md, docs/hld/2026-09-27-aws-web-stack.md, "docs/epics/aws-web-live-on-real-aws.md (delete)"]
---

# AGENTS.md and CONCEPT.md describe AWS Layer 3, and the PR closes the epic

The AGENTS.md and CONCEPT.md half of the epic's last bullet, and the close. No real run.

**You:** nothing before the PR; review it and merge, or tell the lead to.

AGENTS.md § Layer 3 (:154-159; heading 'Layer 3 (real Scaleway)') gets one AWS sentence. It points
at docs/operations.md § Layer 3 (AWS) for the credential file, the whole-account scope (ADR-0040)
and the probe contract (ADR-0033/0039), and keeps the ADR-0025 sentence. CONCEPT.md has no AWS
Layer 3 text: add it to § 11 Layer 3 credential contract (:610), covering
`~/.config/infrafactory/layer3-aws.env` at 0600 with exactly AWS_ACCESS_KEY_ID and
AWS_SECRET_ACCESS_KEY, the aws block (region, account_id, principal_arn) and the exact STS match.
Add one AWS paragraph under § Real Scaleway Deploy (Layer 3) (:465-479); it is not a cost estimate,
so the 'Cost: no estimation' bullet (:473) stays true. The PR deletes
docs/epics/aws-web-live-on-real-aws.md, turns the HLD's `## Epics` link (:479) into a
'— done (#PRs)' entry, and moves STATUS.md Now to aws-web-stack-load-balancer.

**Done when:**
- `grep -n 'layer3-aws.env' CONCEPT.md AGENTS.md` returns at least one line in each. CONCEPT.md
  names AWS_ACCESS_KEY_ID, AWS_SECRET_ACCESS_KEY, region, account_id and principal_arn. AGENTS.md
  still contains 'ADR-0025'.
- docs/epics/aws-web-live-on-real-aws.md is gone.
  `grep -rn 'epics/aws-web-live-on-real-aws' docs STATUS.md AGENTS.md README.md` returns nothing
  outside docs/archive. TestDocLinksResolve passes.
- The HLD line reads aws-web-live-on-real-aws — done with its PR numbers, and STATUS.md Now names
  the next epic.
- The PR body holds every real-cloud run id with its evidence bundle (aws-layer3-iam-measured,
  aws-layer3-claim-legs, aws-web-live-real-run, aws-web-live-holdout-mutation,
  aws-learned-pitfall-from-real-aws) and the cumulative dated cost bound, under €5. Code-only PR
  numbers are listed separately. No account id (the configured `aws.account_id` in any form, or an ARN's account field) in the PR body.
- `make doc-hygiene` and `go test -tags noui ./internal/generator/ ./internal/cli/` pass.
