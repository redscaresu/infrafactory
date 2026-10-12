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
prints its help and exits 0. An unknown subcommand is a usage error, and so, anywhere in the
family, is a flag parse error, a missing required flag or a broken flag group. It reads no
`infrafactory.yaml`, because a rehearsal takes its account and region from flags.

The group's flag-error func and persistent pre-run make those usage errors for every subcommand.
Two rules keep that true:

- Cobra checks a command's args before any pre-run, so the group cannot catch them: every rehearse
  subcommand's `Args` validator returns a usage `CLIError`, and cobra's stock validators (such as
  `cobra.ExactArgs`) are wrapped.
- Cobra runs only the nearest persistent pre-run, so no rehearse subcommand sets its own
  `PersistentPreRunE` or `PersistentPreRun`: one would shadow the group's, and with it the root's
  `--output` check. A test walks the family and holds this.

- `rehearse prepare <dir> --account <id> --region <r> [--var-file f]` is decided here.
- `rehearse read` is defined by ADR-0041, not redefined here.
- `rehearse end` is reserved to epic rehearsal-walls; `plan|apply|destroy|import|state mv|console`
  passthrough to rehearsal-passthrough; discover, load, check and the base-then-head loop to
  rehearsal-against-the-copy. Each amends this ADR when it lands.

Exit codes are ADR-0002's: a usage error is 2, and every refusal is `command_failed`, exit 1. The
family adds no error code.

### The layout

A rehearsal lives in `.infrafactory/rehearsals/<id>/` at the root of the repository that contains
the current directory (its nearest ancestor holding `.git`), never under `.infrafactory/runs/` or
`.infrafactory/live/`, which the UI serves. `prepare` refuses a `<dir>` that is not inside that
same repository, and every later step, which names only `<id>`, finds it under that same root.
`rehearsals/` holds a `.gitignore` of `*`. Each `<id>/` has `work/` (the copied code), `data/`
(`TF_DATA_DIR`), `home/` (an empty `HOME`), `state/` (the state file, outside the copy) and
`vars.tfvars` (the `--var-file`, copied outside `work/` so tofu never auto-loads it). Directories
are 0700 and files 0600. Tofu writes files with its own modes, so after each tofu run the modes are
tightened again: directories 0700, executables 0700 (provider binaries must stay runnable), other
files 0600.

### The walls

Three walls stand between the user's code and prod:

1. **The gate** (this epic): the copy is parsed and refused before any tofu runs.
2. **The two inits** (this epic): a network-only first init, the gate again over every module it
   resolved, then an offline second init under a dead proxy.
3. **The provider override, the environment and the backend override** (rehearsal-walls; the
   second init already writes the backend override), so a bare `tofu` in the work directory
   reaches only this rehearsal's fakeaws.

Every tofu command runs in **replace mode** (story rehearsal-prepare-replace-env): the child's
environment is `PATH` plus the variables the step names, and nothing else is inherited. So
`TF_ENCRYPTION`, `TF_VAR_*`, `TF_CLI_ARGS*`, `TF_CLI_CONFIG_FILE`, `TF_PLUGIN_CACHE_DIR`, the AWS
credential and profile variables, and anything else in the user's shell never reach tofu.

### The gate bar

ADR-0023's bar, applied to user code: the gate is an allowlist, so what it does not name is
refused; it parses, never scans text; an unparseable file is refused; nothing is skipped; and no
tofu command runs for a refused configuration.

The gate runs before every tofu step, not once. Every rehearse step that runs tofu first re-runs
the full gate over the whole copy (the root, every module gate 2 admits, and every template a
`templatefile` names) and refuses on any problem. So a file an apply wrote is gated like any other,
whatever its name: an apply that overwrote a template or wrote configuration is refused at the next
step. Infrafactory's own override files (the backend override the second init writes, the provider
override rehearsal-walls writes) are the one exception to the override refusal: a re-gate admits
each only when its bytes equal what infrafactory writes.

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

A local source that crosses a symlink at any component is refused too, naming the module and the
link. Tofu joins a source to the calling dir as written and cleans it lexically, so through
`./link` a module's `path.module` spells the link, while the gate would check the dir the link
resolves to: `"${path.module}/../.."` names a different dir on each side. Refusing the link keeps the
two the same.

Gate 2 reads the manifest the first init writes (its shape pinned by
`TestAssumption_ModulesManifestShape`) and admits only the root entry and local entries that
resolve inside the copy. Nothing fetched is ever admitted, so there is no nested-fetch bound to
record.

