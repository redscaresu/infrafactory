# Operations

Task-specific detail that `AGENTS.md` points to. Read a section when its task comes up; nothing
here needs reading at the start of a session.

## Layer 3: real Scaleway

Layer 3 spends real money against `api.scaleway.com`. Read ADR-0023 before changing anything on
this path; it defines what a passing Layer 3 result means.

**The run owns its project** (ADR-0025). infrafactory creates the run's project through the
Account API before the apply and hands it to the provider as the default, so every resource
lands in it. Generated HCL must not declare `scaleway_account_project` or set `project_id`; the
Layer 3 HCL gate refuses both.

**Credentials.** Authenticate as the `infrafactory-layer3` IAM application, whose key lives in
`~/.config/infrafactory/layer3.env` (mode `0600`; rotated 2026-09-07):

```bash
set -a; . ~/.config/infrafactory/layer3.env; set +a
```

Do not fall back to the default `scw` profile: on a developer machine that is a personal key,
here the organization owner's. The sandbox strips `SCW_PROFILE`/`SCW_CONFIG_PATH`, which also
forces the default profile, so "just use the default profile" silently means "run as whoever
that is". The application's policy set is recorded in ADR-0023's credential amendments.

**`openclaw-prod` is protected by software, not by the API**, since `InstancesFullAccess` was
granted. The guards are the deny-by-default allowlist, the run-project guard
(`AssertProjectDeletable`), the orphan sweep, and project-per-run. Treat each as load-bearing;
widening any is a blast-radius decision (ADR-0023).

**Configuration:**
1. `SCW_DEFAULT_ORGANIZATION_ID` — required; preflight fails closed without it (`layer3.env` sets it).
2. `validation.layers.sandbox_deploy.enabled: true`.
3. `validation.layers.sandbox_deploy.allow_resource_types` — deny-by-default; empty or absent
   denies everything. Checked after generation and before apply, so a denied type costs nothing.
   The list lives in three places (`internal/config/config.go`, `infrafactory.yaml`,
   `docs/layer3/coverage.md`) and tests keep them in step.
4. `scaleway.fallback_project_id` — a dedicated disposable project for anything that resolves the
   provider default. Refused if set to the organization id.

**Operational notes:**
- The sandbox environment is sealed, not merely overridden (`harness.SandboxStripEnv`): an
  inherited `SCW_API_URL` cannot retarget a real apply at mockway.
- `infrafactory reap <scenario>` destroys what an interrupted run left behind, gated by
  `AssertProjectDeletable` and verified by a real-API sweep. A reap that cannot prove the account
  clean fails.
- Layer 3 is opt-in, off by default, and never wired into a scheduled CI job.
- Real-vs-mock deltas: `docs/layer3/real-vs-mock-deltas.md`. Real Scaleway can return a create
  error after the resource exists; apply retries once (`sandboxApplyAttempts`), never on a
  cancelled context.
- A `run` writes auto-learned pitfalls into `pitfalls/*.yaml` **in the checkout it runs from**. Run
  generated and real-cloud runs from a throwaway worktree, not the lead's working copy, and never
  `git add -A` after one: a learned entry once replaced a curated safety pitfall and nearly merged
  inside an unrelated PR.
- `infrafactory pitfalls check-avoid aws --resource R --attribute A --from DIR` is the only way a
  `source: avoid` rule leaves the corpus: it replays the shape cut from `DIR` on fakeaws, with no
  LLM, and retires the rule only on a clean apply and plan (ADR-0034). It writes
  `pitfalls/avoid-checks/`; commit exactly what it wrote, from a throwaway worktree.
- A failed run auto-destroys and then sweeps. If the sweep cannot confirm the account clean, the
  failure names `infrafactory reap <scenario>`; act on it.

## Layer 3 (AWS)

