---
status: active
hld: 2026-09-30-import-production-state-into-the-mocks
depends_on: [rehearsal-walls]
---

# The user runs tofu against the private fakeaws through infrafactory with live output: `rehearse plan|apply|destroy|import|state mv|console`.

**Done when:**
- The runner gains a streaming/stdin mode on the same Setpgid/SIGINT/single-interrupt path (today it is batch-only, exec_runner.go): output is live, stdin reaches `console`.
- `rehearse plan|apply|destroy|import|state mv|console` run with the work directory as cwd, under the walls' environment and overrides, `-out` bound inside it; one Ctrl-C stops tofu cleanly and nothing reports success after an interrupt. If `console` proves too wide, it is refused by name and the HLD's list amended (decided in the first story).
- Any tofu action not in the list is refused by name.
- ADR-0042 amended; golden files; docs/operations.md.

**Out of scope:** discovery, read, load, check, reset (rehearsal-against-the-copy); the UI.

**Constraints:** no new os/exec path; exit codes follow ADR-0002 and ADR-0042, with tofu's own exit code passed through unchanged, and passthrough amends ADR-0042 to say how `plan -detailed-exitcode`'s 2 (changes present) is told apart from a usage error's 2; passthrough refuses `-var` and `-var-file` by name, passes prepare's copied var file and no other, and reuses replace mode (ADR-0042); re-runs the gate over the copy before every tofu command, admitting infrafactory's own override files only when byte-identical to what infrafactory writes (ADR-0042).

**Your part:** none.

**Areas:** `internal/cli/ (exec_runner.go streaming mode, rehearse_passthrough.go new, golden files)`; `docs/decisions/0042`; `docs/operations.md`

**Checks before every stage:**
- `go vet ./...` and `go test ./internal/cli/... ./internal/harness/... ./cmd/...` pass. Existing tests pass unchanged, in particular exec_runner_test.go's signal and env cases, TestRehearseSubcommandsSetNoPersistentPreRun, TestRehearseUsageErrorsExitTwo, rehearse_walls_env_test.go's exact key set, and contract_test.go's usage gives 2.
- Once rehearsal-passthrough-exec-paths has merged, TestNoNewExecPath passes: no story adds an os/exec importer or an exec call site.
- Each new test is shown failing by a cp-backup break of the code it covers (never git checkout), with the FAIL line in the PR.
- A test that execs a script uses writeScript (#569) or the helper process in exec_runner_test.go.
- `make doc-hygiene` passes. The tip commit carries the ADR edit the story names, or `ADR: none — <reason>`.
- No real cloud and no AWS credentials: tofu runs only against the rehearsal's fakeaws or a local root.
- Every PR gets the codex review loop.

**Design calls (from scoping, 2026-10-16; the stricter call on each):**
- **console / stdin** (rehearsal-passthrough-runner-stream): Spike console over a pipe only. A pty is refused outright: it needs a new dependency, raw terminal mode and a session leader, and those break the Setpgid single-interrupt path (exec_runner.go:238-268). Console stays only if all three hold: tofu console answers each piped line before EOF; `console -input=false` with an unset required variable errors and does not read stdin; Run returns once the child exits even with stdin still open. Otherwise console is refused by name and there is no Stdin field.
- **stdout/stderr** (rehearsal-passthrough-runner-stream): Optional Stdout and Stderr writers on harness.Command (nil keeps today's buffering, static.go:12-35), run through the same exec.CommandContext. Production passes cmd.OutOrStdout()/ErrOrStderr(), which are *os.File, so no copy goroutine is added. Batch mode already uses buffers, so this adds no new hang case (exec_runner.go:272-275).
- **flags** (rehearsal-passthrough-args): Tofu arguments go after `--` (`rehearse plan <id> -- -out=p`). Cobra keeps parsing infrafactory's own flags, so the family's usage rules (ADR-0042 L21-34, rehearse.go:30-46) hold without a hand-written parser. DisableFlagParsing is rejected. Each action has its own allowlist of tofu flags; any other flag is refused by name. The user's -input is refused, and -input=false is injected.
- **unknown action** (rehearsal-passthrough-commands): Stays a usage error, exit 2 (ADR-0042 L21-23). The message names the action and lists the allowed ones, and the same holds under `state` (`state rm`, `state pull`). `rehearse state` with nothing after it prints help and exits 0, the same as the group. A refused tofu flag or path is command_failed, exit 1 (ADR-0042 L43-44).
- **--output** (rehearsal-passthrough-commands): `--output json` on a passthrough action is a usage error, exit 2, raised before tofu runs. In human mode nothing is printed after tofu exits.
- **timeout** (rehearsal-passthrough-gated-run): A constant rehearsalPassthroughTimeout of 2 hours (AGENTS.md:152), not rehearsalTofuTimeout. One Ctrl-C (or SIGTERM/SIGHUP) cancels: tofu gets one SIGINT, then SIGKILL after cancelKillFallback. rehearsalTofuTimeout is not used (rehearse_prepare_init.go:20-22).
- **exit codes** (rehearsal-passthrough-exit-code (carrier), rehearsal-passthrough-gated-run (mapping), rehearsal-passthrough-commands (ADR text)): Tofu's own code passes through when tofu ran to completion. An interrupt, an exit code of -1, or a failure to tighten modes afterwards gives exit 1, whatever tofu returned. On 2 vs 2: infrafactory's 2 is a usage error decided from argv alone, before tofu starts, with a usage message and no tofu output, so a command line that ran once can never produce it again. A 2 after tofu ran is tofu's.
- **root** (rehearsal-passthrough-gated-run): Runs in the record's only root. A record with more than one root is refused and the roots are named. No --root flag until rehearsal-against-the-copy needs one (rehearse_walls.go:37-47).
- **Review changes:** tofu runs under a 2-hour deadline (AGENTS.md:152), not none. apply takes no saved plan file in this epic: the re-gate cannot inspect the configuration inside one, so apply runs from source with -auto-approve. A record must have exactly one root.
