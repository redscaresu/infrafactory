---
kind: code
status: blocked
blocked_by: [rehearsal-prepare-command-group, rehearsal-prepare-gate-core, rehearsal-prepare-gate-modules]
epic: rehearsal-prepare-gate
depends_on: [rehearsal-prepare-command-group, rehearsal-prepare-gate-core, rehearsal-prepare-gate-modules]
touches: [internal/cli/rehearse.go, internal/cli/rehearse_prepare.go, internal/cli/rehearse_prepare_copy.go, internal/cli/rehearse_prepare_test.go, internal/cli/command_golden_test.go, internal/cli/testdata/golden/commands/]
risk: high
---

# `rehearse prepare <dir>`: flags, the rehearsal directory, the copy with its local module ancestor, gate 1, no tofu

**Command.** rehearse_prepare.go adds `prepare <dir> --account <12 digits> --region <aws region> [--var-file f]`. There is no `--allow-module`: every non-local module is refused (ADR-0042). It has its own Args validator, and its usage errors are CLIError usage (exit 2). It reads no infrafactory.yaml, and its runner is an injected harness.CommandRunner (default execCommandRunner{}).

**Copy.** The repository is the nearest ancestor holding `.git` (a dir or a worktree file); outside one, prepare refuses. It follows module sources recursively with rehearsal-prepare-gate-modules' `rehearsalLocalModuleSources`; a non-local or non-literal source, or one resolving outside the repo (Abs plus EvalSymlinks), is refused by name before anything is copied. It copies the nearest common ancestor of the root and its modules into `<id>/work/`, excluding .terraform/, .git/, .infrafactory/ and *.tfstate*; a symlink anywhere in the copied tree is refused. Dirs are 0700 and files 0600. `.infrafactory/rehearsals/` is created at the repository root, 0700, with a `.gitignore` of `*`; a symlinked `.infrafactory` or `rehearsals` is refused. The id is new, and an existing id is refused. --var-file is copied to `<id>/vars.tfvars` (outside work/, so tofu never auto-loads it) at 0600; a `.json` var file is refused. Gate 1 then runs on the copy over the root and every local module dir. On a refusal the half-made `<id>` dir is removed and the exit is 1. Prepare outputs human or json (id, work dir). This story runs no tofu. Add the golden case and files.

Tip: `ADR: none — ADR-0042 records prepare`.

**Done when:**
- A table test: a missing --account, an 11-digit account, a missing --region, a bad region, two dirs and an `--allow-module` flag each exit 2 with a usage error.
- A repo fixture with root envs/prod and ../../modules/x copies both under the ancestor. A stat walk shows every dir 0700 and every file 0600. rehearsals/.gitignore reads `*`. .terraform/, .git/, .infrafactory/ and terraform.tfstate are absent from the copy, and the source tree hashes the same before and after.
- A source escaping the repo, a symlink in the tree, a symlinked .infrafactory/rehearsals and a `.json` var file are each refused naming the path, and no `<id>` dir remains.
- Each of these is refused, and the fake runner records zero calls: encryption in a .tf, a .tofu file, a .tf.json file, `cloud {}`, a `git::` source, a registry source with an exact version, an override file and a `terraform.d/` dir.
- Golden human and json files exist for a refusal and a success.
- Each new test is shown failing by a cp-backup break recorded in the PR.
