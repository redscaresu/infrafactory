---
kind: code
status: blocked
blocked_by: [rehearsal-passthrough-gated-run]
epic: rehearsal-passthrough
depends_on: [rehearsal-passthrough-gated-run]
touches: [docs/decisions/0002-cli-command-contract.md, docs/decisions/0042-rehearsal-command-contract-and-walls.md, internal/cli/command_golden_test.go, internal/cli/rehearse.go, internal/cli/rehearse_passthrough_args.go, internal/cli/rehearse_passthrough_cmd.go, internal/cli/root_test.go, internal/cli/testdata/golden/commands/rehearse_plan*.txt, internal/cli/testdata/golden/commands/rehearse_refresh*.txt, internal/cli/testdata/golden/commands/rehearse_state_rm*.txt]
risk: high
---

# rehearse plan|apply|destroy|import|state mv|console wired, golden files, ADR-0042 amended

rehearse.go:47 registers plan, apply, destroy, import, console (if kept) and a `state` group with only `mv`. The state group gets its own RunE that makes an unknown subcommand a usage error, since cobra exits 0 for one on a group. Each command is `<id> [-- tofu args]`. Args validators wrap cobra's in rehearseUsage. No command sets PersistentPreRun. runner, wd and notify are injected the way prepare does it (rehearse_prepare.go:22-41). Streams: cmd.OutOrStdout, ErrOrStderr, and InOrStdin for console. `--output json` and unknown actions are handled as in decisions. Golden cases and helpers are added beside goldenEndRoot. ADR-0042 gets a § Passthrough covering: the list, the `--` form, the allowlist, injected flags, the -out boundary, the single root, the fakeaws proof, the re-gate before every command, no timeout, the exit codes with the 2-vs-2 rule, and --output json refused. The L39-41 bullet is updated. ADR-0002 gets one amendment line pointing to it.

**Done when:**
- `go test ./internal/cli -run 'TestCommandGolden|TestRehearse' -v` passes with new goldens rehearse_plan (stub tofu output streamed), rehearse_plan_refused_var, rehearse_plan_output_json, rehearse_refresh and rehearse_state_rm. It also passes TestRehearsePassthroughRefusesUnlistedActions, which runs init, validate, refresh, output, show, taint, untaint, force-unlock, workspace, providers, get, graph, fmt, test, login, logout, version, state list|rm|pull|push|show|replace-provider (and console if refused); each must give ExitCodeForError 2 with the action named in the message. The existing TestRehearseSubcommandsSetNoPersistentPreRun walks the new commands, `state mv` included, unchanged. `bash scripts/check_doc_hygiene.sh` passes with the ADR-0042 edit on the tip commit.

**From review (binding; overrides the scope above where they differ):**
- done_when runs `-run 'TestCommandOutputGoldenSnapshots|TestRehearse'`; TestCommandGolden matches nothing.
- Add a `modes` field to the golden case struct so passthrough cases run in human mode only. Drop rehearse_plan_output_json, rehearse_plan_refused_var, rehearse_refresh and rehearse_state_rm as goldens, since the harness captures only stdout and they would be empty. Their cases move to TestRehearsePassthroughCLIRefusals, which runs the root on `plan <id> -- -var=x=y` (ExitCodeForError 1, message names -var, runner never called), `plan <id> --output json -- -out=p` (2, runner never called), `refresh <id>` and `state rm <id>` (2, message names the action and lists the allowed ones), and `state` alone (0, help).
- Args validators use cobra's ArgsLenAtDash, so `plan <id> -- -out=p` is accepted, and a test runs `<id> -- <args>` through cobra.
- A bounded helper-process test runs a registered cobra command, signals once, and asserts that the child receives exactly one SIGINT and the command reports exit 1 even when the child exits 0.
- ADR-0042 L43-44 (the family adds no exit code; every refusal exits 1) is rewritten to point at § Passthrough's exit codes, and a grep in the PR shows the old sentence is gone. § Passthrough records the 2-hour deadline and that apply takes no saved plan file.

The checks in the epic's **Checks before every stage** apply.