Layer 3 on AWS spends real money in one dedicated member account, the scope, built once by hand
below. infrafactory never creates or deletes the account: a run claims it, and the sweep proves it
empty (HLD 2026-09-27-aws-web-stack § Containment; ADR-0023). The SCP is the enforced boundary,
and preflight cannot assert it, so it is recorded here.

What the code reads from the setup:
- The key file `~/.config/infrafactory/layer3-aws.env`, mode `0600`: exactly `AWS_ACCESS_KEY_ID`
  and `AWS_SECRET_ACCESS_KEY`. Preflight refuses any group or other permission bit.
- The config's `aws` block (`region`, `account_id`, `principal_arn`), which
  `sts:GetCallerIdentity` must answer exactly.
- Two SSM parameters. The claim, `AWSClaimParameter` (`/infrafactory/layer3/claim`), holds the
  running holder; the key writes and deletes it and nothing else. The stamp, `AWSStampParameter`
  (`/infrafactory/layer3/stamp`), holds the account id; the admin writes it once in step 5 and the
  key can only read it.
- The region's default VPC is gone.

### Scope setup

Run this once, from the repo root. The account id and the key id never enter git (the
repo is public): they stay in shell variables and the local config, and outputs pasted into a PR
are cut to their last four characters. No command prints the secret key.

**0. Variables and the admin profile.** `MGMT` is the CLI profile of the Organizations management
account. `OTHER` is any region but `REGION`, for the deny checks.

```bash
REGION=us-east-1     # aws-web-live and every AWS scenario pin us-east-1
OTHER=eu-west-1
MGMT=<management profile>
```

`ACCOUNT_ID` is the member account step 1 creates. Once it is set, add the admin profile, which
assumes the role Organizations creates in every new member account:

```bash
cat >> ~/.aws/config <<CONFIG

[profile infrafactory-admin]
role_arn = arn:aws:iam::$ACCOUNT_ID:role/OrganizationAccountAccessRole
source_profile = $MGMT
region = $REGION
CONFIG
```

**1. The account.**

```bash
REQUEST=$(aws organizations create-account --email <an unused email> --account-name infrafactory-layer3 \
  --profile $MGMT --query CreateAccountStatus.Id --output text)
aws organizations describe-create-account-status --create-account-request-id $REQUEST \
  --profile $MGMT --query 'CreateAccountStatus.[State,AccountId]' --output text
```

Repeat the second command until it prints `SUCCEEDED` and the id, then set `ACCOUNT_ID=<the id>`
and add step 0's profile. Verify:

```bash
aws organizations describe-account --account-id $ACCOUNT_ID --profile $MGMT \
  --query Account.Status --output text                                      # ACTIVE
aws sts get-caller-identity --profile infrafactory-admin --query Account --output text   # ACCOUNT_ID
```

**2. Delete the default VPC** in `REGION`, with its subnets and internet gateway:

```bash
VPC=$(aws ec2 describe-vpcs --filters Name=isDefault,Values=true --query 'Vpcs[0].VpcId' \
  --output text --profile infrafactory-admin --region $REGION)
for SUBNET in $(aws ec2 describe-subnets --filters Name=vpc-id,Values=$VPC --query 'Subnets[].SubnetId' \
  --output text --profile infrafactory-admin --region $REGION); do
  aws ec2 delete-subnet --subnet-id $SUBNET --profile infrafactory-admin --region $REGION
done
IGW=$(aws ec2 describe-internet-gateways --filters Name=attachment.vpc-id,Values=$VPC \
  --query 'InternetGateways[0].InternetGatewayId' --output text --profile infrafactory-admin --region $REGION)
aws ec2 detach-internet-gateway --internet-gateway-id $IGW --vpc-id $VPC --profile infrafactory-admin --region $REGION
aws ec2 delete-internet-gateway --internet-gateway-id $IGW --profile infrafactory-admin --region $REGION
aws ec2 delete-vpc --vpc-id $VPC --profile infrafactory-admin --region $REGION
```

