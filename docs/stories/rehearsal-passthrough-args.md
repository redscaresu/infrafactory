---
kind: code
status: ready
epic: rehearsal-passthrough
depends_on: []
touches: [internal/cli/rehearse_passthrough_args.go, internal/cli/rehearse_passthrough_args_test.go]
risk: high
---

# One pure function turns a passthrough action and the user's tofu args into tofu's argv, or a refusal by name

New rehearse_passthrough_args.go. Input: the action, the user's args after `--`, the work dir and the rehearsal dir. Each action has an allowlist of flags, each marked bool or value, accepted as -x, --x, -x=v or -x v. Anything else is refused by name, including -var, -var-file, -input, -state, -state-out, -backup, -chdir, -plugin-dir, -backend-config and -generate-config-out. A `--` inside the args is refused. `plan -out` and apply's plan-file positional must resolve inside the work dir (rehearsalResolvesInside, rehearse_walls_regate.go:110). Positional counts: import 2, state mv 2, apply at most 1, the rest 0. Injected before the user's args: -input=false on plan, apply, destroy, import and console; -var-file=<id>/vars.tfvars (rehearsalVarFile), only when that file exists, on plan, apply without a plan file, destroy, import and console; nothing on state mv. rehearsal-passthrough-commands drops the console row if console was refused. Tip commit: `ADR: none — no caller until rehearsal-passthrough-commands`.

**Done when:**
- `go test ./internal/cli -run TestRehearsePassthroughArgs -v` passes. Each case asserts either the exact argv or the refused flag's name in the error. Refused: -var x=y, -var=x=y, --var=x=y, -var-file=f, -input=true, -state=s, -chdir=d, an unknown -foo, a `--` inside the args, -out=../../x, -out=/abs/p, and -out=link/p where link points outside. Accepted: -out=p and -out p, with the exact argv. Also: apply with a plan file inside the work dir gets no -var-file; with vars.tfvars absent nothing gets -var-file; state mv's argv is exactly the user's args.

**From review (binding; overrides the scope above where they differ):**
- Relative -out and apply plan-file paths are joined onto the root dir (tofu's cwd), which is passed in as such, never onto <id>/work.
- -out is checked with rehearsalResolvesInside on filepath.Dir(out) against the root's boundary, plus os.Lstat on the leaf: an absent file or a regular file passes, and a symlink or anything else is refused.
- An -out leaf is refused when rehearsalEntryProblem refuses it, when it ends in .tf, .tf.json, .tfvars or .tfvars.json, or when it is a name in ours() (infrafactory's override files). Refused cases include -out=main.tf, -out=x.auto.tfvars and -out=infrafactory_override.tf.
- Apply takes no plan-file positional in this epic and refuses one by name: the re-gate checks the current source, not the configuration inside a saved plan, so a copied-in plan could run configuration the gate never admitted (ADR-0042 L180). Apply runs from source with -auto-approve. Admitting plans that infrafactory itself wrote is a later story.
- Each action's allowlist is written out in full in the scope comment and the test table. Also refused: -json-into=x on plan, apply, import and state mv; import -config=d; -out=p where p is a symlink pointing outside.

The checks in the epic's **Checks before every stage** apply.
