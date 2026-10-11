---
title: Import production state into the mocks
date: 2026-09-30
status: agreed
---

# Import production state into the mocks

## Problem

The user, 2026-09-30: "I want to be able to pull down state from prod, load it into the mock, then
run terraform actions against it."

The user, 2026-10-10, after the first real AWS Layer 3 runs: "I think its unlikely that people will
go from scenario -> mock -> real cloud … what we should do it pull down the state first from prod
import it into the mock then run terraform apply." This path is infrafactory's lead workflow from
here, not a side feature; the generate path stays and feeds it (Where it lives).

The user, 2026-10-11, setting the shape: "Imagine we have some terraform we just wrote. We want to
pull it down the state from the actual cloud. Then run our terraform against the prod state but
that is hosted on our mock." The Terraform is **new**: written for an estate it has never run
against. Asked where the state comes from, the user chose both the live account and the tfstate;
asked how the new code relates to what is deployed, both: it adds resources alongside the estate
and takes some existing ones over.

Today a mock only ever holds what a `tofu apply` put into it during a run. Nothing can put an
existing estate into a mock, so new code cannot be tried against a copy of what is really deployed
before it reaches prod (Current state). `tofu plan` against prod reads the estate but runs
nothing: an overlapping CIDR, a duplicate name, a dependency that refuses deletion, an import that
does not match, all surface only at the real apply.

## Goals

1. **The copy is prod as it is: the live account, plus the tfstate where there is one** (user,
   2026-10-11). The account is read through the AWS API with a read-only identity, so resources no
   Terraform manages, and drift, are in the copy. A tfstate, pulled through the backend its
   configuration declares, gives the Terraform addresses existing resources already have, for code
   that takes them over.
2. **The HCL is the user's new code, and the cloud is AWS first** (user, 2026-09-30 and
   2026-10-11). It may add resources that sit in or refer to the estate (data sources finding the
   real VPC, subnets, security groups by tag, name or id) and may take existing resources over
   (`import` blocks, `moved` blocks, or continuing a pulled tfstate). infrafactory generates
   nothing on this path.
3. **The read is targeted: what the code and the state reference, and their surroundings** (user,
   2026-10-11, review-2 S10). Not the whole account. The read takes what the new code's data
   sources and imports ask for, what the pulled state names, every modelled resource in the VPCs
   those touch, and every existing resource whose name a planned create would take. This bounds the
   calls and the copy, and makes goal 6's refusal list the types the code actually touches. What it
   gives up, a conflict with something unrelated in another VPC, is named in Risks.
4. **The copy is checked before anything is rehearsed; drift is absorbed, gaps refuse** (user,
   2026-10-11, review-2 B1). Every loaded resource reads back from the mock as it read from prod.
   When a tfstate is pulled, a refresh-only plan of it against the mock follows; whatever it
   changes is drift since the state's last refresh, written into the starting state as a real
   plan's refresh would, and listed. A fakeaws gap (a request the mock could not answer from loaded
   rows) refuses the rehearsal, named.
5. **Prod is only ever read, by infrafactory's read phase, in the account the user names** (user,
   2026-09-30 and 2026-10-11). The read takes the expected account id and refuses any other, proved
   by `sts:GetCallerIdentity` before any other call (review-2 S3). Its identity's IAM policy is
   exactly the reader table's actions, tied to the table by a test (review-2 S7). **The copy holds
   the secret-bearing fields the provider needs** (user, 2026-10-11, review-2 B2): instance and
   launch-template user data and SSM parameter values, because without them every rehearsal plans
   a replacement the real apply would not. Secrets Manager values and KMS decryption stay out:
   their types are not read. The rehearsal never holds the credential.
6. **What fakeaws cannot model refuses a rehearsal only when the code touches it** (user,
   2026-09-30, scoped 2026-10-11). A rehearsal is refused, naming every type, when the new code
   creates, reads, imports or (through a pulled state) continues a resource of a type fakeaws does
   not model. **S3 is refused by name for now** (user, 2026-10-10, review-1 B7): Layer 2's S3 is
   SeaweedFS, which cannot be loaded, snapshotted or restored, and fakeaws's own S3 handler is
   deprecated for provider traffic (`CONCEPT.md:2366`); loading buckets is a later epic.
