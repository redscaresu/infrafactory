---
kind: code
status: ready
epic: rehearsal-prepare-gate
depends_on: []
touches: [internal/cli/rehearse.go, internal/cli/root.go, internal/cli/root_test.go, docs/decisions/0042-rehearsal-command-contract-and-walls.md, docs/decisions/README.md, docs/stories/prod-read-seam-rehearse-read-cli.md, docs/epics/rehearsal-walls.md, docs/epics/rehearsal-passthrough.md]
risk: high
---

# The `infrafactory rehearse` command group and ADR-0042, the rehearsal command contract and its walls

Every other `rehearse *` story waits for this one.

**Command.** New `rehearse.go`: a parent `rehearse` command in the style of newMockCmd (root.go:278-313), registered by one more entry in NewRootCmd's AddCommand list (root.go:101-113). It reads no infrafactory.yaml. Cobra hides a non-runnable parent with no subcommands from `infrafactory --help` until `prepare` lands, so the root test uses `root.Find` or executes `rehearse` directly, never root help (root_test.go:15-31). contract.go is not touched: a refusal is command_failed (exit 1), a usage error is exit 2, and no error code is added (contract.go:72-83).

**ADR-0042, "the rehearsal command contract and its walls".** It states decisions. Every fact about tofu or git that a decision rests on cites a `TestAssumption_*` test (ADR-0028) that already exists or that a named story in this epic adds; a reason no decision rests on is worded as an unverified reason. It covers:
- The family: `prepare` here; `read` cited to ADR-0041, not redefined; `end`, passthrough and the loop reserved to their epics. ADR-0002 exits: usage 2, any refusal command_failed 1.
- The layout: `.infrafactory/rehearsals/<id>/` under the cwd; dir 0700, files 0600; `.gitignore` of `*` in rehearsals/; subdirs work/ data/ home/ state/; never under runs/ or live/. After each tofu run modes are tightened: dirs 0700, executables 0700 (provider binaries), other files 0600.
- The walls: the gate and the two inits here; the provider override, the environment and the backend override in rehearsal-walls. The replace-mode env (rehearsal-prepare-replace-env): PATH plus named variables, nothing inherited, so TF_ENCRYPTION, TF_VAR_*, TF_CLI_ARGS* and the rest never pass.
- The gate bar: an allowlist; unparseable means refused; nothing is skipped; no tofu runs for a refused configuration.
- Decision 1, encryption is closed whatever its source: the `encryption` block is refused by the terraform{} allowlist; TF_ENCRYPTION never passes replace mode; `.tofu`, `.tofu.json`, `.tf.json`, `.tfvars.json`, `.tftest.*`, `.tofutest.*` and `.tfmock.hcl` are refused by name, never parsed or skipped.
- Decision 2, module sources are judged by what the download does, and every download is refused: only local `./` and `../` sources inside the repository are admitted; every registry, git, SSH, HTTP, bucket or other-host source is refused, registry modules included, because a registry download runs git against github.com. No `--allow-module` flag exists. Gate 2 admits only the root and local entries of modules.json, so nothing fetched is ever admitted, and there is no nested-fetch bound to record. Init 1 also carries GIT_CONFIG_NOSYSTEM=1, GIT_TERMINAL_PROMPT=0 and GIT_ALLOW_PROTOCOL=file as a second wall (claims pinned in rehearsal-prepare-first-init).
- The extension and file policy: per gated module dir, `.tf` and `.tfvars` are parsed; the suffixes above are refused; user override files are refused; a `terraform.d` entry is refused (tofu's implied local provider mirror); other files are data tofu does not load (pinned by `TestAssumption_LoaderIgnoresUnknownSuffixes`).
- Providers: hashicorp/{aws,random,null,tls,time,cloudinit,local,archive} plus terraform_data; a required_providers local name must equal its source's type; each non-aws provider admits a closed list of resource and data types and only `alias` in its block; functions are the pure-function allowlist (layer3PureFunctions, read only) plus the path-checked file functions, and any name containing `::` is refused.
- provider "aws": a static argument table tied to 5.100.0 by a drift test; an argument the table does not name is refused under any lock version; prepare makes no `tofu providers schema -json` call.
- Goal 11, static: every root provider "aws" block, aliases included, carries a `region` that is a literal or var.x resolved by tofu's precedence (pinned by `TestAssumption_VariablePrecedenceMatchesGate`) and equals `--region`; an absent region is refused; a resource-level `region` gets the same rule.
- The lock: a missing, unparseable or aws-less `.terraform.lock.hcl` is refused; the provider version is always the lock's; `providers lock -platform` always runs and the locked aws version must be unchanged after it; a registry.terraform.io lock warns; a non-5.100.0 version is named in the report so a failure is filed as a fakeaws story.
- Variables: `--var-file` at prepare is the only variable source for every later rehearse step.
- Tofu claims cited: `TestAssumption_UserOverrideMergesAfterOurs`, `TestAssumption_RootOverrideMissesChildModuleProvider`, `TestAssumption_SecondInitMakesNoProxiedConnection`, `TestAssumption_ExternalKeyProviderRunsAtBackendInit` (existing), and the new ones each story below names.

Run `make adr-index`.

**Cross-epic obligations.** Add to the **Constraints** line of `docs/epics/rehearsal-passthrough.md`: passthrough refuses `-var` and `-var-file` by name, passes prepare's copied var file and no other, and reuses replace mode (ADR-0042). Add to the **Constraints** line of `docs/epics/rehearsal-walls.md`: the environment sets AWS_REGION from `--region` and sets no other region variable, with a test; the "every file is 0600" test admits 0700 for executables tofu writes (ADR-0042); walls reuses rehearsal-prepare-replace-env's ReplaceEnv and rehearsal-prepare-second-init's rehearsalDeadProxy.

**read-cli.** In `docs/stories/prod-read-seam-rehearse-read-cli.md`, re-point blocked_by and depends_on from rehearsal-prepare-gate to rehearsal-prepare-command-group and drop its "not yet planned" paragraph. Add to its body: it merges main in before building; its rehearse.go edit is one AddCommand entry; and if docs/operations.md has no `## Rehearsal` section yet, it creates one and puts its lines there.

Tip: this PR edits ADR-0042, which satisfies doc-hygiene.

**Done when:**
- A root test, using root.Find or executing `rehearse` directly, asserts `rehearse` is registered and that `infrafactory rehearse` with no subcommand exits 0 printing help naming the group. A cp-backup break removing the AddCommand entry turns it red, recorded in the PR.
- TestCommandOutputGoldenSnapshots passes without UPDATE_GOLDEN=1 (a regression guard: no existing command's output changes).
- docs/decisions/0042-rehearsal-command-contract-and-walls.md names each item above; docs/decisions/README.md lists it; `make doc-hygiene` passes.
- Every `TestAssumption_*` name in ADR-0042 either exists on main (`grep -rn 'func TestAssumption_<name>' internal/`) or appears in the **Done when** of a story file with `epic: rehearsal-prepare-gate`; a loop in the PR checks each name.
- `grep -E '^(blocked_by|depends_on):' docs/stories/prod-read-seam-rehearse-read-cli.md` names rehearsal-prepare-command-group and not rehearsal-prepare-gate, and its body carries the merge-main and `## Rehearsal` lines.
- `grep -n -- '-var-file' docs/epics/rehearsal-passthrough.md`, `grep -n 'AWS_REGION' docs/epics/rehearsal-walls.md` and `grep -n '0700' docs/epics/rehearsal-walls.md` each hit a line under **Constraints**.