Verify, each printing `0`:

```bash
aws ec2 describe-vpcs --query 'length(Vpcs)' --profile infrafactory-admin --region $REGION
aws ec2 describe-subnets --query 'length(Subnets)' --profile infrafactory-admin --region $REGION
aws ec2 describe-internet-gateways --query 'length(InternetGateways)' --profile infrafactory-admin --region $REGION
aws ec2 describe-network-interfaces --query 'length(NetworkInterfaces)' --profile infrafactory-admin --region $REGION
aws ec2 describe-addresses --query 'length(Addresses)' --profile infrafactory-admin --region $REGION
aws ec2 describe-instances --query 'length(Reservations)' --profile infrafactory-admin --region $REGION
```

**3. The IAM user**, before the SCP, since the SCP denies IAM. Its policy,
`docs/layer3/aws/iam-policy.json`, covers claim, sweep and reap only; the apply's actions are epic
[aws-web-live-on-real-aws](epics/aws-web-live-on-real-aws.md)'s. It grants
`sts:GetCallerIdentity`; `ssm:PutParameter` and `ssm:DeleteParameter` on the claim alone;
`ssm:GetParameter` on the claim and the stamp; and, pinned by `aws:RequestedRegion`,
`ssm:DescribeParameters`, `ec2:Describe*` and exactly the EC2 actions `harness.AWSReapSteps`
exports. `internal/harness/aws_scope_policy_test.go` holds it there.

```bash
aws iam create-user --user-name infrafactory-layer3 --profile infrafactory-admin
sed -e "s/REGION/$REGION/g" -e "s/ACCOUNT_ID/$ACCOUNT_ID/g" docs/layer3/aws/iam-policy.json > "$TMPDIR/p.json"
aws iam put-user-policy --user-name infrafactory-layer3 --policy-name infrafactory-layer3-scope \
  --policy-document "file://$TMPDIR/p.json" --profile infrafactory-admin
```

Verify with the policy simulator:

```bash
sim() { aws iam simulate-principal-policy --policy-source-arn arn:aws:iam::$ACCOUNT_ID:user/infrafactory-layer3 \
  --profile infrafactory-admin --query 'EvaluationResults[].EvalDecision' --output text "$@"; }
PARAM=arn:aws:ssm:$REGION:$ACCOUNT_ID:parameter/infrafactory/layer3
sim --action-names ssm:PutParameter --resource-arns $PARAM/claim                            # allowed
sim --action-names ssm:PutParameter ssm:DeleteParameter --resource-arns $PARAM/stamp        # implicitDeny implicitDeny
sim --action-names ssm:PutParameter ssm:DeleteParameter --resource-arns $PARAM/other        # implicitDeny implicitDeny
sim --action-names ec2:DescribeVpcs ec2:DeleteVpc \
  --context-entries ContextKeyName=aws:RequestedRegion,ContextKeyValues=$REGION,ContextKeyType=string   # allowed allowed
sim --action-names ec2:DescribeVpcs \
  --context-entries ContextKeyName=aws:RequestedRegion,ContextKeyValues=$OTHER,ContextKeyType=string    # implicitDeny
sim --action-names ec2:RunInstances \
  --context-entries ContextKeyName=aws:RequestedRegion,ContextKeyValues=$REGION,ContextKeyType=string   # implicitDeny
```

**4. The key file**, written without printing the key. `umask` sets the mode only on a new
file, so if `layer3-aws.env` already exists, `chmod 600` it before running this:

```bash
mkdir -p ~/.config/infrafactory && (umask 077; aws iam create-access-key --user-name infrafactory-layer3 \
  --profile infrafactory-admin --query 'AccessKey.[AccessKeyId,SecretAccessKey]' --output text \
  | awk '{printf "AWS_ACCESS_KEY_ID=%s\nAWS_SECRET_ACCESS_KEY=%s\n",$1,$2}' > ~/.config/infrafactory/layer3-aws.env)
```