The first init also carries `GIT_CONFIG_NOSYSTEM=1`, `GIT_TERMINAL_PROMPT=0` and
`GIT_ALLOW_PROTOCOL=none`, a second wall in case the gate is wrong. `none` names no transport, so
git refuses every one, `file` included: a local module never uses git, and allowing `file` would
let a `git::file:///...` source the gate missed clone a host repository into the rehearsal. That git
then reads no system config, so no system credential helper
(`TestAssumption_GitNoSystemDropsCredentialHelper`), and refuses https, ssh and file:// before
connecting (`TestAssumption_GitAllowProtocolNoneRefusesEveryTransport`) is pinned;
`GIT_TERMINAL_PROMPT=0` is not. No decision here rests on any of them, because gate 1 has already
refused every download.

### The extension and file policy

In every gated module directory:

- `.tf` and `.tfvars` files are parsed. A `.tfvars` file holds only attributes with literal values.
- The suffixes Decision 1 names are refused by name.
- A user override file (`override.tf*`, `*_override.tf*`) is refused (infrafactory's own, on a
  re-gate, as § The gate bar says): a user override merges after
  infrafactory's (`TestAssumption_UserOverrideMergesAfterOurs`), so it would undo the walls'
  provider and backend overrides.
- A `terraform.d` entry is refused: tofu reads it as an implied local provider mirror
  (**unverified**; the refusal holds either way).
- A `tests` entry is refused: tofu reads it as the test directory, so broken HCL in
  `tests/a.tftest.hcl` fails `init` and `validate` (`TestAssumption_LoaderIgnoresUnknownSuffixes`),
  and a `run` block there is configuration the gate would never see.
- A `.tf` file holds only blocks; a top-level attribute is refused.
- Any other file, and any other subdirectory, is admitted unparsed, because tofu's configuration
  loader skips them (`TestAssumption_LoaderIgnoresUnknownSuffixes`). Tofu still reads some of those
  files by other routes, and another rule owns each: `.terraform.lock.hcl`, from which tofu takes
  provider versions, is parsed before the first tofu command (§ The lock); `.terraform/`, tofu's default data dir, is left out of the copy
  (story rehearsal-prepare-copy), and every step's `TF_DATA_DIR` lies outside it (§ The layout);
  a file the configuration names by path is held to § Paths and templates.

### Providers

Admitted provider sources are hashicorp/{aws, random, null, tls, time, cloudinit, local, archive},
plus the built-in `terraform_data`. A `required_providers` local name must equal its source's
type, so `x = { source = "hashicorp/aws" }` cannot dodge the aws rules. Each non-aws provider
admits a closed list of resource and data types, and only `alias` in its provider block. A
`provider "aws"` block in a child module is refused by name: a root override does not reach it
(`TestAssumption_RootOverrideMissesChildModuleProvider`). `provisioner` and `connection` are
refused at any depth.

Functions are the Layer 3 pure-function allowlist (`layer3PureFunctions`, read, never edited), plus
the file functions below, whose path argument is checked. Any function name containing `::` is
refused by name.

### Paths and templates

Every path the code reads or writes must resolve inside the copy (`<id>/work`). The checked paths
are the first argument of `file`, `fileexists`, `fileset`, `filebase64`, `filemd5`, `filesha1`,
`filesha256`, `filesha512`, `filebase64sha256`, `filebase64sha512` and `templatefile`;
`filename` and `source` of `local_file` and `local_sensitive_file`; `filename` of data
`local_file` and data `local_sensitive_file`; `output_path`, `source_file` and `source_dir` of
`archive_file`, resource and data; and every aws argument terraform-provider-aws 5.100.0 reads a
local file from: `source` of `aws_s3_object` and `aws_s3_bucket_object`; `filename` of
`aws_lambda_function`, `aws_lambda_layer_version`, `aws_connect_contact_flow`,
`aws_connect_contact_flow_module` and `aws_rds_custom_db_engine_version`; `zip_file` of
`aws_synthetics_canary` and `aws_gamelift_script`; `configurations` of `aws_emr_cluster`; and
`asset.import_assets_from_signed_url.filename` of `aws_dataexchange_revision_assets`, a `dynamic`
block's `content` included.

A checked path is admitted only as `path.module` or `path.root`, alone or joined with a literal
starting with `/`, resolved from that directory, and only if it stays inside the copy. A bare
relative literal is refused: tofu resolves it from its working directory rather than the module
directory (**unverified**; refusing it holds either way). Absolute, escaping, variable and
otherwise computed paths are refused, and `abspath` and `pathexpand` are refused by name.

