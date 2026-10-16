---
kind: code
status: blocked
blocked_by: [rehearsal-passthrough-commands]
epic: rehearsal-passthrough
depends_on: [rehearsal-passthrough-commands]
touches: [.github/fakeaws-e2e-tests, internal/cli/rehearse_passthrough_args.go, internal/cli/rehearse_passthrough_e2e_test.go]
---

# End to end: every passthrough action against the rehearsal's fakeaws with real tofu

New rehearse_passthrough_e2e_test.go beside rehearse_walls_e2e_test.go: e2e.SkipUnlessEnabled, fakeawsAtPin, local tofu, no cloud. It prepares a root with a var file, then runs through the cobra commands. `plan -- -out=p -detailed-exitcode` gives 2, and p is in the work dir. `apply -- p` gives 0. `state mv` gives 0. `import` of a resource created in fakeaws outside the state gives 0. `destroy -- -auto-approve` gives 0. `console`, if kept, answers var.x from the var file over stdin. `plan -- -var=x=y` is refused before tofu runs. `apply` without -auto-approve fails in tofu and does not hang. Any injected-flag row the run proves wrong is fixed in rehearse_passthrough_args.go.

**Done when:**
- `INFRAFACTORY_ENABLE_E2E=1 go test ./internal/cli -run TestRehearsePassthroughE2E -v` passes in the builder's session against fakeaws at FAKEAWS_SHA, run beside a clone at the pin (memory reference_fakeaws_e2e_against_ci_pin). The output is in the PR. Every step asserts its exact exit code.

**From review (binding; overrides the scope above where they differ):**
- Assert observable results, not only exit codes: plan -out writes p and shows the expected change; apply -auto-approve creates the expected mock resource in fakeaws and in local state; state mv changes its address; import records the external mock ID; destroy removes the mock resource; console (if kept) prints var.x's value.
- Apply runs with -auto-approve (no plan file; see the args story). `apply -- p` is refused by name.
- TestRehearsePassthroughE2E is listed in .github/fakeaws-e2e-tests, and the PR shows its PASS line from the CI log.

The checks in the epic's **Checks before every stage** apply.
