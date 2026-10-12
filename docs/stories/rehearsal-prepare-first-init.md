---
kind: code
status: blocked
blocked_by: [rehearsal-prepare-replace-env, rehearsal-prepare-copy, rehearsal-prepare-gate-aws-provider, rehearsal-prepare-gate-paths, rehearsal-prepare-gate-modules]
epic: rehearsal-prepare-gate
depends_on: [rehearsal-prepare-replace-env, rehearsal-prepare-copy, rehearsal-prepare-gate-aws-provider, rehearsal-prepare-gate-paths, rehearsal-prepare-gate-modules]
touches: [internal/cli/rehearse_prepare.go, internal/cli/rehearse_prepare_init.go, internal/cli/rehearse_prepare_init_test.go, internal/cli/rehearse_lock.go, internal/cli/rehearse_lock_test.go, internal/cli/rehearse_prepare_e2e_test.go, internal/cli/rehearsal_assumptions_test.go, internal/cli/testdata/golden/commands/, docs/decisions/0028-factual-claims-are-tests-not-comments.md, .github/fakeaws-e2e-tests]
risk: high
---

# The lock is read and kept, then the network-only first init runs under replace mode

Hot-file chain: after rehearsal-prepare-gate-aws-provider, before rehearsal-prepare-second-init.

**Lock.** Before gate 1, prepare parses the copy's root `.terraform.lock.hcl` (hclsyntax, never a text scan). A missing or unparseable lock, one without hashicorp/aws, or one naming a provider outside the eight the gate admits, is refused by name. A registry.terraform.io aws entry adds a warning that moved, import, removed and ephemeral semantics may differ. A version other than harness.AWSProviderVersion is named in the report as "a failure under it is a fakeaws gap", and the gate's static aws table still refuses any argument it does not name.

**Init 1.** After the full gate passes, `tofu providers lock -platform=<GOOS_GOARCH>` runs, then `tofu init -backend=false -input=false -no-color`. Both run through the injected runner with ReplaceEnv and Env exactly {HOME=<id>/home (empty, 0700), TF_DATA_DIR=<id>/data, GIT_CONFIG_NOSYSTEM=1, GIT_TERMINAL_PROMPT=0, GIT_ALLOW_PROTOCOL=file}. Gate 1 already refuses every module download, so git has nothing to fetch; the git variables are a second wall in case the gate is wrong. Prepare then refuses if the copy's locked aws version differs from the user's lock, and tightens modes (dirs 0700, executables 0700, other files 0600). A cancel or tofu error gives exit 1 with tofu's stderr, never a success report.

**Assumption tests (ADR-0028), each with its ADR-0028 row, in rehearsal_assumptions_test.go:**
- `TestAssumption_GitNoSystemDropsCredentialHelper` (not e2e-gated; needs git, fails if git is absent): with an empty HOME, `git config --show-scope --list` under GIT_CONFIG_NOSYSTEM=1 lists no `system` entry and no credential.helper. Where a system config exists (darwin's osxkeychain), a control run without the variable shows it; on a machine with none (CI's ubuntu) the control logs that it was vacuous. The row records that CI exercises only the linux case.
- `TestAssumption_GitAllowProtocolRefusesRemoteTransports` (not e2e-gated): under GIT_ALLOW_PROTOCOL=file, `git ls-remote` of an https and an ssh URL each fail with git's transport-not-allowed error, before any connection (the URLs point at a closed loopback port).
- `TestAssumption_ProvidersLockKeepsLockedVersion` (e2e-gated): `providers lock -platform` on a lock pinning an older aws version keeps that version.
- `TestAssumption_RegistryTerraformIoLockInitialises` (e2e-gated): a lock whose aws entry is registry.terraform.io initialises under tofu.

**E2E.** `TestRehearsePrepareFirstInit` in rehearse_prepare_e2e_test.go (e2e.SkipUnlessEnabled, real tofu): a temp git repo with a lock and a local module only, no registry module.

Add the three e2e-gated names to `.github/fakeaws-e2e-tests` (sorted). Tip: the ADR-0028 edit satisfies doc-hygiene.

**Done when:**
- A fake-runner test checks the commands are exactly [providers lock -platform=<GOOS_GOARCH>, init -backend=false -input=false -no-color], with ReplaceEnv and that exact Env key set. HOME is an empty dir, and TF_DATA_DIR lies outside `<id>/work`.
- A missing lock, an unparseable lock, a lock without aws and a lock naming hashicorp/http are each refused by name, with zero runner calls.
- A registry.terraform.io lock gives the warning in human and json output. With a 5.99.0 lock, the output names 5.99.0, and an argument outside the aws table is refused with the refusal naming 5.99.0.
- A fake runner whose `providers lock` call rewrites the aws version makes prepare refuse naming both versions.
- A fake runner returning context.Canceled exits 1 with no "prepared" output.
- The first-init e2e: after init 1 the aws provider is under `<id>/data` at the lock's version, modules.json names the local module, the user's lock bytes are unchanged, and the mode walk passes. The registry.terraform.io lock path also initialises.
- `grep -x` finds TestAssumption_ProvidersLockKeepsLockedVersion, TestAssumption_RegistryTerraformIoLockInitialises and TestRehearsePrepareFirstInit in `.github/fakeaws-e2e-tests`; ADR-0028's table names all four assumptions.
- Each new test is shown failing by a cp-backup break recorded in the PR.