7. **The rehearsal directory is inert without infrafactory** (user, 2026-09-30; review-1 B8,
   review-2 S6). Every action runs under infrafactory, and a bare `tofu` in the work directory
   cannot reach prod either: a provider override pins every `provider "aws"` the gate found to this
   rehearsal's fakeaws loopback URL, STS included, so a bare `apply` lands on the mock while it runs
   and dies at a closed port after `end`, whatever the shell holds. The backend override names no
   `path`, and the state copy and `TF_DATA_DIR` live outside the configuration copy.
8. **Credentials come from named files and nothing from the shell** (user, 2026-09-30). Mode 0600
   `KEY=VALUE` files: one for the account read, one for the backend when a tfstate is pulled (they
   may name the same identity). Each admits `AWS_SESSION_TOKEN`, since read access to prod is
   normally an assumed role or SSO session (review-2 S3). Each read step gets exactly its file's
   variables; no other command reads them.
9. **The rehearsal runs the provider version the user's lock file names, always** (user,
   2026-10-06). fakeaws is tested against exactly `5.100.0` (ADR-0039 § 4); a failure under
   another version is a fakeaws gap, fixed at source. A pulled state newer than the rehearsal's
   `tofu` is refused; a lock file naming `registry.terraform.io` sources warns that `moved`,
   `import`, `removed` and ephemeral semantics may differ (user, 2026-10-10, corrected by review-2
   S5: older states are the normal case, and `terraform_version` cannot tell the two tools apart).
10. **The local copy is plaintext, owner-only, and wiped on end** (user, 2026-10-06). The recorded
    read, the state copy and the mock's database live under the gitignored `.infrafactory/`
    (`.gitignore:5`), mode 0600, never in the run store or the UI; an explicit end deletes them. A
    single-user machine is assumed: the mock's admin routes take a per-rehearsal token (review-2 M7).
11. **One account, one region, plus IAM** (user, 2026-10-10, review-1 B5). fakeaws takes the region
    from the URL path (`../fakeaws/handlers/ec2.go:38,42`). New code whose provider names another
    region is refused. Several regions waits on fakeaws reading the region from the SigV4 credential
    scope, a later change.
12. **The base is rehearsed first, then the change** (user, 2026-10-11, after epic planning). A
    rehearsal names two git refs of the user's repository: the **base** (by default `main`, the
    code that manages prod today) and the **head** (by default the working tree, the change). The
    base plans against the checked copy with the pulled state first; then the head plans against
    the same copy and state, and its plan is applied to the copy. The base's plan is not refused
    when it is non-empty: the real cloud makes changes that are fine (user), so every change it
    plans is listed as **drift between the base and prod**, and the head's result is shown against
    it. Fakeaws mistakes are caught by the check (goal 4), never by the base's plan. Locally first;
    a PR check comes later, once a CI runner can hold a prod read credential (user, 2026-10-11).

## Non-goals

- Writing to prod in any way. The user applies the new code to prod through their own pipeline;
  the rehearsal is what they run first.
- Keeping the copy in sync. A copy is one read at one time; a fresh read is a new rehearsal.
- Generating HCL, for the new code or from the account (Terraformer-style export).
- Scaleway, GCP and Genesys. AWS is first; another cloud is a later HLD.

## Current state

**A mock's state has one way in: the provider's own create calls.** mockway, fakegcp and fakeaws
expose the same admin surface, `POST /mock/reset`, `POST /mock/snapshot`, `POST /mock/restore`,
`GET /mock/state` and `GET /mock/state/{service}` (`../mockway/handlers/handlers.go:25-29`,
`../fakegcp/handlers/handlers.go:268-272`, `../fakeaws/handlers/admin.go:48-56`). None can
load an estate. The one seed is fakeaws's `POST /mock/images` (fakeaws #43), which adds a single
AMI for the Layer 2 fixture (ADR-0039 rule 7; `internal/cli/aws_layer2_ami_seed.go:33`, `:51`).