A written path (`filename` of `local_file` and `local_sensitive_file`, `output_path` of
`archive_file`) must also not name a file tofu could load, so an apply that would write
configuration is refused before it runs. This is the early refusal; the re-gate before every tofu
step (§ The gate bar) is what gates every file an apply writes, whatever its name. A written path
is refused when its base name has a `.tf` or `.tfvars` suffix or any suffix Decision 1 refuses,
when it is `.terraform.lock.hcl`, or when any of its elements is `terraform.d` or `.terraform`.
Names are compared case-insensitively, since a case-insensitive filesystem (macOS's default) maps
`ZZ_OVERRIDE.TF` and `.TERRAFORM.LOCK.HCL` onto the names they spell.

A written path is also refused when it is the same file as a template any `templatefile` in the
gate run reads, in any module: `templatefile` can run at apply, after the write, on a template the
gate never saw, and the re-gate before the next step comes too late. The filesystem decides
sameness (`os.SameFile`), so a spelling a case-insensitive filesystem maps onto the template is
caught.

`fileset`'s second argument, the glob, is checked too, since it is joined onto the path before
globbing (**unverified**; the check holds either way). It must be a literal relative pattern of
ASCII letters, digits and `/ . - _ * ?` with no `..` element; anything else is refused. The
characters are an allowlist because glob syntax spells `..` without writing it: `{..,x}` and
`[.][.]` both match it. Any `..` element is refused, not only one that leaves the copy lexically,
because `**` can match zero directories, so a lexical check of a `..` after a glob undercounts how
far it climbs.

The template a `templatefile` names is gated too, because the gate admits other files unparsed
and a template can call any function. It is parsed with `hclsyntax.ParseTemplate`, and every
function call in it is held to the function rule above. No path in a template can be checked:
every variable in it, `path` included, comes from the `templatefile` vars map, so
`{ path = { module = "/Users/me/.aws" } }` makes `path.module` a host directory. So every path
function call in a template, `templatefile` included, is refused. A missing or unparseable
template is refused.

### provider "aws"

The arguments of `provider "aws"` are checked against a static table, tied to the 5.100.0 provider
schema by a drift test (`TestRehearsalAWSProviderTableMatchesSchema`) that fails when the schema
and the table disagree. An argument the table does not name is refused under any lock version, and
the refusal names the lock's.
Prepare makes no `tofu providers schema -json` call: the table, not the user's provider binary,
decides what is admitted.

### Goal 11, statically

Every root `provider "aws"` block, aliased ones included, carries a `region` that is a literal, or
`var.x` resolved by tofu's precedence (variable default, `terraform.tfvars`, `*.auto.tfvars` in
lexical order, then the `--var-file`; pinned by `TestAssumption_VariablePrecedenceMatchesGate`).
The resolved value must equal `--region`. An absent region is refused, and so is a root with no
unaliased `provider "aws"` block, whatever blocks it holds: a rehearsal is AWS-only, and anything
tofu plans through the default provider (a resource, a `removed` or `import` block, an address only
the state holds) would otherwise get tofu's implied empty block, which has no region. A `region`
argument on an `aws_*` resource or data block gets the same rule, except that in a child module it
must be a literal: the gate resolves variables only in the root. A root var file spelled in another
case (`Terraform.tfvars`, `x.AUTO.tfvars`) is refused: on a case-insensitive filesystem tofu may
load it (**unverified**; refusing it holds either way).

### The lock

Before gate 1, prepare parses the copy's root `.terraform.lock.hcl` (hclsyntax, never a text scan).
A missing or unparseable lock is refused, and so is one with no `hashicorp/aws` entry, one with an
entry whose version it cannot read, one whose entries for one provider pin different versions, and
one naming any provider outside the eight admitted, or from a registry other than
registry.opentofu.org and registry.terraform.io, by name. No tofu runs for a refused lock. Every
provider version is the lock's.

The first init runs `tofu providers lock -platform=<GOOS_GOARCH>` before `init`, so a lock that
lacks this platform's hashes still initialises. providers lock keeps the locked version
(`TestAssumption_ProvidersLockKeepsLockedVersion`), and prepare holds every provider's whatever tofu
does: after the first init it parses the copy's lock again under the same rules and refuses, naming
both versions, if the init changed any provider's version or added an entry the user's lock did not
have. An entry the init drops passes: tofu rewrites the lock with only the providers the
configuration needs (**unverified**), and whether or not it does, dropping an entry brings in no
version the user did not lock. Versions compare by provider, so a registry.terraform.io entry that
tofu replaces with a registry.opentofu.org one at the same version is no change. The user's own lock is never written: only the copy's is.

A lock whose aws entry is registry.terraform.io's initialises: providers lock adds a
registry.opentofu.org entry at the same version, and init installs it
(`TestAssumption_RegistryTerraformIoLockInitialises`). Prepare admits it with a warning that
`moved`, `import`, `removed` and ephemeral semantics may differ.

