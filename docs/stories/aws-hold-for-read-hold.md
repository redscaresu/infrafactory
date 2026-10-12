---
kind: code
status: ready
epic: aws-hold-for-read
depends_on: [aws-hold-for-read-adr-0040, aws-hold-for-read-release, aws-hold-for-read-guard]
touches: [internal/cli/root.go, internal/cli/test_command.go, internal/cli/aws_scope_lifecycle.go, internal/cli/aws_hold.go, internal/cli/aws_hold_test.go]
risk: high
---

# `test aws-web-live --hold` keeps the stack under the claim only when every check passed, the run was not interrupted and its tfstate copy is written

Hot file: internal/cli/root.go is also touched by rehearsal-prepare-command-group; rebase if second.

**The flag.** New on `test` only, off by default; not on `run`, whose flags the UI builds
(ui_command.go:287-298). Usage errors, before the AMI resolve or any AWS request: a scenario other
than aws-web-live; Layer 3 off; destruction disabled (a failed hold must destroy); with `--no-destroy`
(exclusive, as `--keep` is on run, run_command.go:1363-1367). An existing copy is refused by the guard
story's check, which runs first.

**Not SkipDestroy and not KeepSandbox.** SkipDestroy keeps a failed run's claim too
(aws_scope_lifecycle.go:272-279), and the aws arm never reads KeepSandbox (test_command.go:1016), so a
widened `--keep` would still destroy. `layer3LiveCloud` is not touched: deploy and live upgrade share it.

**The decision.** executeTest computes `holdWanted := opts.Hold && len(failures) == 0` beside
keepingSandbox, as late as `--keep`'s rule (test_command.go:996-1014). The finishTeardown closure
captures the run's ctx and passes both into awsScopeTeardown, the one teardown
(aws_scope_lifecycle.go:267-286). Inside, it reads the **run's** ctx, never the ctx finishTeardown hands
in (a fresh teardownContext, WithoutCancel, Err()==nil, reap_command.go:414-439):
1. holdWanted and run ctx not cancelled → write the copy with release's helper (same 0700/0600,
   same verify);
2. re-check the run's ctx after the copy; cancelled, or the copy failed → awsDestroyAndRelease,
   unchanged, with a stage naming the copy's failure or the interrupt;
3. only then the held stage.
Anything else is awsDestroyAndRelease, unchanged. Because the closure re-reads the run's ctx each call,
finishTeardown's rerun on a fresh context destroys rather than holds. A copy written before a
fall-through is kept and its path printed; the guard then refuses generate, run and test until it is
deleted.

**The held stage** is awsTeardownSkipped's held branch (StageAWSScopeClaimKept, skip,
aws_scope_lifecycle.go:291-311) with reason `--hold`: the holder, the copy path, nothing expires it,
and `reap <scenario> --take-over <holder> --release-hold`, with `--config` when passed. No account id
(aws_scope_lifecycle.go:219). The process then exits, so reap's live-holder refusal clears.

**Testing the hold-then-release chain.** The holder is `<run>@<this host>:<go test pid>`, which
refuseLiveLocalHolder (aws_scope.go:219-238) rightly refuses. Add a test-only seam in internal/cli
beside mintAWSClaimHolder that mints the hold's holder with the pid of a child process the test started
and waited for. harness is untouched and refuseLiveLocalHolder is never stubbed. A second test keeps
the default holder and shows `--release-hold` refused as a live holder.

**Unwritten rules it inherits (S156d):** the AWS teardown decision lives in awsScopeTeardown, never
in KeepSandbox; claim taken once and released only in awsReleaseAfterCleanSweep; keep only a passing,
uninterrupted iteration, read as late as possible; read the run's ctx, not the teardown's; a failed or
interrupted run reaches destroy, sweep and release on a fresh bounded context (#436/#438), and the
interrupt failure is still added (test_command.go:1161-1166); an operation having run is not one having
worked; never widen layer3LiveCloud; the UI's run path cannot reach the flag; non-Scaleway clouds print
their own stage, not a live/keep/TTL one (run_command.go:1022-1034); mutually exclusive flags refused as
usage errors; advice names only a flag the command has; no account id in stage detail or iteration.json.

Tip commit: `ADR: none — implements ADR-0040's hold amendment (aws-hold-for-read-adr-0040)`.

**Done when:**
- Against fakes, a passing `test aws-web-live --hold` ends with aws_scope_claim_kept (skip), no destroy
  call and no aws_scope_sweep or aws_scope_release, the claim held by the run's holder, and the copy
  (dir 0700, file 0600) equal to the output-dir state.
- The held detail names the holder, the copy path and
  `reap scenarios/training/aws-web-live.yaml --take-over <holder> --release-hold`, and does not contain
  the configured account id.
- While held: `test` of another aws scenario fails aws_scope_claim naming the hold's holder with no
  apply; plain `reap` (no --take-over) refuses naming the holder with nothing written; `test aws-web-live`
  is refused by the guard with no AWS request.
- Each of these reaches destroy, aws_scope_sweep and aws_scope_release with no held line: a failing
  real_probe; a copy that cannot be written (holds dir pre-made 0755), reported naming the copy; an
  interrupt fired from the last check's stub, so failures are empty and only the run's ctx decides; an
  interrupt fired from inside the copy write.
- Each refusal (other scenario, Layer 3 off, destruction disabled, with --no-destroy) is a usage error
  with zero SSM, EC2 and STS requests.
- Without --hold, and with --no-destroy, every existing test passes with no assertion line changed, and
  neither creates `.infrafactory/holds/`; a test shows registerRunFlags defines no `hold` flag.
- One test runs hold, deletes the output dir, then `reap --take-over <holder> --release-hold` (holder
  minted with a reaped child's pid), against fakes, and ends with destroy called on the restored state,
  the sweep empty and the claim released. A second, with the default holder, shows the release refused
  as a live holder.
- Each new test is shown failing by a cp-backup break of the code it guards, recorded in the PR.
