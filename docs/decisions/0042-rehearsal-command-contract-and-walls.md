# ADR-0042: The Rehearsal Command Contract and Its Walls

## Status
Accepted

## Context

`docs/hld/2026-09-30-import-production-state-into-the-mocks.md` adds a command family that runs
the user's own OpenTofu code, not generated code, on the user's machine, next to a credential that
can read prod. That code is untrusted input to tofu. This ADR records the family's public contract
and the walls that keep the code off prod, before any `rehearse` subcommand lands.

Every fact about tofu or git that a decision below rests on cites a `TestAssumption_*` test
(ADR-0028): one that exists on main, or one a named story of epic rehearsal-prepare-gate adds. A
reason no decision rests on is marked **unverified**: the decision holds whether or not it is true.

## Decision

### The family

`infrafactory rehearse` is a command group. It has no action of its own: with no subcommand it
prints its help and exits 0, and it reads no `infrafactory.yaml`, because a rehearsal takes its
account and region from flags.

- `rehearse prepare <dir> --account <id> --region <r> [--var-file f]` is decided here.
- `rehearse read` is defined by ADR-0041, not redefined here.
- `rehearse end` is reserved to epic rehearsal-walls; `plan|apply|destroy|import|state mv|console`
  passthrough to rehearsal-passthrough; discover, load, check and the base-then-head loop to
  rehearsal-against-the-copy. Each amends this ADR when it lands.

Exit codes are ADR-0002's: a usage error is 2, and every refusal is `command_failed`, exit 1. The
family adds no error code.

### The layout

A rehearsal lives in `.infrafactory/rehearsals/<id>/` under the current directory, never under
`.infrafactory/runs/` or `.infrafactory/live/`, which the UI serves. `rehearsals/` holds a
`.gitignore` of `*`. Each `<id>/` has `work/` (the copied code), `data/` (`TF_DATA_DIR`), `home/`
(an empty `HOME`) and `state/` (the state file, outside the copy). Directories are 0700 and files
0600. Tofu writes files with its own modes, so after each tofu run the modes are tightened again:
directories 0700, executables 0700 (provider binaries must stay runnable), other files 0600.

### The walls

Three walls stand between the user's code and prod:

1. **The gate** (this epic): the copy is parsed and refused before any tofu runs.
2. **The two inits** (this epic): a network-only first init, the gate again over every module it
   resolved, then an offline second init under a dead proxy.
3. **The provider override, the environment and the backend override** (rehearsal-walls), so a
   bare `tofu` in the work directory reaches only this rehearsal's fakeaws.

Every tofu command runs in **replace mode** (story rehearsal-prepare-replace-env): the child's
environment is `PATH` plus the variables the step names, and nothing else is inherited. So
`TF_ENCRYPTION`, `TF_VAR_*`, `TF_CLI_ARGS*`, `TF_CLI_CONFIG_FILE`, `TF_PLUGIN_CACHE_DIR`, the AWS
credential and profile variables, and anything else in the user's shell never reach tofu.

### The gate bar

ADR-0023's bar, applied to user code: the gate is an allowlist, so what it does not name is
refused; it parses, never scans text; an unparseable file is refused; nothing is skipped; and no
tofu command runs for a refused configuration.

### Decision 1: encryption is closed whatever its source

The `encryption` block is refused because `terraform {}` admits only `required_providers`,
`required_version`, and `backend` in the root; `cloud` is refused by name too. An `external` key
provider runs its command at a backend-initialising init
(`TestAssumption_ExternalKeyProviderRunsAtBackendInit`), and `tofu state pull` emits decrypted
state (`TestAssumption_StatePullEmitsDecryptedState`).

Tofu can also take encryption from `TF_ENCRYPTION`, from `.tofu` files and from `.tf.json` files
(**unverified**). The decision closes all three without relying on it: `TF_ENCRYPTION` never passes
replace mode, and `.tofu`, `.tofu.json`, `.tf.json`, `.tfvars.json`, `.tftest.*`, `.tofutest.*` and
`.tfmock.hcl` files are refused by name, never parsed and never skipped.

### Decision 2: every module download is refused

Module sources are judged by what the download does, not by how the source string looks. Only a
literal local source, `./` or `../`, that resolves inside the user's repository is admitted. Every
registry, git, SSH, HTTP, bucket or other-host source is refused naming it, registry modules
included. A registry download runs git against github.com (**unverified**: seen while
`TestAssumption_SecondInitMakesNoProxiedConnection` was written, not pinned), where a system git
config can carry a credential helper; refusing every download holds whatever the download does.
There is no `--allow-module` flag.

Gate 2 reads the manifest the first init writes (its shape pinned by
`TestAssumption_ModulesManifestShape`) and admits only the root entry and local entries that
resolve inside the copy. Nothing fetched is ever admitted, so there is no nested-fetch bound to
record.

