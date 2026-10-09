---
kind: code
status: ready
depends_on: []
touches: ["internal/cli/run_command.go", "internal/cli/test_command.go", "internal/cli/run_command_test.go"]
---

# A mock reset failure ends `run` at once as an environment error, without spending a repair generation

Seen on the aws-web-live Layer 2 run 20261009T213429Z (2026-10-09). The s3 mock's url answered
HTML, because a different service held SeaweedFS's port behind s3router. Every iteration then
failed at `iteration_N_test`, check `reset` ("s3 reset: parse list response: expected element type
<ListAllMyBucketsResult> but have <html>"; test_command.go:336). `run` treated that as an
iteration failure and generated again with the LLM. It then stopped with terminal reason `stuck`
after 2 iterations, and no HCL had ever been applied. A reset failure says the environment is
wrong, not the HCL. Repair cannot fix it, so each lap wastes a model call, and the run looks like a
pipeline bug.

Make a `reset` failure end the run on the first iteration with its own terminal reason (e.g.
`mock_unavailable`). The output names the mock and its url, so the operator can see which service
is wrong. No repair generation runs, and nothing is learned from it: no pitfall, no mock-gap.
Commit trailer: `ADR: none — run stops on an environment fault`.

**Done when:**
- A `run` test whose reset fails (a stub mock serving HTML) ends after iteration 1 with terminal
  reason `mock_unavailable`. The generator is called exactly once, and the output names the mock and
  its url.
- No pitfall or mock-gap entry is written for that run.
- Removing the new branch (cp backup, restore by cp) makes the test fail with reason `stuck` or a
  second generator call; the PR records it.
- `go test -tags noui ./internal/cli/...` and `make doc-hygiene` pass.
