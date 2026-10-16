---
kind: code
status: ready
touches: [internal/runstore/runstore.go, internal/runstore/runstore_test.go, internal/generator/pitfalls.go, internal/cli/run_command.go, docs/auto-learning-loop.md]
---

# Each run records which pitfalls its prompt carried and which one a failure matched

Nothing records whether a pitfall in `pitfalls/<cloud>.yaml` was in a run's prompt, or whether a
failure it was meant to prevent happened anyway. So "do pitfalls prevent repeats?" cannot be
answered (a Fable review, 2026-10-16; the shelved pitfall-pruning plan needed the same signal).
Add two fields to the run record that internal/runstore writes as run.json:
`pitfalls_rendered`, a list of stable ids for the entries rendered into the generation prompt,
and, on each recorded failure, `matches_pitfall`: the id of a rendered entry for the same
resource type whose rule the failing HCL breaks, or empty. A match means the rule was in the
prompt and the failure happened anyway. The id is stable across runs: a hash of cloud, resource
and rule text, computed in one place in internal/generator. Existing run.json readers keep
working (fields are additive, omitted when empty). docs/auto-learning-loop.md gains a short
section on reading them. No change to what is learned, kept or discarded.

**Done when:**
- `go test ./internal/runstore/... ./internal/generator/... ./internal/cli/...` passes with a test that runs a stubbed generation with two pitfalls rendered and asserts run.json lists both ids, and a test where the failing HCL breaks one rendered rule and the failure's `matches_pitfall` is that id.
- A test shows the id is the same for the same entry loaded twice and differs when the rule text changes.
- An existing run.json without the fields still loads (a test reads a checked-in fixture).
- Each new test is shown failing with its code reverted by a cp backup.