**A snapshot is not a portable artefact.** Snapshot is SQLite `VACUUM INTO` a file beside the
mock's own database and restore swaps it back (`../fakeaws/repository/repository.go:312-352`);
fakeaws refuses it on an in-memory database (`repository.go:317-318`). The S3 data plane
(SeaweedFS) has no snapshot or restore at all (`internal/cli/mockway_client.go:196-211`).

**The mocks mint their own identifiers.** A created resource gets an id the mock generates
(`../fakeaws/handlers/ec2.go:260`, `:2244`). The account id is the compile-time
`awsproto.FakeAccountID` (`../fakeaws/handlers/awsproto/awsproto.go:33`), used by every ARN builder
(`awsproto/arn.go:15-36`) and the EC2 dispatcher (`ec2.go:49`). A restart seeds fixture AMIs into
every common region (`../fakeaws/handlers/handlers.go:51-57`).

**fakeaws ignores most filters.** `DescribeVpcs` reads only `VpcId.N` and lists every VPC
otherwise (`../fakeaws/handlers/ec2.go:278-283`); `DescribeSecurityGroups` refuses `Filter.` and
`GroupName.` (`ec2.go:1623-1629`); `DescribeSubnets` honours only `vpc-id` (`ec2.go:493-502`);
`DescribeImages` honours only `ImageId.N` (`ec2.go:2706-2708`); there is no
`DescribeAvailabilityZones` (`ec2.go:50-193`). A data source by tag or name gets the wrong answer or
an error. Unmodelled actions answer 501 (`handlers.go:109-116`) and some collections an empty set
(`ec2.go:145-146`).

**infrafactory already reads a real AWS account, read-only, for a narrow purpose.** The Layer 3
sweep lists fifteen collections with paginated Describe calls to prove a dedicated scope empty
(`internal/harness/aws_sweep.go:82-98`), through per-service HTTP doers (`AWSDoers`, `:35-39`),
under a sealed two-key credential file (`internal/harness/aws_sealed_env.go:58-100`), with STS
proof of account and principal first (`internal/cli/aws_preflight.go:35-54`) and a shipped IAM
policy tied to its tables by test (`internal/harness/aws_scope_policy_test.go:17-24`). It records ids
and states, not responses, and never runs outside the Layer 3 scope (ADR-0040).

## Design

The feature is a **rehearsal**: a copy of the relevant part of prod in a dedicated fakeaws, the
user's new code, and OpenTofu actions against the copy that cannot reach prod. Five steps:
**prepare** the code, **discover** what it touches, **read** that from prod, **load and check** the
copy, **rehearse** the base and then the head (goal 12). Only the read touches prod, and it never shares an environment, a credential or
a process with anything else.

### Prepare: two work directories, gated before any tofu

