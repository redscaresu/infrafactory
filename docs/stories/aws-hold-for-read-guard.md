---
kind: code
status: blocked
blocked_by: [aws-hold-for-read-adr-0040, aws-hold-for-read-release]
epic: aws-hold-for-read
depends_on: [aws-hold-for-read-adr-0040, aws-hold-for-read-release]
touches: [internal/cli/aws_hold.go, internal/cli/aws_hold_test.go, internal/cli/generate_command.go, internal/cli/run_command.go, internal/cli/test_command.go]
risk: high
---

# While a hold's copy exists, generate, run and test of that scenario are refused before they touch its output dir

While held, the stack's only live state is `./output/aws-web-live/terraform-live.tfstate`
(runtime.go:200). Generation removes the output dir (generate_command.go:1033), and `test` runs tofu
in it. One `run` or `generate` during a hold deletes the state: plain reap then reads the absent state
as nothing applied (test_command.go:1208-1214) and leans on the sweep alone. Lands before the hold, so
no hold can exist without this guard.

**The guard**, one function in internal/cli/aws_hold.go using release's `.infrafactory/holds` const:
when anything exists at `.infrafactory/holds/<scenario name>/terraform-live.tfstate` (Lstat, so a broken
symlink counts), or that dir cannot be read, refuse. A local check only, no AWS request. The error
names the copy's path, the release (`reap <scenario> --take-over <holder> --release-hold`), and that
the copy is deleted after the read, which lifts the guard. An unreadable holds dir refuses (an absent
input is not an empty input).

Called once where generate, run and test converge before the output dir is reset or written. The
builder greps every caller of writeGeneratedFiles (both write modes) and of the test path's tofu work
in the output dir; with no single convergence point, one call at each command's entry, before AMI
resolve, the Layer 2 mocks or any AWS request. `reap`, `rehearse read` and the `--release-hold` path
are not guarded: they do not regenerate. The UI's run path (ui_command.go:287-298) goes through the
same `run` code, so it is refused too.

**Unwritten rules it inherits (S156d):** generation removes the output dir, so state that must
survive cannot live only there; an absent input is not an empty input; usage refusals come before any
AWS request; the UI's run path cannot reach around a CLI rule; advice names only a flag that exists
(--release-hold has merged); no account id in the message.

Tip commit: `ADR: none — implements ADR-0040's hold amendment (aws-hold-for-read-adr-0040)`.

**Done when:**
- With a copy planted at `.infrafactory/holds/aws-web-live/terraform-live.tfstate`, each of
  `generate`, `run`, `test` and a run started through the UI's run request on aws-web-live exits
  non-zero naming the copy and `--release-hold`, with zero SSM, EC2 and STS requests, no LLM call, and
  the output dir byte-identical before and after (every file hashed).
- The same holds with the planted entry a broken symlink, and with the holds dir mode 0000.
- With no copy, and with a copy planted for another scenario name, every existing generate, run and
  test test passes with no assertion line changed.
- After the planted copy is deleted, `test aws-web-live` against fakes proceeds.
- Each new test is shown failing by a cp-backup break of the code it guards, recorded in the PR.
