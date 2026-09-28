---
kind: lead
status: blocked
blocked_by: [aws-default-deny-ingress-policy, aws-no-instance-profile-policy, aws-phase2-ingress-shape, aws-web-live-holdout]
epic: aws-ingress-policy-and-holdout
depends_on: [aws-default-deny-ingress-policy, aws-no-instance-profile-policy, aws-phase2-ingress-shape, aws-web-live-holdout]
touches: ["scenarios/training/aws-web-live.yaml (new)", "internal/scenario/scenario_aws_test.go", "docs/epics/aws-ingress-policy-and-holdout.md (deleted)", "docs/hld/2026-09-27-aws-web-stack.md", "docs/stories/aws-web-live-scenario.md (deleted)", "docs/review-passes/aws-web-live-scenario.md (new)"]
---

# scenarios/training/aws-web-live.yaml lands (the lead runs the model on it), and the epic closes

- This story is lead-run, not agent-built, because adding a scenarios/training file runs a model. On this PR, scenario-gate.yml does the following. It clones mockway, fakegcp and fakeaws at main (not the CI pin). If OPENROUTER_API_KEY is unset, it warns and skips green having run nothing. If the key is set, it switches the agent to openrouter anthropic/claude-sonnet-4, runs `make mocks-up`, then `infrafactory run --clean` on each changed scenario (720s cap each), and fails unless Status is success with terminal reason target_reached (scenario-gate.yml:22-26,44-48,80-110; scripts/scenario_change_gate.sh).
- Before opening the PR, the lead runs `infrafactory run scenarios/training/aws-web-live.yaml` against `make mocks-up`, within the standing €5 cap, and records the result in the review-pass file. The PR never merges on a red gate. A red gate is the lead's to fix at the source (pipeline or mock), never by editing pitfalls.
- The scenario lifts web-step-one.yaml (anchors, networking.vpc) and adds: service nginx "1.27" on port 80, health_path /, ttl 4h; compute web-server small count 1; criteria region_restriction (us-east-1, expect pass), default_deny_ingress (expect pass) and http_probe compute 80 expect reachable. It has no connectivity criterion, so the generator never sees a holdout check (ADR-0033).
- As the last story, it deletes docs/epics/aws-ingress-policy-and-holdout.md and marks the epic done in the HLD epic list. check_doc_hygiene.sh:132-135 goes red while any story still names the epic, so this story depends on all the others.

**Done when:**
- A test loads aws-web-live.yaml with the schema and pins cloud aws, the service (nginx, 1.27, port 80, ttl set) and compute (small, count 1). It also pins the exact criteria list, each with its expect: region_restriction params.region us-east-1 pass, default_deny_ingress pass, http_probe compute 80 reachable. Changing any name, param or expect fails it.
- A test asserts that aws-web-live.yaml has no connectivity criterion, and that none of aws-web-live-unseen.yaml's criteria appears in it.
- TestAWSPhase1PromptCarriesALiteralSizeTable passes with aws-web-live declaring compute small.
- check-doc-hygiene passes with the epic file deleted.