**Another provider version.** An aws version other than 5.100.0 is named in the report, so a failure
under it is filed as a fakeaws story, and so is the refusal of a `provider "aws"` argument the table
does not name. The report alone would fail open, though: § Paths and templates names the arguments
that read a local file in one release of each provider it covers (`rehearsalPathArgReleases`):
hashicorp/aws 5.100.0, hashicorp/local 2.9.1 and hashicorp/archive 2.8.1, the last two checked
against those releases' `tofu providers schema -json`. An argument a later release adds that reads a
file would pass the gate unchecked. So the lock admits each of the three only at an exact `X.Y.Z`
release of that release's major, no later than it. A later release, another major or a pre-release
is refused, naming the version. An earlier release of the same major is admitted because a provider
removes an argument only in a major release, so every argument it has, the table's release has, and
the table speaks for it. That is the providers' versioning policy, **unverified**, and the admission
rests on it: were it false, a file-reading argument dropped before the table's release would pass
unchecked under an earlier lock. Admitting only the table's releases would not rest on it, and was not
taken: the epic runs other aws versions, and their failures are how fakeaws learns them.

### Variables

Beyond the `.tfvars` files in the code, the `--var-file` given to `prepare` is the only variable
source for every later rehearse step: `TF_VAR_*` never passes replace mode, and passthrough refuses
`-var` and `-var-file` (rehearsal-passthrough). The var file is copied to `<id>/vars.tfvars` at
0600, outside `work/` so tofu never auto-loads it; a `.json` var file is refused; and the gate
parses it like any `.tfvars` file, so it holds only attributes with literal values.

### The two inits

The first init is three tofu commands in the copy's root: `tofu get -no-color`, `tofu providers
lock -platform=<GOOS_GOARCH>` and `tofu init -backend=false -input=false -no-color`, so no backend
is initialised. `get` comes first because providers lock fails on a root whose module calls are not
installed (`TestAssumption_ProvidersLockKeepsLockedVersion`); gate 1 has refused every download, so
it only records the local modules. Each runs under replace mode with exactly `HOME=<id>/home`
(empty, 0700), `TF_DATA_DIR=<id>/data`, `GIT_CONFIG_NOSYSTEM=1`, `GIT_TERMINAL_PROMPT=0` and
`GIT_ALLOW_PROTOCOL=none`, so no `TF_CLI_CONFIG_FILE` or `TF_PLUGIN_CACHE_DIR`, and under a deadline
at which the runner interrupts tofu's process group, then kills it. Then prepare checks the lock
again (§ The lock) and tightens the modes (§ The layout).

A cancel or a tofu error refuses the prepare with tofu's stderr, never a success report, and
prepare removes the half-made rehearsal and exits 1. Prepare holds a signal guard from start to end,
so a Ctrl-C is such a cancel: during a tofu command, tofu gets its one interrupt; during the copy,
the lock or gate 1, which do not stop for it, prepare refuses before the first tofu; after the last
tofu, it refuses once the modes are tightened.

Before the second init, prepare dials the dead proxy and refuses unless the connection is refused
(one that connects, or fails any other way, refuses), then writes `infrafactory_override.tf` at 0600
in the copy's root (the work dir prepare reports, under `<id>/work/`): a `backend "local" {}`
override that names no `path`. The second init is `tofu init -input=false -no-color -reconfigure -get=false
-lockfile=readonly -backend-config=path=<id>/state/terraform.tfstate
-plugin-dir=<id>/data/providers`, under replace mode with only `HOME`, `TF_DATA_DIR` and
`HTTP_PROXY`/`HTTPS_PROXY` at the dead proxy, and no `NO_PROXY`. It makes no proxied connection
(`TestAssumption_SecondInitMakesNoProxiedConnection`).

A user `backend` block is neutralised: the override replaces it, and the command-line path puts the
state outside the copy (`TestAssumption_CLIBackendPathKeepsStateOutOfTheCopy`, whose root declares
`backend "s3"`). So no user backend is ever initialised.

## Consequences

- Every `rehearse` subcommand is reviewed against one contract and one set of walls; a later
  epic that changes either amends this ADR.
- Every tofu step pays for a full gate over the copy first.
- The gate refuses code a real `tofu apply` would accept: registry modules, `.tofu` files, JSON
  configuration, encryption, bare relative paths and any `provider "aws"` argument outside the
  table. Admitting any of
  them is a new decision, not a flag.
- A tofu or provider release that breaks a cited claim fails its `TestAssumption_*` test in CI,
  not a rehearsal.
- The aws table is tied to one provider version, so a new version's arguments are refused until
  the table is widened by hand.