Verify:

```bash
stat -f %Lp ~/.config/infrafactory/layer3-aws.env   # 600 (on Linux: stat -c %a)
cut -d= -f1 ~/.config/infrafactory/layer3-aws.env   # AWS_ACCESS_KEY_ID, AWS_SECRET_ACCESS_KEY
(set -a; . ~/.config/infrafactory/layer3-aws.env; set +a
 env -u AWS_PROFILE AWS_CONFIG_FILE=/dev/null AWS_SHARED_CREDENTIALS_FILE=/dev/null \
   aws sts get-caller-identity --region $REGION --query '[Account,Arn]' --output text)
# ACCOUNT_ID  arn:aws:iam::ACCOUNT_ID:user/infrafactory-layer3
```

A new key can take a few seconds to work. Put the two values and `REGION` in the `aws` block of
the config you pass with `--config`, kept out of git:

```yaml
aws:
  region: REGION
  account_id: ACCOUNT_ID
  principal_arn: arn:aws:iam::ACCOUNT_ID:user/infrafactory-layer3
```

**5. The stamp**, written by the admin because the key cannot:

```bash
aws ssm put-parameter --name /infrafactory/layer3/stamp --type String --value $ACCOUNT_ID \
  --region $REGION --profile infrafactory-admin
```

Verify:

```bash
aws ssm describe-parameters --query 'Parameters[].Name' --output text \
  --region $REGION --profile infrafactory-admin                             # /infrafactory/layer3/stamp
aws ssm get-parameter --name /infrafactory/layer3/stamp --query Parameter.Value --output text \
  --region $REGION --profile infrafactory-admin                             # ACCOUNT_ID
```

**6. The SCP**, last, from `MGMT`. `docs/layer3/aws/scp.json` has two Deny statements: every
action but `ec2:*`, `ssm:*` and `sts:*`, and every action where `aws:RequestedRegion` is not
`REGION`. Each is exempted by `ArnNotLike` `aws:PrincipalArn`
`arn:aws:iam::*:role/OrganizationAccountAccessRole`, the role the admin profile assumes, so the
admin can rotate the key and plant the leaks the real-cloud proof needs. That is the one
exemption; `aws_scope_policy_test.go` fails on a second, or on a fourth service.

```bash
ROOT=$(aws organizations list-roots --profile $MGMT --query 'Roots[0].Id' --output text)
aws organizations list-roots --profile $MGMT --output text \
  --query "Roots[0].PolicyTypes[?Type=='SERVICE_CONTROL_POLICY'].Status"
# Only if that did not print ENABLED:
aws organizations enable-policy-type --root-id $ROOT --policy-type SERVICE_CONTROL_POLICY --profile $MGMT

sed -e "s/REGION/$REGION/g" docs/layer3/aws/scp.json > "$TMPDIR/scp.json"
POLICY=$(aws organizations create-policy --name infrafactory-layer3-scope --type SERVICE_CONTROL_POLICY \
  --description 'infrafactory Layer 3 scope' --content "file://$TMPDIR/scp.json" \
  --query Policy.PolicySummary.Id --output text --profile $MGMT)
aws organizations attach-policy --policy-id $POLICY --target-id $ACCOUNT_ID --profile $MGMT
```

Verify. The first command lists `infrafactory-layer3-scope`; with the key, S3 is denied by a
message naming a service control policy, EC2 in `OTHER` is denied, and step 4's
`get-caller-identity` still answers:

```bash
aws organizations list-policies-for-target --target-id $ACCOUNT_ID --filter SERVICE_CONTROL_POLICY \
  --profile $MGMT --query 'Policies[].Name' --output text
(set -a; . ~/.config/infrafactory/layer3-aws.env; set +a
 env -u AWS_PROFILE AWS_CONFIG_FILE=/dev/null AWS_SHARED_CREDENTIALS_FILE=/dev/null \
   aws s3api list-buckets --region $REGION)
(set -a; . ~/.config/infrafactory/layer3-aws.env; set +a
 env -u AWS_PROFILE AWS_CONFIG_FILE=/dev/null AWS_SHARED_CREDENTIALS_FILE=/dev/null \
   aws ec2 describe-vpcs --region $OTHER)
```

