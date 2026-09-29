---
kind: code
status: ready
repo: fakeaws
touches: ["examples/provider_smoke_test.go", "examples/known_red_test.go", "handlers/"]
---

# fakeaws `provider-smoke` runs in under 5 minutes, then becomes a required check

The user's decision (2026-09-29): `provider-smoke` becomes required on fakeaws `main` once it is
fast. It took 34 minutes on run 36501856838 with 41 examples, all against the local fake, for three
reasons:

- The examples run one at a time: `provider_smoke_test.go` never calls `t.Parallel`.
- Each `tofu` step starts the AWS provider (about 2-3 s), 4-6 steps per example.
- Six examples take half the run in provider polling loops tuned for real AWS:
  `update_rds_parameter_group` 265 s, `update_sqs_queue_visibility` 233 s (known red; waits out a
  timeout before failing), `rds_instance` 171 s, `update_default_tags` 105 s, `route53` 90 s,
  `update_tags_every_service` 90 s.

Run the examples in parallel (each has its own directory; check the fake's names do not collide).
For each slow example, find what the provider polls for and make the fake answer with the finished
state at once. Fix the known-red SQS example or make it fail fast.

**Done when:** `provider-smoke` passes in under 5 minutes on a PR, with no example removed and no
new known-red entry; then `gh api repos/redscaresu/fakeaws/rulesets` lists `provider-smoke` as a
required check.
