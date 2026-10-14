---
kind: code
status: blocked
blocked_by: [rehearsal-walls-fakeaws-pin, rehearsal-walls-provider-override, rehearsal-walls-gate-admission, rehearsal-walls-fakeaws-start]
epic: rehearsal-walls
depends_on: [rehearsal-walls-fakeaws-pin, rehearsal-walls-provider-override, rehearsal-walls-gate-admission, rehearsal-walls-fakeaws-start]
touches: [internal/cli/rehearse_prepare.go, internal/cli/rehearse_prepare_init.go, internal/cli/rehearse_prepare_copy.go, internal/cli/rehearse_walls.go, internal/cli/rehearse_walls_test.go, internal/cli/rehearse_prepare_init_test.go, internal/cli/rehearse_prepare_test.go, internal/cli/command_golden_test.go]
risk: high
---

# prepare derives the endpoints keys, starts fakeaws, writes the record and the provider override, behind injectable seams

rehearsePrepare gains three injectable seams beside its runner: the schema call goes through p.tofu; a binary resolver `fakeawsBin func(ctx, runner, flagPath string) (string, error)`, defaulting to rehearsalFakeawsBinary and passed the `fakeawsBinPath` field (empty until rehearsal-walls-prepare-e2e binds --fakeaws-bin to it); and a starter, defaulting to rehearsalStartFakeaws.

New file `internal/cli/rehearse_walls.go` holds the record type, its 0600 writer (writeSynced) and its reader. The record holds: schema, account, region, pin, pid, port, token, and `roots`, a list whose entries each hold a work dir and root path (relative to <id>), endpoint_keys and aws_aliases. Today prepare makes one work dir by its existing copy convention (rehearse_prepare_copy.go:80-96), so `roots` has one entry; rehearsal-against-the-copy adds base and head.

After the second init, prepare:
1. runs `tofu providers schema -json` through p.tofu with ReplaceEnv and the second init's exact env, and derives keys with rehearsalEndpointKeys;
2. resolves the binary through fakeawsBin;
3. starts fakeaws through the starter with --account and --region, `<id>/fakeaws` as its dir;
4. writes `<id>/rehearsal.json` at 0600 with pid, port and token at once;
5. writes `<root>/infrafactory_aws_override.tf` at 0600 per root with rehearsalProviderOverride, using the gate's aws aliases;
6. rewrites the record complete, and tightens modes.

Any error or cancel after the start calls stop() before the deferred RemoveAll (rehearse_prepare_copy.go:63-67), so #436/#438 hold: no success after a cancel, and no half-made rehearsal. prepare's printed output does not change here (no fakeaws_url yet), so the command goldens are unchanged. The golden harness (command_golden_test.go:349) injects a stub resolver returning a fixed path, a starter returning a fixed port, and a fake tofu that answers the schema call with a fixture.

**Done when:**
- A fake-runner test checks that the runner log ends [second init, providers schema -json], with the schema call under ReplaceEnv and the second init's exact env key set. The resolver and then the starter are called after it, and the record and both override files exist at 0600.
- Fake-runner tests inject a failure after the start (the override write fails) and a cancel after the start. Each gives stop() called exactly once, no <id> dir, exit 1, and no 'prepared' output. Removing the stop call turns them red (cp-backup).
- A test makes the override write fail (the root made read-only) and, from inside the stub starter's stop(), lists the rehearsal dir: rehearsal.json is there at 0600 holding pid, port and token, and infrafactory_aws_override.tf is not. Moving the record write after the override write turns it red (cp-backup).
- A schema stdout without sts refuses the prepare before the resolver or the starter is called.
- With fakeawsBin unset, a test shows the default calls rehearsalFakeawsBinary with the fakeawsBinPath value (rehearsalFakeawsBinary's runner call is visible on the fake runner); every other fake-runner prepare test injects a stub resolver, so no test depends on the machine's cache.
- The record reader round-trips the writer's output, roots list included, and refuses an unparseable file.
- TestCommandOutputGoldenSnapshots passes, and `git diff origin/main -- internal/cli/testdata/golden/commands` is empty.
- Checks before every stage in docs/epics/rehearsal-walls.md hold.

**From review (binding; overrides the scope above where they differ):**
- Split from the original prepare-wiring: the --fakeaws-bin flag, user-facing output, goldens and the real-fakeaws e2e move to rehearsal-walls-prepare-e2e (codex).
- The binary resolver is injectable beside the starter and defaults to rehearsalFakeawsBinary, so a cold or warm cache cannot change a unit test's outcome; command_golden_test.go is in touches (skeptic).
- rehearse_walls.go holds the record type, writer and reader (skeptic).
- The record (pid, port, token) is written straight after a successful start, before the override write, so a prepare killed there leaves what `end` needs (critic).
- The record holds a list of roots, and the override is written per root (critic).