The base and the head are exported from git (`git archive <ref>` for a committed ref, never a
checkout of the user's tree; the default head, the working tree, is copied from the files
`git ls-files --cached --others --exclude-standard` lists, so uncommitted edits are rehearsed), and each, plus a throwaway copy of the configuration that
manages the pulled tfstate (the base's by default, deleted right after the pull), is copied to its
own work directory under `.infrafactory/` (mode 0700), with the local module
sources they reach: their nearest common ancestor is copied, and a source escaping the user's
repository is refused by name (review-1 B3, review-2 M1). Both are gated **before the first tofu
command** (review-2 S1; ADR-0023 sets the bar at "before any tofu"):

- **The `terraform {}` block** admits only `required_providers`, `required_version` and `backend`;
  `encryption` and `cloud {}` are refused by name (an `external` key provider runs a command, and
  `state pull` emits decrypted state, review-2 E2). Module sources that need `git`, SSH or a
  private registry fail closed, and are refused with the source named.
- **The new code** is then gated as a whole, root and every module named by
  `<data dir>/modules/modules.json` after the network-only init below. Inside `provider "aws"`,
  arguments and nested blocks are allowlisted from the locked provider's schema (`tofu providers
  schema -json`), with a test that fails when a provider version adds one the table does not name
  (review-1 S3, review-2 M4): `default_tags` and `ignore_tags` admitted; `endpoints`, the proxy
  arguments, `shared_config_files`, `shared_credentials_files`, `assume_role` and
  `assume_role_with_web_identity` refused. Refused anywhere, naming each address: `provisioner`
  and `connection` blocks (SSH to a real address goes through no HTTP proxy), the `http` and
  `external` data sources, `terraform_remote_state`, providers for any remote service other than
  AWS, and absolute or escaping paths in `local_file` and `archive_file`. Admitted providers by
  source: `hashicorp/aws`, `random`, `null`, `tls`, `time`, `cloudinit`, `local`, `archive`. Unlike
  the Layer 3 gate, which polices generated code, it admits `data`, `module`, `lifecycle`,
  `count`/`for_each` and the filesystem functions within the copy (review-2 S9).

Each directory is initialised twice (review-1 B1, review-2 S2, verified E3): first `tofu init
-backend=false` under a network-only environment with no credential (fetching providers and
modules, plus `tofu providers lock -platform=<this machine>` when the lock file lacks it, review-2
M2); then, with a `backend "local" {}` override with no `path`, `tofu init -reconfigure -get=false
-lockfile=readonly -plugin-dir=<data dir>/providers -backend-config=path=<outside the copy>` under
the dead proxy. That this second init opens no socket, and that both overrides hold for the user's
tofu version, are tests (ADR-0028).

### Discover: let the code say what it reads

The base and the head are each planned against an **empty** walled fakeaws (the read set is the
union of what both sent), and fakeaws's request log
(the `audit` table, `../fakeaws/handlers/admin.go:35`) records every Describe, List and Get the
provider sent: each data source's filters, each import's id lookup. Those requests, the ids a pulled
tfstate names, and the names every planned create would take (from `tofu show -json` of that plan)
are the **read set**. The planned values need no HCL evaluation in infrafactory; tofu evaluated
them. Against an empty mock a data source finds nothing and the plan fails, but tofu walks
independent nodes concurrently, so every data source whose inputs are known sends its request
before the plan ends; one that depends on another resolves on the next pass. Discovery repeats
against the loaded copy until a pass adds no request and the plan succeeds, bounded at five passes;
each pass that adds requests is its own read step, then the cumulative read is reloaded (user,
2026-10-11, at epic planning). The empty mock already answers with the user's account and region,
so code that pins `allowed_account_ids` configures;
that the provider sends a data source's request before failing, and that independent ones all
send, are assumption tests (ADR-0028).

### Read: prod, through one seam

infrafactory reads with the AWS SDK through the sweep's doer seam (`AWSDoers`), extended so that the
doer refuses any request whose `Action` or `X-Amz-Target` is outside the **reader table**, tested so
a mutating call fails before it leaves (review-2 S7). The sweep's collections are a starting list
of types, not code to refactor: the sweep returns ids, the readers record exchanges (review-2 M9).

1. **Identity.** `sts:GetCallerIdentity` must return the account id the user passed (and the
   principal ARN, when given); otherwise nothing else is sent (goal 5).
2. **The read set**, replayed: each discovered request is sent to prod; one whose action is not in
   the table refuses the read, naming it (epic planning, 2026-10-11). Each id from the state is
   described by its type's reader.
3. **Surroundings.** For every VPC the read touched, every modelled resource in it (subnets,
   security groups and their rules, route tables, gateways, interfaces, instances), so overlaps and
   duplicate names inside it surface. For every name a planned create takes (IAM role, key pair,
   parameter, security group within its VPC), a lookup by that name.
4. **Follow-ups** each type's provider read needs and the list call does not return: a VPC's
   attributes, an instance's attributes (user data among them, goal 5), a security group's rules,
   an IAM role's policies (about eight calls per role, review-2 M6).

The read throttles itself and backs off on `Throttling`, because API rate limits are per account
and prod's own automation shares them; it reports call count and duration. The **recorded read**
is a list of exchanges (protocol, action and parameters, raw response body), written under
`.infrafactory/` (review-2 S11). Each read step reads the credential file
itself and holds the key in that process only; no tofu process ever sees it (user, 2026-10-11).

**The tfstate, when there is one**, is pulled in its gated directory: `tofu init -input=false`
(with `--backend-config` when the block is partial, review-1 B6, and `--workspace` selected through
`TF_WORKSPACE` and recorded, review-1 S5, review-2 S4) and `tofu state pull`, under the backend
credential file and nothing else (review-1 S4: a backend whose credentials are environment
variables and whose `init` only reads; S3 first, each admitted backend with an assumption test). Its
version is checked (goal 9). Resources of unmodelled types are pruned from the rehearsal's copy of
the state with `tofu state rm` and listed; if the new code declares one, goal 6 refuses it
(review-2 B4).

### Load and check: fakeaws owns the mapping

fakeaws gains `POST /mock/load`, taking the recorded read. It parses each response with the same
XML and JSON types it writes (`../fakeaws/handlers/ec2.go:201-214`) and inserts rows through the
repository, not the handlers, since creates mint ids; all in one transaction
(`../fakeaws/repository/repository.go:99`). It refuses a non-empty field it has no column for,
naming type and field (review-1 S8), so a gap surfaces at load, not as a plan diff. **A loaded row
is what AWS returned**: ids, ARNs, owner ids, default security groups, main route tables, private
DNS names, exactly. The load sets the account and region fakeaws answers with, stored in a table
so restore and restart keep them, and suppresses the fixture AMI seed on a loaded database
(review-1 S7, review-2 M3); `/mock/reset` returns to `000000000000`. The body ceiling is a flag
with a default (review-2 M6). The admin routes refuse a non-loopback `Origin` (ADR-0026) and require
the rehearsal's token (goal 10).

**fakeaws answers data sources from loaded rows** (user, 2026-10-11, review-2 B3): generic
`Filter.N` evaluation (tags and the documented filter names) on every Describe a data source uses,
`GroupName.N` on security groups, owner and name filters on images, and `DescribeAvailabilityZones`
answered from the recorded read (AZ names and ids are account-scoped). Public images and public SSM
parameters the code's filters ask for are recorded by the read when discovery saw the request, and
loaded like any other row.

**The check** (goal 4), every time, not only in tests:

1. **Read-back.** The readers run against the loaded mock; their output must equal the recorded
   read field by field over SDK-parsed values, never bytes (request ids, element order and
   whitespace differ).
2. **Refresh-only, when a tfstate was pulled.** `tofu apply -refresh-only` of the pruned state
   against the mock. Classification is by what the mock answered, not by the diff (review-2 B1):
   if every request was answered from loaded rows, what changed is drift, written into the starting
   state and listed; if any request hit an unmodelled action (501), a refused filter or an empty
   collection, the request log names the gap and the rehearsal is refused. A data source the mock
   cannot read ends the check with exit 1 (E1), which is a gap too.

Then the mock is snapshotted with the starting state beside it, so `reset` undoes any rehearsal
without a second read. The rehearsal learns nothing and `docs/mock-gaps.md` is written only by the
classifier inside `run` (`docs/auto-learning-loop.md:83`), so each gap is filed as a fakeaws story
by hand (review-1 M3).

### Rehearse: the base, then the head, walled

With a pulled state, the rehearsal runs in this order (goal 12):

1. **Base.** `plan` of the base against the checked copy and the drift-absorbed state. Every change
   it plans is listed, by address and attribute, as drift between the base and prod. It is never
   applied and never refuses the rehearsal.
2. **Head.** `plan` of the head against the same copy and state. The report splits it into the
   head's own changes (not in the base's plan), base drift the head keeps, base drift the head
   changes, and any head change on an address the base also changes (flagged).
3. **Apply.** The head's saved plan is applied to the copy; then the head plans again and must be
   empty (a change that never converges is named).

The request-log classifier runs after every plan and apply, so a gap refuses wherever it is first
hit. `reset` restores the checked copy and state, so the head can be edited and rehearsed again
without a second read.

Without a pulled state (new code that only adds alongside prod), the starting state is empty (new code that adds alongside, and takes resources over with `import`
blocks whose real ids the mock holds), or the checked state (new code that continues an existing
configuration, with `moved` blocks where it renames). The user's `--var-file` is copied in. Every
tofu action runs in an **allowlisted environment** (review-1 S2): it starts empty and gains only the
AWS endpoint variables pointed at this rehearsal's fakeaws, with the region from the read and S3 at
the dead address while S3 is refused (review-2 M5), a catch-all `AWS_ENDPOINT_URL` and an
`HTTPS_PROXY`/`HTTP_PROXY` at dead loopback addresses, and fake keys. The runner composes child
environments from `os.Environ()` today (`internal/cli/exec_runner.go:245`), so `harness.Command`
gains a replace mode, tested so that `HOME`, `TF_CLI_CONFIG_FILE`, `TF_REATTACH_PROVIDERS`,
`TF_LOG_PATH`, `TF_PLUGIN_CACHE_DIR`, `TF_CLI_ARGS*`, `TF_VAR_*` and
`AWS_IGNORE_CONFIGURED_ENDPOINT_URLS` never pass. The provider override (goal 7) is the first wall;
the environment is the second.

tofu is then a passthrough: `plan`, `apply`, `destroy`, `import`, `state mv`, `console`, with the
work directory as `cwd` and `-out` bound inside it (review-2 M8), through the existing runner (one
Ctrl-C stops tofu cleanly, nothing reports success after an interrupt; #436, #438). `reset` restores
the snapshot and the starting state; `end` stops this rehearsal's fakeaws by its recorded pid, then
deletes the work directories, the recorded read, the state copies, the snapshot and the database
with its `-wal` and `-shm` files, idempotently (goal 10, review-1 M4).

**The rehearsal runs its own fakeaws** (user, 2026-10-06): file-backed for snapshots
(`../fakeaws/cmd/fakeaws/main.go:20`), never the shared instance training runs reset, bound to
loopback through a new bind-address flag (it listens on every interface today, `main.go:30`).

**What a rehearsal catches is what fakeaws validates.** A subnet outside its VPC fails now (fakeaws
#45); an overlapping subnet does not yet (`docs/layer3/real-vs-mock-deltas.md` § D8), and is a
Rollout 3 fakeaws story (review-2 M10). The deltas file is the rehearsal's known-limits list.

### Where it lives

**In infrafactory, not a separate product** (user's question, 2026-10-10; lead's recommendation).
The rehearsal runs on what infrafactory already owns: fakeaws, the AWS read seam the sweep started,
the tofu runner with its process group and signal guards (`internal/cli/exec_runner.go:163`,
`:243`), the walled environments, the credential-file rules and the containment ADRs. The
rehearsal is only as good as fakeaws is faithful, and the deltas Layer 3 finds against real AWS
close the gaps. A separate brand over the same packages stays cheap later.

A new command family in `internal/cli/`, which crosses the public CLI contract line and gets an
ADR; the mock's new endpoint amends `docs/mockway-contract.md` and the fakeaws contract. Nothing
touches `run`, the scenario schema, the prompts or the pipeline.

## Alternatives

- **The tfstate alone** (the 2026-09-30 draft). Rejected by the user, 2026-10-11: it sees only
  what some Terraform manages, and new code adding alongside needs what exists. It also forced a
  second mapping, state attributes to mock rows, beside the wire shape fakeaws already speaks.
- **The live account alone.** Rejected (user, 2026-10-11): code continuing an existing
  configuration needs that configuration's addresses, which only its tfstate holds.
- **Read the whole region** (the first 2026-10-11 draft). Rejected by the user, 2026-10-11 (review-2
  S10): more calls against prod's rate limits, more of prod on disk, and the IAM-scope and
  public-catalogue questions left open, to catch conflicts the targeted read's surroundings mostly
  catch anyway.
- **Withhold user data and parameter values.** Rejected by the user, 2026-10-11 (review-2 B2): the
  provider reads them, so every rehearsal would plan replacements the real apply would not.
- **Refuse on drift.** Rejected by the user, 2026-10-11 (review-2 B1): most estates have some, and
  a real plan absorbs it at refresh.
- **Data sources by id only.** Rejected by the user, 2026-10-11 (review-2 B3): real code finds the
  VPC by tag, and the wedge needs it.
- **Rehearse only the change set of a `-refresh=false` plan** (the 2026-10-10 revision). Superseded:
  it assumed the HCL already managing prod plus a change, not new code.
- **`tofu plan` of the new code against prod.** Nothing runs, so apply-time conflicts stay unseen,
  and it needs a prod credential in the provider, one mistake from an apply.
- **Replay**: apply the code to an empty fakeaws. No estate to collide with, no real id to import.
  Rejected by the user (2026-09-30).
- **Export the account to HCL and state (Terraformer, former2).** A non-goal, and a second
  translation between the API and the mock.
- **LocalStack Cloud Pods.** They persist LocalStack's own state; nothing reads a real account
  into them, and the user owns the mocks.

- **Refuse when the base's plan is not empty.** Rejected (user, 2026-10-11): some drift is expected
  because the real cloud makes changes that are fine. The base's plan is listed instead, and
  fakeaws mistakes are left to the read-back and the request-log classifier, which do not depend
  on the diff.

The wedge test this HLD must pass and none of these can: with the web stack's real state pulled,
the base (the stack as applied) plans against the copy and its plan is listed; the head renames a
resource with `moved`, adds a subnet to the real VPC and changes a tag, and its plan shows exactly
those changes against the base, applies to the copy with the real ids, converges, and fails where
AWS would, with no route to any cloud. The same without a state: new code that finds the real VPC by tag,
adds a subnet to it and imports a real security group plans and applies against a copy of the
account, with the real ids, and fails where AWS would, with no route to any cloud.

## Risks and safety

**A credential that reads prod, under infrafactory.** New: until now infrafactory read only the
dedicated Layer 3 account (ADR-0040). Contained by construction: the account id is named and proved
first; the policy is exactly the reader table, tied by test; the doer refuses anything outside the
table, so a reader bug cannot write whatever the policy allows; EC2's `DryRun=true` lets a preflight
prove the key cannot send the reap table's mutating actions (`UnauthorizedOperation`), doing nothing
either way (review-2 S7); the file is read by one step and never present in the rehearsal. ADR-0023
is amended: a credential that reaches prod is read-only, named to its account, its policy ships in
the runbook, and it is present only for the read.

**The copy holds prod secrets** (goal 5): user data and parameter values, with topology. Owner-only,
under `.infrafactory/`, behind a per-rehearsal admin token, never in the run store, which the UI
serves (`internal/api/server.go:138-154`), never in `output/`, deleted by `end`. Secrets Manager and
KMS are not read.

**Reading prod costs prod something.** Calls share the account's rate limits with its own
automation; IAM's limit is global. The targeted read bounds the count; it throttles, backs off and
reports.

**A targeted read can miss a conflict.** Something outside every touched VPC that a create collides
with by a property the read does not look up (not a name) is not in the copy. The report lists what
was read, so the user sees the boundary.

**A rehearsal that reaches prod.** Three walls: the provider override pinned to the mock's loopback
URL; the allowlisted environment with dead proxy and catch-all; the backend override to `local`;
plus the gate. Each is proved the fakeaws way: remove one in a test and the `apply` dies at a dead
address, fast. The provider override is new for this path: ADR-0039 routes generated HCL by
environment only; here the user's code must be inert even outside infrafactory (goal 7), which only
an override can give.

**A permissive mock certifies a wrong rehearsal.** An apply fakeaws accepts and AWS rejects is a
known limit; the rehearsal proves references, ids, ordering and the rules fakeaws enforces. The
round trip per type also plans each example's state against its loaded copy (exit 0), and the
coverage matrix records which types have a real-shaped fixture, so "loadable" never reads as
"proved against AWS" (review-2 S8).

**Most real estates need fakeaws work first** (review-2 B4). Of the sweep's fifteen collections,
eleven have fakeaws tables; volumes, NAT gateways, snapshots and launch templates do not, and a
typical web estate also holds S3, ELBv2, autoscaling and CloudWatch. Goal 6 refuses only what the
code touches, so a change confined to modelled types rehearses today; the rest is the fakeaws queue
Rollout 5 drains.

**The copy is a moment.** Prod changes after the read; a rehearsal reports the read's age.

**ADRs touched.** ADR-0023 (amended, above); ADR-0032 (the recorded read is the API reference it
lacked: a refresh against loaded rows is classified by what the mock answered, review-2 B1);
ADR-0039 (the provider override replaces environment-only routing on this path); ADR-0040 (the read
seam extends beyond the Layer 3 scope, for reading only); ADR-0026 (its `Origin` rule extended to
fakeaws's admin routes); ADR-0009 (snapshot and restore reused for a baseline never applied by a
run); ADR-0028 (the overrides and the offline init are tests). New: the rehearsal command contract
and its walls; the account-read contract and the recorded-read format; the fakeaws load contract.

## Rollout

0. **Assumption tests and a real recording, now.** Against tofu alone: the overrides, the two
   inits, refresh-only behaviour (review-2 E1-E3 are the first three). Against AWS: under the Layer
   3 claim, `aws-web-live` is applied to the dedicated account with `--keep`, its pre-destroy state
   kept (review-1 S9), the account read through a first cut of the seam into `.infrafactory/` (it
   carries the account id, so never git), and `reap` clears it. That recording is the loader's
   real fixture from step 2.
1. **The read** (infrafactory): the seam with its action wall, the reader table and the shipped
   policy tied to it, the account proof, session-token credentials, throttling, the
   recorded-read format; first the web stack's types. Proved against fakeaws, then once against the
   Layer 3 account.
2. **The loader** (fakeaws): `/mock/load`, field refusal, the stored identity, the token and
   `Origin` refusal, the bind flag, the round trip with its plan check, the coverage-matrix column;
   the instance root volume (`docs/layer3/real-vs-mock-deltas.md` D7) from the step-0 recording.
3. **Data sources and validation** (fakeaws): generic filters, `DescribeAvailabilityZones`,
   image owner and name filters, and the subnet-overlap check.
4. **The rehearsal** (infrafactory), state path first (user, 2026-10-11): prepare and gate the
   base and the head, the tfstate pull and pruning, discovery over both, the provider override,
   the allowlisted environment, passthrough, check with the refresh-only drift absorbed, the base
   plan listed, the head planned, applied and converged against it, snapshot, `reset`, `end`, and
   the fail-closed tests. Proved on the step-0 recording and the web stack's own pre-destroy state
   with the wedge. Then the path without a state: new code that finds the real VPC by tag, adds a
   subnet and imports a real security group.
5. **A larger estate**: one infrafactory never built. What the code touches and fakeaws does not
   model is refused by name, and that list orders the fakeaws epics (S3 on fakeaws's handler, ELBv2,
   autoscaling, NAT gateways, volumes, CloudWatch first).

## Open questions

- **Which larger estate proves Rollout 5.** Before it is planned, the user runs read-only in that
  estate's directory `tofu state list | cut -d. -f1 | sort | uniq -c` and `tofu providers` and
  shares the counts (no ids leave the machine), which sizes the fakeaws queue (review-2 S9).
- **`assume_role` in the new code's provider block** is refused by the gate today; whether it
  becomes admitted once fakeaws models `AssumeRole`.
- **Where the UI stands**: nothing here; the rehearsal is CLI-only.

## Epics

- [rehearsal-assumption-tests](../epics/rehearsal-assumption-tests.md)
- [fakeaws-rehearsal-instance](../epics/fakeaws-rehearsal-instance.md)
- [fakeaws-loaded-identity](../epics/fakeaws-loaded-identity.md)
- [fakeaws-data-sources](../epics/fakeaws-data-sources.md)
- [rehearsal-prepare-gate](../epics/rehearsal-prepare-gate.md)
- [aws-hold-for-read](../epics/aws-hold-for-read.md)
- [prod-read-seam](../epics/prod-read-seam.md)
- [fakeaws-loader](../epics/fakeaws-loader.md)
- [rehearsal-walls](../epics/rehearsal-walls.md)
- [rehearsal-passthrough](../epics/rehearsal-passthrough.md)
- [rehearsal-state-pull](../epics/rehearsal-state-pull.md)
- [rehearsal-against-the-copy](../epics/rehearsal-against-the-copy.md)
- [rehearsal-without-state](../epics/rehearsal-without-state.md)
- [larger-estate](../epics/larger-estate.md)
