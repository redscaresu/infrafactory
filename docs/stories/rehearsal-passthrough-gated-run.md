---
kind: code
status: blocked
blocked_by: [rehearsal-passthrough-runner-stream, rehearsal-passthrough-exit-code, rehearsal-passthrough-args]
epic: rehearsal-passthrough
depends_on: [rehearsal-passthrough-runner-stream, rehearsal-passthrough-exit-code, rehearsal-passthrough-args]
touches: [internal/cli/rehearse_passthrough.go, internal/cli/rehearse_passthrough_test.go]
risk: high
---

# The gated run: open, prove fakeaws, re-gate, run tofu under the walls, tighten, map the exit

New rehearse_passthrough.go: the one function that runs one tofu action in a rehearsal, and the seam rehearsal-against-the-copy reuses. It holds notifyGuarded for the whole call (exec_runner.go:142). rehearsalRepo, then rehearsalOpen: every error is a refusal, proxy-live and record included (end only warns on those, rehearse_end.go:99-103). It refuses a record with more than one root. It proves the recorded fakeaws with rehearsalEndProof (rehearse_end.go:133). It builds argv with the args function and refuses if ctx is cancelled. It runs rehearsalRegate on the root right before the run. Tofu runs through the injected runner with Dir set to the root's work dir, Env from rehearsalWallsEnv(..., rehearsalFakeawsURL(port), region), ReplaceEnv, the caller's writers (plus the reader for console if kept), and no timeout. rehearsalTightenModes runs after every run. The exit is mapped as in decisions. Tip commit: `ADR: none — no command calls it until rehearsal-passthrough-commands`.

**Done when:**
- `go test ./internal/cli -run TestRehearsePassthroughRun -v` passes, using a stub runner that records its calls. A planted infrafactory_override.tf that is not byte-identical is refused and the runner is never called. The re-gate runs before every call: a stub that edits an override on call 1 gets call 2 refused. Proxy-live and a refused record are refused without the runner. Two roots are refused, naming both. A failing fakeaws proof is refused without the runner. A cancelled ctx with the stub returning 0 gives ExitCodeForError 1, never nil. A stub exit of 2 gives 2. -1 gives 1. The Command the stub sees has Dir <id>/<root>, ReplaceEnv true, Env keys equal to rehearsalWallsEnv's, and argv with the injected flags. Modes are tightened after a failed run too.

**From review (binding; overrides the scope above where they differ):**
- Replace the cancelled-ctx case with a stub that cancels the guard ctx during its call (through the injected notify) and returns ExitCode 0, nil; assert ExitCodeForError gives 1. A separate case checks that a ctx cancelled before the run is refused and the runner is never called.
- Assert the Command's Env equals rehearsalWallsEnv(r.dir, rehearsalFakeawsURL(record.Port), record.Region) as a whole map (assert.Equal), not its keys.
- Plant a non-identical infrafactory_aws_override.tf as well as infrafactory_override.tf. Each is refused, with the runner never called.
- Tighten modes on the whole rehearsal dir, as prepare and walls do (rehearse_walls.go:140). A failed tighten gives exit 1, and a test shows it.
- Refuse a record unless it has exactly one root. A schema-valid record with zero roots is a named refusal, exit 1, runner never called, and never an index panic.
- Deadline (AGENTS.md:152): tofu runs under a constant rehearsalPassthroughTimeout of 2 hours, not rehearsalTofuTimeout's 10 minutes. Ctrl-C is the normal stop. A test shows that when the deadline passes, the process group is killed and the result is exit 1. ADR-0042 records the bound (commands story).
- The fixture for the stub-runner test needs a record whose port passes rehearsalEndProof and matches the provider override prepare wrote. Build it with a helper beside regatePrepared, not by hand-editing the record.

The checks in the epic's **Checks before every stage** apply.