## Sibling mocks

Three first-party HTTP-level mocks + one third-party backend live alongside infrafactory:

- **mockway** (`../mockway`, github.com/redscaresu/mockway) — Scaleway mock; runs on `:8080`. Apache-2.0, public.
- **fakegcp** (`../fakegcp`, github.com/redscaresu/fakegcp) — GCP mock; runs on `:8081`. Memorystore + Cloud SQL + GKE + IAM + Storage + DNS + Pub/Sub + Secret Manager + Cloud Run + Cloud KMS (added 2026-05-31 in fakegcp@c7999b5).
- **fakeaws** (`../fakeaws`, github.com/redscaresu/fakeaws) — AWS mock; runs on `:8082`. Ships 10 services across 5 wire formats (IAM, S3, EC2, RDS, DynamoDB, EKS, SQS, Route53, Secrets Manager, KMS).
- **fakegenesys** (`../fakegenesys`, github.com/redscaresu/fakegenesys) — Genesys Cloud CCaaS mock; runs on `:8083`. Ships 15 resources across identity (user, group, location, auth_role, oauth_client), routing (queue + members + skill + wrapupcode + language + utilization singleton), and architect (datatable + rows + user_prompt + flow with multipart + lock/publish state machine + responsemanagement_response + idp_generic singleton). Spec-driven fidelity via `specs/genesys-openapi.json`. Added 2026-06-06 (arc S108-S115); hardened in arc S123-S127 + sibling rollout S128-S130 (v0.2.0).
- **SeaweedFS** (`chrislusf/seaweedfs` container) — third-party S3 backend for `aws_s3_bucket` reads (`terraform-provider-aws` needs the full management surface; fakeaws's stripped S3 handler isn't enough). Runs on `:9090` via `docker-compose.mocks.yml`. Anonymous-mode `ListAllMyBuckets` returns empty even when buckets exist — use HEAD-by-name as the assertion path. Empirical evaluation log in `CONCEPT.md` § "Third-Party Mock Integration" (rejects Adobe S3Mock + Garage + LocalStack + MinIO).
- **s3router** (`cmd/s3router/`, S80) — in-repo reverse-proxy shim that listens on `:9091` and fans S3 traffic across SeaweedFS and fakeaws. `?publicAccessBlock` → fakeaws (SeaweedFS uniquely 501s on that subresource); everything else → SeaweedFS; `PUT/DELETE /<bucket>` fans out to both. `infrafactory.yaml` `s3.url` points at the shim, not SeaweedFS directly. Add a subresource to `fakeawsSubresources` in `main.go` only when a new SeaweedFS 501 surfaces. ADR-0015 § "S80 — S3 backend router" carries the rationale.

**AWS needs fakeaws STS up for Layer 1 and `validate`, not only for apply.** The generated provider
block has no `skip_*` (ADR-0039), so the provider calls STS `GetCallerIdentity` when it configures,
which `tofu plan` does. cloudEnv routes STS to fakeaws's `/sts`; with fakeaws down, Layer 1 fails at
plan naming the fakeaws address.

All four sibling repos are independent public OSS repos on origin/main; cross-repo work cascades (`AGENTS.md` § Operational caveats). The s3router is part of the infrafactory repo, not a sibling.

When extending a sibling mock, mirror the per-bundle PR rule in `../fakeaws/concepts.md` — handler + tests + examples + scenario anchors + coverage_matrix.yaml + `LandedServices` flip all in one slice. The `TestFullCoverageAudit` + `TestRegressionSeedAuditManifestMatchesHandlers` audits in each mock repo enforce this.

