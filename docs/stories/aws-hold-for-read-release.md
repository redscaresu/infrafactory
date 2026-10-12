---
kind: code
status: ready
epic: aws-hold-for-read
depends_on: [aws-hold-for-read-adr-0040]
touches: [internal/cli/root.go, internal/cli/reap_command.go, internal/cli/aws_hold.go, internal/cli/aws_hold_test.go, internal/cli/aws_reap_command_test.go]
risk: high
---

# `reap --take-over <holder> --release-hold` copies the pre-destroy tfstate under .infrafactory/holds/ before it destroys, and refuses when it cannot

Goes before the hold, so the hold's advice names a flag that exists (root.go:219-222). Hot file:
internal/cli/root.go is also touched by rehearsal-prepare-command-group; rebase if second.

**The flag.** New on `reap`, off by default. Usage errors, before any AWS request: a non-aws
scenario, a scenario other than aws-web-live, no `--take-over`, with `--dry-run`.

**The copy, local work before credentials, stamp or claim**, in a new helper in internal/cli/aws_hold.go
that the hold reuses:
- The root is a const `.infrafactory/holds`, defined like runstore.DefaultRoot (runstore.go:16) and
  livestore.DefaultRoot (livestore.go:40), derived from no env var and not from resolveRunStoreRoot
  (run_command.go:1146-1150, which follows INFRAFACTORY_RUNSTORE_ROOT). The copy is
  `.infrafactory/holds/<scenario>/terraform-live.tfstate`.
- MkdirAll 0700, then Lstat `holds/` and `holds/<scenario>/`: a symlink, a non-directory or any
  group/other bit refuses. Never chmod (the repo's MkdirAll is 0o755 everywhere).
- **No copy yet:** the source is runtime.OutputDir()'s terraform-live.tfstate. Missing or unreadable
  refuses (an absent input is not an empty input, test_command.go:1193-1215). Write it 0600 through a
  temp file in the same dir, fsync, rename, then re-read and compare to the source; a mismatch refuses
  (an operation having run is not an operation having worked).
- **A copy exists:** Lstat it first. Anything but a regular file, mode exactly 0600, link count 1 is
  refused and left as it was. Then: output state byte-identical → keep and proceed; output state
  missing → restore it from the copy (the copy is the authority, never the other way round) and
  proceed; output state present and different → refuse, both files left as they were.
- Every refusal exits non-zero and names plain `reap <scenario> --take-over <holder>` as the way to
  destroy without the copy.

Then runAWSReap unchanged. The order is copy, destroy, sweep and reap, release. Output adds the copy's
path and that it is kept for the read (the operator deletes it after; the guard story refuses
generate, run and test while it exists).

**Unwritten rules it inherits (S156d):** credentials then stamp before any claim read or write
(reap_command.go:159-169); take-over only by name, refusing a holder alive on this host
(aws_scope.go:219-238); a failed destroy or delete still reaches the verdict sweep
(reap_command.go:251-294); non-zero unless proved empty (reap_command.go:191-201); ReleaseAWSClaim
only in awsReleaseAfterCleanSweep (aws_scope_lifecycle_test.go:823); an absent input is not an empty
input; an operation having run is not one having worked; explicit 0700/0600 modes, refuse rather than
chmod, no symlinks; nothing the UI serves holds state; advice names only a flag the command has; no
account id in anything it adds.

Tip commit: `ADR: none — implements ADR-0040's hold amendment (aws-hold-for-read-adr-0040)`.

**Done when:**
- Carried from #476's review: a `--release-hold` that destroys and then fails its sweep leaves an output state that differs from the copy, so a retry is refused. That refusal names the plain `reap --take-over <holder>` exit with the reap's own holder, and a test proves it.
- Against fakes, the stubbed SandboxDestroy asserts on call that the copy exists, equals the
  output-dir state, and has dir 0700 and file 0600; the recorded order is copy, destroy, sweep,
  release, and the claim ends released.
- Each refusal exits non-zero with zero SSM, EC2 and STS requests and no destroy call, names plain
  `reap ... --take-over <holder>`, and leaves any existing copy byte-identical: non-aws scenario,
  another aws scenario, no --take-over, --dry-run, state missing with no copy, state mode 0000,
  holds dir pre-made 0755, holds dir a symlink, existing copy differing from a present output state,
  existing copy a symlink to the output-dir state, existing copy mode 0644, existing copy hard-linked.
- An existing byte-identical copy is kept and the release proceeds.
- With the output dir deleted after the copy was written, the release restores the state from the
  copy, the stubbed SandboxDestroy sees the copy's bytes in the output dir, and sweep and release follow.
- With INFRAFACTORY_RUNSTORE_ROOT set to a non-default value, the copy still lands under
  `.infrafactory/holds/`; a test asserts the path is under neither runstore.DefaultRoot nor
  livestore.DefaultRoot, and `git check-ignore` matches it.
- Without the flag, every existing reap test passes with no assertion line changed, and a test shows
  no `.infrafactory/holds/` is created.
- Each new test is shown failing by a cp-backup break of the code it guards, recorded in the PR.
