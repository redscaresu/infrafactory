---
kind: code
status: ready
epic: rehearsal-prepare-gate
depends_on: []
touches: [internal/harness/static.go, internal/cli/exec_runner.go, internal/cli/exec_runner_test.go, internal/cli/assumptions_test.go, internal/cli/export_test.go]
risk: high
---

# harness.Command replace mode: PATH plus named variables only, honoured by the exec runner; runTofu folded onto it

Wave 1: it needs nothing from the command group, so it builds beside rehearsal-prepare-command-group.

**Field.** Add `ReplaceEnv bool` to harness.Command (static.go:12-27). When it is set, execCommandRunner.Run (exec_runner.go:245) builds the child env from the parent's PATH, then applies cmd.Env through withEnvOverrides, so a PATH in cmd.Env wins over the parent's. Nothing else is inherited. The builder always returns a non-nil slice: os/exec treats a nil Env as "inherit everything", and withEnvOverrides returns its base unchanged when overrides is empty (exec_runner.go:271-274), so an unset PATH and an empty Env must still yield an empty, non-nil slice. ReplaceEnv together with a non-empty StripEnv is an error, returned before exec. The process group, single SIGINT, kill fallback and signal guards stay as they are. The field ships off: when it is false the env is byte-identical to today's.

**runTofu.** Fold the test-only runTofu (assumptions_test.go:267-290) onto the production runner, so there is one os/exec path for tofu. runTofu becomes a wrapper that calls execCommandRunner.Run with ReplaceEnv set and a tofuDeadline context, and keeps the RunTofu export (export_test.go:55-65). At the deadline tofu now gets SIGINT, then SIGKILL after cancelKillFallback.

**Test child.** The env-printing child is the test-binary helper (exec_runner_test.go:150-156) printing os.Environ(), or /usr/bin/env exec'd directly; never `sh -c`, which adds PWD, SHLVL and _. The group-interrupt re-run passes INFRAFACTORY_EXEC_HELPER_READY through Env, since replace mode no longer inherits it (exec_runner_test.go:203,238-241).

None of this story's tests is gated by e2e.SkipUnlessEnabled; they run in `make test`, so `.github/fakeaws-e2e-tests` is not touched.

Tip: `ADR: none — an off-by-default runner mechanism; ADR-0042 (rehearsal-prepare-command-group) records its use`.

**Done when:**
- A real-child test plants these in the parent: HOME, TF_CLI_CONFIG_FILE, TF_REATTACH_PROVIDERS, TF_LOG_PATH, TF_PLUGIN_CACHE_DIR, TF_CLI_ARGS, TF_CLI_ARGS_init, TF_VAR_region, TF_ENCRYPTION, TF_TOKEN_app_terraform_io, AWS_IGNORE_CONFIGURED_ENDPOINT_URLS, AWS_SESSION_TOKEN, AWS_PROFILE, SSH_AUTH_SOCK, GIT_CONFIG_GLOBAL, HTTPS_PROXY and NO_PROXY. The child's printed env is exactly PATH plus the Env keys; any planted or extra key fails the test.
- With ReplaceEnv, an empty Env and PATH removed from the parent, the real child prints an empty environment. A cp-backup break making the builder return nil turns it red.
- With ReplaceEnv and a PATH in Env different from the parent's, the child sees Env's PATH.
- With ReplaceEnv false, the child env equals the parent minus GCP auth and StripEnv, plus Env, byte for byte; the existing exec_runner tests pass unchanged.
- ReplaceEnv plus a non-empty StripEnv returns an error and starts no process.
- A ReplaceEnv command runs in its own process group and receives exactly one SIGINT on cancel (the group-interrupt test re-run with ReplaceEnv).
- runTofu, pointed at a stub `tofu` that prints its env (on a temp PATH set with t.Setenv), prints exactly its env slice, with a planted HOME and TF_CLI_ARGS absent. A cp-backup break dropping ReplaceEnv from the wrapper turns it red.
- assumptions_test.go no longer calls exec.Command; the rehearsal assumption tests pass with e2e enabled.
- Each new test is shown failing by a cp-backup break recorded in the PR.