**Provider smoke-harness pattern (canonical)**: every sibling fake uses the same `examples/provider_smoke_test.go` pattern — `examples/{working,misconfigured,updates}/<svc>/` directories auto-discovered, real provider binary run against the fake (no real-cloud credentials needed; the provider IS the wire-format validator). Gating is per-repo env var (`MOCKWAY_ENABLE_E2E` / `FAKEGCP_ENABLE_E2E` / `INFRAFACTORY_ENABLE_E2E` / `FAKEGENESYS_ENABLE_E2E`). When spawning a new sibling, copy this harness as-is — it's the cheapest correctness gate we have. Detail in each sibling's `AGENTS.md` § "Provider smoke harness".

**Contract-coverage convention (canonical)**: every sibling fake ships a `handlers/contract_audit_test.go` that enforces the `CRITICAL[<id>]:` / `MUST[<id>]:` docstring → `TestContract_<id>` test pairing. A wire-shape invariant the consuming provider depends on must NOT live as a comment alone — drift becomes a failed `go test`, not a missed code review. New sibling fakes inherit the file as part of the day-one OSS checklist (see `feedback_oss_mature_day_one.md` item 14). Reference impl: `../fakegenesys/handlers/contract_audit_test.go`.

### Fidelity strategies

How each sibling fake decides what wire shape a handler SHOULD return (the smoke harness above is the correctness *gate*, not the discovery *source*). Quick reference for fresh agents:

| Fake | Strategy | Source of truth | Cost per new handler |
|---|---|---|---|
| **mockway** | Spec-driven | `specs/` tree (downloaded Scaleway OpenAPI YAML) | Low — spec gives shape upfront |
| **fakegcp** | Hybrid | GCP discovery docs (referenced ad-hoc, no `specs/` tree) | Medium — semi-discovered per handler |
| **fakeaws** | Reactive | `terraform-provider-aws` source + `TF_LOG=DEBUG` capture | High — ~1-2hr per service before handler is confident |
| **fakegenesys** | Spec-driven (mirrors mockway) | `specs/genesys-openapi.json` (filtered Genesys Swagger 2.0, ~200KB) | Low |

**Recommendation for new fakes**: prefer spec-driven if the target cloud publishes an OpenAPI / Swagger / discovery doc. For sibling extensions: opportunistic upgrade — existing handlers stable, new ones can adopt spec-driven if a spec is available.

Detail in each sibling's `AGENTS.md` § "Fidelity strategy".

## The planning chain

Work is planned top-down, and the user approves each level before the next is made:

1. **HLD** — `/hld <title>` opens a pane on the most capable model (Fable), where the user writes
   `docs/hld/YYYY-MM-DD-<slug>.md` with it; a reviewer attacks the draft before it is `agreed`.
2. **Epics** — `/plan-hld <hld>`: a swarm, led by Fable, splits the HLD into epics that each
   have a **Done when** that could fail, and skeptics and codex check for gaps and overlaps.
3. **Stories** — `/plan-epic <epic>`: a swarm, led by Opus, splits each epic into one-PR stories.
4. **Build** — `scripts/swarm.sh story <slug>`: `ready` stories built in parallel panes.

