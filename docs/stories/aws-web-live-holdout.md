---
kind: code
status: ready
epic: aws-ingress-policy-and-holdout
depends_on: []
touches: ["scenarios/holdout/aws-web-live-unseen.yaml (new)", "internal/scenario/holdout_test.go", "internal/harness/real_probe.go (only if the resolver needs an aws_instance case)", "internal/harness/real_probe_test.go", "internal/harness/testdata/realprobe/aws/ (new)", "docs/stories/aws-web-live-holdout.md (deleted)", "docs/review-passes/aws-web-live-holdout.md (new)"]
---

# scenarios/holdout/aws-web-live-unseen.yaml is discoverable, and the real probe dials aws_instance.public_ip

- The holdout keeps the HLD contract (HLD Goals 2, :37-38; aws-web-live-on-real-aws.md:7,10): public_internet -> compute 22 blocked and 443 blocked, with 443 named the positive control. It adds public_internet -> compute 80 success, because a blocked dial passes on any dial error (real_probe.go:140-150). See contradictions.
- The file mirrors web-live-paris-unseen.yaml: type holdout, references aws-web-live, cloud aws, no resources key. The path is scenarios/holdout/, so it does not trigger the scenario gate.
- The resolver fixture is a terraform state captured by applying web-step-one.tf on fakeaws, trimmed and written as terraform-live.tfstate, under internal/harness/testdata/realprobe/aws/. aws_instance falls through to pickHost's default patterns today (real_probe.go:384, :418-420). If they pick anything other than public_ip, add an aws_instance case.

**Done when:**
- A test runs DiscoverCriteriaOnlyHoldouts(<repo>/scenarios/holdout, "aws-web-live") and gets exactly aws-web-live-unseen.yaml. That file loads with the schema, and each criterion is pinned: type, from, to, port and expect for 22 blocked, 443 blocked and 80 success. Discovery for web-live-paris is unchanged.
- A real_probe_test case gives resolveProbeHost(captured state, "compute") and gets aws_instance.public_ip, not private_ip or public_dns.
- RealProbeHarness.Run over the holdout's three checks, with a recording dialFunc, dials that same public_ip on 22, 443 and 80.
