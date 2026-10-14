---
kind: code
status: blocked
blocked_by: [rehearsal-walls-adr]
epic: rehearsal-walls
depends_on: [rehearsal-walls-adr]
touches: [internal/cli/rehearse_fakeaws_bin.go, internal/cli/rehearse_fakeaws_bin_test.go, docs/stories/fakeaws-loader-pin.md]
risk: high
---

# A fakeaws pin compiled into infrafactory: install it or take --fakeaws-bin, and refuse any binary not built at the pin

New file `internal/cli/rehearse_fakeaws_bin.go`. It holds `rehearsalFakeawsPin` (the full SHA, equal to ci.yml's FAKEAWS_SHA; today ece63928a1ecd9129aa7731544131ff8313009e3, .github/workflows/ci.yml:229) and `rehearsalFakeawsBinary(ctx, runner, flagPath) (string, error)`. When flagPath is set, it must be an absolute, regular, executable file. Otherwise the binary is `<os.UserCacheDir()>/infrafactory/fakeaws/<pin>/fakeaws`, and when it is missing it is installed with `go install github.com/redscaresu/fakeaws/cmd/fakeaws@<pin>`. The install goes through the injected runner with ReplaceEnv and Env exactly {HOME (an empty 0700 dir under the cache), GOBIN, GOPATH, GOCACHE (all under the cache dir), GOTOOLCHAIN=local, GOPROXY=https://proxy.golang.org, GOSUMDB=sum.golang.org, CGO_ENABLED=0}. With no `direct`, no git ever runs.

Every resolved binary is checked by `rehearsalVerifyFakeawsBuildinfo(path, pin string) error`, which rehearsalFakeawsBinary calls with rehearsalFakeawsPin. It reads debug/buildinfo.ReadFile: Path is github.com/redscaresu/fakeaws/cmd/fakeaws and Main.Path is github.com/redscaresu/fakeaws, and either Main.Version is a pseudo-version ending in pin[:12], or vcs.revision equals the pin. Whenever vcs settings are present, vcs.modified must be false. Anything else is refused, naming what was found.

A lockstep test (model cloud_prefix_lockstep_test.go) holds the const equal to ci.yml's FAKEAWS_SHA. This story adds no flag (rehearsal-walls-prepare-e2e adds --fakeaws-bin).

It also edits docs/stories/fakeaws-loader-pin.md: add `internal/cli/rehearse_fakeaws_bin.go` to its touches, and one scope line worded to be true whichever story lands first: "If rehearsalFakeawsPin exists, move it with FAKEAWS_SHA." Ordering: fakeaws-loader-pin is blocked in another epic and bumps FAKEAWS_SHA. Whichever lands second merges main in and moves both; TestRehearsalFakeawsPinMatchesCI fails on any drift, so neither order can merge a mismatch.

**Done when:**
- TestRehearsalFakeawsPinMatchesCI fails when the const and ci.yml's FAKEAWS_SHA differ (one character changed in the const via cp-backup).
- A fake-runner test with an empty cache sees exactly one call, `go install github.com/redscaresu/fakeaws/cmd/fakeaws@<pin>`, with ReplaceEnv and that exact Env key set. Adding `,direct` to GOPROXY or dropping GOTOOLCHAIN turns it red.
- Buildinfo tests build their binaries from a tiny module the test git-inits (module github.com/redscaresu/fakeaws, package main at cmd/fakeaws), commits and builds with -buildvcs=true, passing that commit's own SHA as pin. A clean build is admitted. The same binary with pin set to another SHA is refused, naming the found revision. A build from a dirty tree (vcs.modified=true, revision equal to the pin) is refused, and removing the vcs.modified check turns that case red. A module path other than fakeaws is refused, naming it. A shell script (no build info) is refused.
- A relative --fakeaws-bin path, a directory, and a non-executable file are each refused with zero runner calls.
- TestRehearsalFakeawsInstallAtPin, skipped unless INFRAFACTORY_ENABLE_E2E=1, runs the real install with the real execCommandRunner into an empty temp cache dir, and the installed binary passes rehearsalVerifyFakeawsBuildinfo at rehearsalFakeawsPin. The PR records its local PASS; rehearsal-walls-prepare-e2e lists it in .github/fakeaws-e2e-tests.
- `grep -n rehearse_fakeaws_bin.go docs/stories/fakeaws-loader-pin.md` is non-empty.
- Checks before every stage in docs/epics/rehearsal-walls.md hold.

**From review (binding; overrides the scope above where they differ):**
- The check is split into rehearsalVerifyFakeawsBuildinfo(path, pin), so tests pass their own commit's SHA. A test binary can never carry the const's revision, so a test against the const would let the modified case pass for the wrong reason (skeptic).
- "No build info" is a shell script: every Go binary since 1.18 carries build info (skeptic).
- One real-install test exists, because nothing else in the epic runs the default install (skeptic).
- The fakeaws-loader-pin line is worded for either landing order, and the lockstep test is what catches drift (skeptic).
