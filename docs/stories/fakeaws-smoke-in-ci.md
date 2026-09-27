---
kind: code
status: ready
epic: fakeaws-step-one-surfaces
repo: fakeaws
depends_on: []
touches: [".github/workflows/ci.yml", "examples/provider_smoke_test.go", "examples/known_red_test.go (new)", "AGENTS.md", "CHANGELOG.md"]
risk: high
---

# The provider smoke harness runs in fakeaws CI, with known-red examples tracked

New provider-smoke job in .github/workflows/ci.yml: setup-go, OpenTofu 1.12.6 (the bake-off's, .swarm/research/moto.md:8-10), fakeaws built and started on :8082 (the examples' endpoints hardcode it, examples/working/basic_instance/main.tf:22), wait on /healthz, then `INFRAFACTORY_ENABLE_E2E=1 go test ./examples/ -v -count=1 -timeout 30m`. No secrets, runs on pull_request. The harness POSTs /mock/reset before each example. New examples/known_red_test.go: knownRed maps example -> {stage, fragment, owner}. A known-red example that passes fails the test ('remove from knownRed'). One that fails at another stage, or without its fragment, fails the test. One that fails as recorded is logged; no t.Skip (AGENTS.md:62-67). Correct the provider_smoke_test.go:15-19 comment, which says infrafactory CI runs the harness, and the AGENTS.md:164-169 CI job list.

**Done when:**
- The provider-smoke job runs on the PR and passes, and knownRed holds exactly the examples CI shows red, each with a stage, a fragment and an owner story (or 'none: follow-up'). The bake-off predicts basic_instance (plan, user_data), eks_cluster and s3_bucket (.swarm/moto/examples-fakeaws.txt:1,3,7)
- An ungated unit test in the test job covers the xfail decision: pass+known -> error; fail at the recorded stage with the fragment -> ok; fail at another stage or without the fragment -> error; fail+unknown -> error
- provider-smoke is a required status check on fakeaws main (the lead adds it after merge; `gh api repos/redscaresu/fakeaws/rulesets` lists it)
- provider_smoke_test.go's header and AGENTS.md name the fakeaws provider-smoke job as the place the harness runs
