---
kind: code
status: ready
depends_on: []
touches: [internal/cli/runtime.go:newCommandRuntime, internal/cli/*_test.go (the tests that reach schemaRunner)]
---

# internal/cli tests stop running a real network tofu init

Found in review of #607. `newCommandRuntime` hard-codes `schemaRunner: execCommandRunner{}`
(internal/cli/runtime.go:433), so tests that build a runtime run a real `tofu init` that downloads
providers. #607's shared plugin cache cut the hook to about 108s, but `make test`,
PRECOMMIT_FULL and check_all.sh still run internal/cli cold, under go's 10-minute default. Let the
runtime options inject the schema runner. Have the tests that only need a schema use a stub that
returns a recorded schema fixture. Keep at least one test on the real runner, behind the
existing tag or env CI uses for network tests.

**Done when:**
- `go test ./internal/cli/` with an empty plugin cache and the network off passes. Give the run time in the PR body, with the cold time before.
- CI still runs the real-runner test (named in the PR body, with its job).
