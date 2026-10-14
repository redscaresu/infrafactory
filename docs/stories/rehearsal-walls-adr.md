---
kind: docs
status: ready
epic: rehearsal-walls
depends_on: []
touches: [docs/decisions/0042-rehearsal-command-contract-and-walls.md, docs/decisions/0039-layer-neutral-aws-generated-hcl.md]
risk: high
---

# Amend ADR-0042 and ADR-0039 with every walls decision before any walls code

ADR-0042 is amended here, once. Code stories after it tip with `ADR: none — ADR-0042 records <x>`.

**§ The walls / § The two inits.** Prepare starts the rehearsal's fakeaws as its last step, after the second init. The order is: `tofu providers schema -json` (offline, under the second init's env), resolve the fakeaws binary, start it, write the record (pid, port, token) at once, then write the provider override and complete the record. A failure or cancel after the start stops fakeaws before the half-made rehearsal is removed. There is no `rehearse start`: if fakeaws dies, the override points at a closed port, and a new prepare is the restart.

**§ The layout.** It gains `<id>/fakeaws/` (0700: db, -wal, -shm, snapshot, log), `<id>/rehearsal.json` (0600: account, region, pin, pid, port, token, and a list of roots, each with its work dir, root path, endpoint keys and aws aliases), `<id>/reads/` (a rehearsal's recorded reads) and `<id>/state-config/` (reserved for rehearsal-state-pull). A work directory is `<id>/work/<name>`. Today prepare makes one, by its existing copy convention, and the record lists it as the only root; against-the-copy adds base and head as further entries. The fakeaws binary lives outside `<id>` (§ fakeaws binary), so the mode rule is unchanged.

**§ Provider override.** `infrafactory_aws_override.tf` sits beside `infrafactory_override.tf` in each root. It holds one `provider "aws"` block per root aws block the gate found, aliases included. Each block has the static fake keys (rehearsalFakeAccessKeyID, rehearsalFakeSecretAccessKey), `skip_metadata_api_check = true`, and an `endpoints` block naming every key of the locked provider's schema. The keys come from the single `provider_schemas` entry `<host>/hashicorp/aws` for a host in rehearsalLockHosts; zero or two such entries refuse. Modelled services go to the fakeaws URL with `{region}` filled, and every other key, s3 included, goes to rehearsalDeadProxy. The "Prepare makes no schema call" sentence is scoped to the gate table.

**§ The environment.** rehearsalWallsEnv's exact key set. AWS_REGION comes from --region, and no other region variable is set.

**§ fakeaws binary.** A pin compiled into infrafactory (rehearsalFakeawsPin), kept in lockstep with ci.yml FAKEAWS_SHA. It is `go install …@pin` into `<UserCacheDir>/infrafactory/fakeaws/<pin>/` under replace mode with GOPROXY=https://proxy.golang.org (no direct) and GOTOOLCHAIN=local, or `--fakeaws-bin`. Either way, debug/buildinfo must name the module at the pin, with vcs.modified false when vcs settings are present.

**§ Start.** Exact args, a token passed only in the child's env (FAKEAWS_ADMIN_TOKEN, never argv), Setpgid, started with exec.Command so it outlives prepare's ctx. Pick-release-start retries on a new port only when the child failed to bind (address in use), 3 tries in all; any other early exit refuses at once with its exit code and log tail. The live checks: /healthz 200, /mock/requests 200 with the token and 401 without, no answer on a non-loopback address.

**§ The gate bar.** rehearsalOpen(repo, id) is the one way a later step finds a rehearsal: the id is a single path element, `<repo>/.infrafactory/rehearsals/<id>` is checked with Lstat and must be a real directory (no symlink, nothing is ever created), the record is read, and rehearsalDeadProxy is dialled again and must refuse. rehearsalRegate(rehearsal, root) admits each of infrafactory's two overrides in that root only byte-identical to the bytes rebuilt from the record; passthrough calls it.

**§ The family.** The `rehearse end <id>` contract: exit 2 for a bad id (a symlinked `<id>` included), exit 0 with nothing created for a missing one, a signal only after /healthz and the token-guarded /mock/requests answer, SIGTERM then SIGKILL to the group after a fallback, and `stop failed: pid P port N`, exit 1 and rehearsal.json kept when the port still answers after SIGKILL. Bounds: a port freed after fakeaws dies can be taken by another local process, and only the override's fake keys reach it.

**ADR-0039.** Decision 1 names awsLayer2Env, not cloudEnv. Decision 3 gains the rehearsal exception: the override writes `endpoints` into the user's provider, outside generated HCL, which is unchanged.

**Done when:**
- A loop in the PR greps ADR-0042 for each of: `infrafactory_aws_override.tf`, `rehearsal.json`, `FAKEAWS_ADMIN_TOKEN`, `buildinfo`, `rehearsalRegate`, `rehearsalOpen`, `work/<name>`, `rehearse end`, `stop failed`, `providers schema -json`, `AWS_REGION`. Each is found, and deleting any one of those lines (cp-backup) turns the loop red.
- `grep -n 'awsLayer2Env' docs/decisions/0039-*.md` is non-empty, and decision 3 names the rehearsal exception.
- Checks before every stage in docs/epics/rehearsal-walls.md hold.

**From review (binding; overrides the scope above where they differ):**
- The record holds a list of roots, and regate and the override are per root; the layout names `<id>/work/<name>` (critic).
- The record is written with pid, port and token straight after a successful start, before the override write, so a prepare killed after the start leaves something `end` can stop (critic).
- rehearsalOpen as above lives with rehearsalRegate, and both `end` and regate call it (critic).
- The schema address rule, the named fake-key constants, the bind-only retry, exec.Command over CommandContext, and the `end` exit codes and stop-failed outcome above come from the skeptics of provider-override, fakeaws-start and end.