A story the swarm must not build says so by its `kind`: `lead` (real cloud or credentials; the
lead runs it) or `operator` (a decision or a hand step that is the user's). An open question for
the user is an `operator` story, not a line in `STATUS.md`, so `docs/Board.base`'s **Waiting on
you** view lists it. An epic is done when its last story's PR deletes the epic file and marks it
done in the HLD's `## Epics`; nothing else records the close-out.

Every agent at every level runs in its own herdr pane. The session that runs the chain supervises,
decides model and effort by role, reviews, and merges; it does not do the agents' work itself.

## Model and effort

Every agent's model and effort come from its role, set in one place: `policy()` in
`scripts/swarm.sh` (`scripts/swarm.sh policy <role>` prints it). The principle is to spend on
judgment and save on reading:

- **Design is Fable.** The HLD is co-written at `high`, because the user waits on every turn, and
  attacked at `xhigh` before it is agreed. Splitting an HLD into epics is the one call that shapes
  all the work below it, so it runs on Fable at `xhigh`.
- **Judgment is Opus.** An epic's decomposition into stories decides everything after it, so it
  runs at `xhigh`. Skeptics and the critic run at `high`, because a skeptic is the only gate a
  story passes before it is built.
- **Reading and running is Sonnet.** Surveys read and cite at `high`, since they feed the lead;
  verification that runs tests and reports runs at `medium`, as do docs and chores.
- **Building code is Opus at `high`**, and at `xhigh` for a story marked `risk: high` (Layer 3,
  teardown, safety, hygiene).
- **Codex is the cross-model check**, read-only, because a different model family shares fewer
  blind spots with the one that wrote the plan.
- **Below the design level, Fable is escalation only**: a story that failed twice, or an epic whose
  contradictions no one can reconcile.

A story's `kind` (and `risk`) chooses its role; `kind: lead` (real cloud, credentials) and
`kind: operator` (a human step) are refused by the swarm.

## Scoping an epic

An epic (`docs/epics/<slug>.md`: goal, **Done when**, out of scope, constraints) becomes stories
with `/plan-epic <slug>` (`.claude/commands/plan-epic.md`). It runs as a herdr swarm, every agent
in its own pane: three surveys (where it lands, what constrains it, what overlaps it), one lead
decomposition into one-PR stories with `kind`, `touches` and `depends_on`, a skeptic per story
(capped at five, the rest logged) and a codex pass, then a critic. Agents write their answers to
`.swarm/<epic>/`. The user sees refuted stories and contradictions first and approves before any
story file is written. About ten agents per run, so scope deliberately.

## Parallel agents (herdr)

A wave builds several `ready` stories at once, one agent per herdr pane, each in its own git
worktree; up to four panes a tab. The lead dispatches, reviews and merges; agents never merge.

**Pick the wave.** Only `ready` stories whose `touches` do not overlap. Work that edits a shared
file (`AGENTS.md`, `infrafactory.yaml`, the schema, the hygiene scripts) is done by the lead, alone.

**Start each story** from the lead's pane:

```bash
scripts/swarm.sh story <slug>      # worktree on story/<slug>, a pane, the agent, its brief
scripts/swarm.sh wait <slug>       # in the background: returns when it settles
scripts/swarm.sh watch             # in the background: returns when a story PR needs the lead
```

`wait` returns when an agent goes idle, which can be early (an agent waiting on its own
background shell looks idle). `watch` is what the lead waits on: it exits when a story PR's checks
finish, when it conflicts with main, when its head has had no checks for 10 minutes, or when an
agent is blocked on a prompt.

The brief is the story file plus the standing rules (never merge, no real cloud, codex loop, reply
with the PR URL when CI is green). `herdr agent read <slug> --source recent-unwrapped` shows what an
agent is doing.

**Merge one at a time.** After each merge, merge `main` into every other open wave branch before
trusting its CI; a branch that conflicts gets no CI at all, which looks like a hang.

**Keep with the lead:** real-cloud runs (a restart mid-apply leaves resources behind),
credentials, and anything that changes permissions.

**Clean up.** After each merge, `git worktree remove ../infrafactory-wt/<slug>` (an error if it is
already gone), then `git worktree prune`. `scripts/swarm.sh close build` closes the wave's tabs.

## Demo recording

`asciinema` (CLI demo via `./docs/demo/record.sh`) → `.cast` → `agg` → `.gif`; Playwright (UI demo
via `make demo-ui`) → `.webm` → `gifski` → `.gif`. Build-time only; not advertised in user docs.
