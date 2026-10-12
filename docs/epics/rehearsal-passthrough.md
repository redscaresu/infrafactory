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

**Constraints:** no new os/exec path; exit codes follow ADR-0002 and ADR-0042, with tofu's own exit code passed through unchanged, and ADR-0042's passthrough amendment says how `plan -detailed-exitcode`'s 2 (changes present) is told apart from a usage error's 2; passthrough refuses `-var` and `-var-file` by name, passes prepare's copied var file and no other, and reuses replace mode (ADR-0042).

**Your part:** none.

**Areas:** `internal/cli/ (exec_runner.go streaming mode, rehearse_passthrough.go new, golden files)`; `docs/decisions/0042`; `docs/operations.md`