The first init also carries `GIT_CONFIG_NOSYSTEM=1`, `GIT_TERMINAL_PROMPT=0` and
`GIT_ALLOW_PROTOCOL=file`, a second wall in case the gate is wrong. That git honours them is pinned
by the assumption tests story rehearsal-prepare-first-init adds; no decision here rests on it,
because gate 1 has already refused every download.

### The extension and file policy

In every gated module directory:

- `.tf` and `.tfvars` files are parsed. A `.tfvars` file holds only attributes with literal values.
- The suffixes Decision 1 names are refused by name.
- A user override file (`override.tf*`, `*_override.tf*`) is refused: a user override merges after
  infrafactory's (`TestAssumption_UserOverrideMergesAfterOurs`), so it would undo the walls'
  provider and backend overrides.
- A `terraform.d` entry is refused: tofu reads it as an implied local provider mirror
  (**unverified**; the refusal holds either way).
- Any other file is data tofu does not load (`TestAssumption_LoaderIgnoresUnknownSuffixes`), and is
  admitted unparsed.

### Providers

Admitted provider sources are hashicorp/{aws, random, null, tls, time, cloudinit, local, archive},
plus the built-in `terraform_data`. A `required_providers` local name must equal its source's
type, so `x = { source = "hashicorp/aws" }` cannot dodge the aws rules. Each non-aws provider
admits a closed list of resource and data types, and only `alias` in its provider block. A
`provider "aws"` block in a child module is refused by name: a root override does not reach it
(`TestAssumption_RootOverrideMissesChildModuleProvider`). `provisioner` and `connection` are
refused at any depth.

Functions are the Layer 3 pure-function allowlist (`layer3PureFunctions`, read, never edited), plus
the file functions whose path argument is checked to resolve inside the copy. Any function name
containing `::` is refused by name.

### provider "aws"

The arguments of `provider "aws"` are checked against a static table, tied to the 5.100.0 provider
schema by a drift test (`TestRehearsalAWSProviderTableMatchesSchema`) that fails when the schema
and the table disagree. An argument the table does not name is refused under any lock version.
Prepare makes no `tofu providers schema -json` call: the table, not the user's provider binary,
decides what is admitted.

### Goal 11, statically

Every root `provider "aws"` block, aliased ones included, carries a `region` that is a literal, or
`var.x` resolved by tofu's precedence (variable default, `terraform.tfvars`, `*.auto.tfvars` in
lexical order, then the `--var-file`; pinned by `TestAssumption_VariablePrecedenceMatchesGate`).
The resolved value must equal `--region`. An absent region is refused, and so is a root that uses
`aws_*` types with no `provider "aws"` block. A `region` argument on an `aws_*` resource or data
block gets the same rule.

### The lock

A missing, unparseable or aws-less `.terraform.lock.hcl` is refused. The provider version is always
the lock's. `tofu providers lock -platform=<this machine>` always runs, and the locked aws version
must be unchanged after it (`TestAssumption_ProvidersLockKeepsLockedVersion`; prepare also checks it
and refuses on a change). A lock whose aws entry is registry.terraform.io initialises
(`TestAssumption_RegistryTerraformIoLockInitialises`), so prepare admits it with a warning that
`moved`, `import`, `removed` and ephemeral semantics may differ. A version other than 5.100.0 is
named in the report, so a failure under it is filed as a fakeaws story.

### Variables

Beyond the `.tfvars` files in the code, which the gate parses, the `--var-file` given to `prepare`
is the only variable source for every later rehearse step: `TF_VAR_*` never passes replace mode,
and passthrough refuses `-var` and `-var-file` (rehearsal-passthrough).

### The two inits

The first init is `tofu init -backend=false` under replace mode with an empty `HOME` and no
`TF_CLI_CONFIG_FILE` or `TF_PLUGIN_CACHE_DIR`. The second is `tofu init -reconfigure -get=false
-lockfile=readonly -plugin-dir=<data>/providers` under a dead proxy, and makes no proxied connection
(`TestAssumption_SecondInitMakesNoProxiedConnection`).

## Consequences

- Every `rehearse` subcommand is reviewed against one contract and one set of walls; a later
  epic that changes either amends this ADR.
- The gate refuses code a real `tofu apply` would accept: registry modules, `.tofu` files, JSON
  configuration, encryption and any `provider "aws"` argument outside the table. Admitting any of
  them is a new decision, not a flag.
- A tofu or provider release that breaks a cited claim fails its `TestAssumption_*` test in CI,
  not a rehearsal.
- The aws table is tied to one provider version, so a new version's arguments are refused until
  the table is widened by hand.
