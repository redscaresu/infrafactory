# Codex review pass 34 — S166+S167 cutover

`codex exec review --base main` on `s166-cutover` at `03601e3`.

One finding, accepted.

## [P1] `run`'s auto-destroy built its environment without the run project

The seventh path, and the same defect pass 33 fixed in `live teardown` and
`reap`: `sandboxCommandEnv(runtime)` returns an env with no
`SCW_DEFAULT_PROJECT_ID`, so the auto-destroy ran against the shared fallback
while the apply had run in the run's own project.

## Why it kept recurring, and what changed

`internal/cli` already had an audit test for exactly this
(`TestPipelineNeverBuildsSandboxEnvWithoutTheRunProject`). **It read
`test_command.go` and nothing else**, so it could not see `run_command.go`,
`live_teardown.go` or `reap_command.go` — the three files the defect actually
landed in, one review pass each.

Two changes, because the audit alone had already failed once:

1. **The helper no longer returns an environment.** `sandboxCommandEnv` became
   `assertSandboxCredentials(runtime) error` — it checks that Layer 3 can run at
   all, before the project exists, and hands back nothing usable. Every caller
   that needs an env must now name a project. The old name read like "the env
   for a sandbox command", which is what invited three call sites to take it.
   This is the fix; the compiler enforces it.
2. **The audit reads every `internal/cli` source file** and now checks for the
   one thing the type system cannot: a call site passing `""` explicitly. It
   fails if it ever finds fewer than two files to read, so it cannot quietly
   narrow again. Verified against synthetic drift — reintroducing the exact
   defect fails it, naming the file and line.

## Nothing declined this pass.
