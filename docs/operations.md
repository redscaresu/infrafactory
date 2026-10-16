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
- The key file `~/.config/infrafactory/layer3-aws.env`: exactly `AWS_ACCESS_KEY_ID` and
  `AWS_SECRET_ACCESS_KEY`. Preflight opens it without following a final symlink and refuses it
  unless it is a regular file owned by you, with no group or other permission bits, of at most
  4096 bytes. A refusal never prints a key from the file; it gives the line number and names
  only `AWS_ACCESS_KEY_ID`, `AWS_SECRET_ACCESS_KEY` or `AWS_SESSION_TOKEN`.
- The config's `aws` block (`region`, `account_id`, `principal_arn`), which
  `sts:GetCallerIdentity` must answer exactly.
- Two SSM parameters. The claim, `AWSClaimParameter` (`/infrafactory/layer3/claim`), holds the
  running holder; the key writes and deletes it and nothing else. The stamp, `AWSStampParameter`
  (`/infrafactory/layer3/stamp`), holds the account id; the admin writes it once in step 5 and the
  key can only read it.
- The region's default VPC is gone.

**Proven 2026-09-29** (`aws-scope-planted-leak-proof`): on the real account, the policy in force
was sufficient for the claim, the sweep and the reap: CloudTrail shows 19 of the reap table's 20
mutating actions sent by `infrafactory-layer3`, none refused. One leak per swept collection was
named by the sweep, a held claim refused reap by name, reap removed them all, and the release left
only the stamp. `DetachNetworkInterface` was never needed: reap terminates instances first, and
a NAT gateway's interface cannot be detached, so no attached interface is left to detach.

### Scope setup

Run this once, from the repo root, in one terminal (later steps reuse its variables). It works in
bash and zsh: a variable followed by a colon is written `${ACCOUNT_ID}:`, because zsh reads
`$ACCOUNT_ID:r` as a modifier and silently drops the `:r`. The account id and the key id never enter git (the
repo is public): they stay in shell variables and the local config, and outputs pasted into a PR
are cut to their last four characters. No command prints the secret key.

**Before step 0: a management account.** If you start from a fresh AWS account, it becomes the
Organizations management account. It holds the organization, your login and the rules, and never
runs workloads: a service control policy cannot restrict it. In the console, as root:

1. Put MFA on the root user (account menu → Security credentials).
2. AWS Organizations → **Create an organization**. The account that creates it is its management
   account, permanently.
3. IAM Identity Center → **Enable**, as a **Single-Region** instance (the multi-Region default adds
   a customer-managed KMS key you do not need). Note its region.
4. Identity Center → Users → add yourself; accept the invitation and register MFA.
5. Permission sets → create the predefined **AdministratorAccess**; AWS accounts → assign your user
   to the management account with it.
6. `aws configure sso --profile mgmt`: the start URL is the access portal URL on the Identity
   Center dashboard, the SSO region is step 3's, the default client region `us-east-1`.

Then `aws organizations describe-organization --profile mgmt` should show this account as
`MasterAccountId`. Keep root for emergencies and use this login from here on.

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
role_arn = arn:aws:iam::${ACCOUNT_ID}:role/OrganizationAccountAccessRole
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
`docs/layer3/aws/iam-policy.json`, is the one list of what the key may do, and
`internal/harness/aws_scope_policy_test.go` holds it to exactly that. Its statements, by Sid:
`Identity` lets preflight confirm the key's account and principal; `WriteOnlyTheClaim` and
`ReadTheClaimAndTheStamp` take, read and release the claim and read the stamp, on those two
parameters alone; `ResolveTheAMI` reads the AL2023 public parameter; `Sweep` lists what the
account holds; `Reap` deletes what the sweep finds; `ApplyAndDestroy` is what `tofu apply` and
`tofu destroy` send beyond the reap's deletes; and `TagOnCreate` lets those creates carry the
`default_tags` generation writes, and tags nothing that already exists. Every statement on every
resource (`*`) except `Identity` is pinned to `REGION`.

It is a managed policy attached to the user, because an IAM user's inline policies share a
2,048-character cap that it outgrew.

```bash
aws iam create-user --user-name infrafactory-layer3 --profile infrafactory-admin
sed -e "s/REGION/$REGION/g" -e "s/ACCOUNT_ID/$ACCOUNT_ID/g" docs/layer3/aws/iam-policy.json > "$TMPDIR/p.json"
aws iam create-policy --policy-name infrafactory-layer3-scope --policy-document "file://$TMPDIR/p.json" \
  --profile infrafactory-admin
aws iam attach-user-policy --user-name infrafactory-layer3 \
  --policy-arn arn:aws:iam::${ACCOUNT_ID}:policy/infrafactory-layer3-scope --profile infrafactory-admin
```

After the JSON changes, write `$TMPDIR/p.json` again and make it the default version (IAM keeps five
versions; delete the oldest with `aws iam delete-policy-version` when it holds five):

```bash
aws iam create-policy-version --policy-arn arn:aws:iam::${ACCOUNT_ID}:policy/infrafactory-layer3-scope \
  --policy-document "file://$TMPDIR/p.json" --set-as-default --profile infrafactory-admin
```

A scope set up before 2026-10-10 already has the user, with the old policy inline under the same
name. Move it to the managed policy. Each step runs only if the one before it succeeded, so the
inline policy is deleted only once the managed one is attached:

```bash
sed -e "s/REGION/$REGION/g" -e "s/ACCOUNT_ID/$ACCOUNT_ID/g" docs/layer3/aws/iam-policy.json > "$TMPDIR/p.json" &&
aws iam create-policy --policy-name infrafactory-layer3-scope --policy-document "file://$TMPDIR/p.json" \
  --profile infrafactory-admin &&
aws iam attach-user-policy --user-name infrafactory-layer3 \
  --policy-arn arn:aws:iam::${ACCOUNT_ID}:policy/infrafactory-layer3-scope --profile infrafactory-admin &&
aws iam delete-user-policy --user-name infrafactory-layer3 --policy-name infrafactory-layer3-scope \
  --profile infrafactory-admin
```

Verify with the policy simulator:

```bash
sim() { aws iam simulate-principal-policy --policy-source-arn arn:aws:iam::${ACCOUNT_ID}:user/infrafactory-layer3 \
  --profile infrafactory-admin --query 'EvaluationResults[].EvalDecision' --output text "$@"; }
PARAM=arn:aws:ssm:${REGION}:${ACCOUNT_ID}:parameter/infrafactory/layer3
sim --action-names ssm:PutParameter --resource-arns $PARAM/claim                            # allowed
sim --action-names ssm:PutParameter ssm:DeleteParameter --resource-arns $PARAM/stamp        # implicitDeny implicitDeny
sim --action-names ssm:PutParameter ssm:DeleteParameter --resource-arns $PARAM/other        # implicitDeny implicitDeny
sim --action-names ec2:DescribeVpcs ec2:DeleteVpc \
  --context-entries ContextKeyName=aws:RequestedRegion,ContextKeyValues=$REGION,ContextKeyType=string   # allowed allowed
sim --action-names ec2:DescribeVpcs \
  --context-entries ContextKeyName=aws:RequestedRegion,ContextKeyValues=$OTHER,ContextKeyType=string    # implicitDeny
sim --action-names ec2:RunInstances \
  --context-entries ContextKeyName=aws:RequestedRegion,ContextKeyValues=$REGION,ContextKeyType=string   # allowed
sim --action-names ec2:CreateKeyPair \
  --context-entries ContextKeyName=aws:RequestedRegion,ContextKeyValues=$REGION,ContextKeyType=string   # implicitDeny
sim --action-names ssm:GetParameter \
  --resource-arns arn:aws:ssm:${REGION}::parameter/aws/service/ami-amazon-linux-latest/al2023-ami-kernel-default-x86_64 \
  --context-entries ContextKeyName=aws:RequestedRegion,ContextKeyValues=$REGION,ContextKeyType=string   # allowed
sim --action-names ec2:CreateTags \
  --context-entries ContextKeyName=aws:RequestedRegion,ContextKeyValues=$REGION,ContextKeyType=string \
                    ContextKeyName=ec2:CreateAction,ContextKeyValues=RunInstances,ContextKeyType=string  # allowed
sim --action-names ec2:CreateTags \
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
awk -F= '/^(AWS_ACCESS_KEY_ID|AWS_SECRET_ACCESS_KEY)=/ {print $1; next} {n++} END {print n+0 " other lines"}' ~/.config/infrafactory/layer3-aws.env
# AWS_ACCESS_KEY_ID, AWS_SECRET_ACCESS_KEY, 0 other lines (it never prints a line's text)
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

Verify. The first command lists `infrafactory-layer3-scope`; with the key, S3 and EC2 in `OTHER`
are denied. AWS words the S3 denial as "an explicit deny in an identity-based policy" even when the
SCP denied it, and the user policy denies nothing, so the refusals alone do not prove the SCP: the
simulator's `AllowedByOrganizations` does, after these.

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

The SCP itself, from the simulator (`sim` from step 3); each line prints the decision, then
whether Organizations allows it:

```bash
orgs() { sim --action-names "$1" --context-entries "ContextKeyName=aws:RequestedRegion,ContextKeyValues=$2,ContextKeyType=string" \
  --query 'EvaluationResults[0].[EvalDecision,OrganizationsDecisionDetail.AllowedByOrganizations]'; }
orgs ec2:DescribeVpcs $REGION        # allowed       True
orgs ec2:DescribeVpcs $OTHER         # explicitDeny  False
orgs s3:ListAllMyBuckets $REGION     # explicitDeny  False
orgs iam:CreateUser $REGION          # explicitDeny  False
```

### Getting into the scope by hand

A member account created by the organization has no users and a root user without a password.
Organizations adds `OrganizationAccountAccessRole` (AdministratorAccess) to it, trusting the
management account, so an admin there can assume it; it is also the SCP's one exemption. The
`infrafactory-admin` profile uses it from the CLI. In the console, from the management login:

```
https://signin.aws.amazon.com/switchrole?account=<ACCOUNT_ID>&roleName=OrganizationAccountAccessRole&displayName=infrafactory-layer3
```

(With multi-session support on, "Switch role" is under the arrow beside **Add session**.) Switch
the console to `REGION`. Look, do not build: anything made by hand is a leak to the next sweep.

### Re-running the planted-leak proof

`docs/layer3/aws/plant-leaks.sh` plants one leak per swept collection as the admin role, and
`plant-eip-on-eni.sh` an elastic IP on a standalone interface (the one case that makes reap send
`DisassociateAddress`). Re-run them after any change to `iam-policy.json` or the reap table, then:
`reap --dry-run` must name every id in `planted.env`; claim the scope as another holder and `reap`
must refuse by name; `reap --take-over <holder>` must delete all but `/leak/outside`; delete that
as the admin and `reap --take-over` again must exit 0 with only the stamp left. CloudTrail
(`lookup-events` by `Username=infrafactory-layer3`, events take 5-25 minutes to appear) shows which
reap-table actions were sent and whether any was refused. Last run 2026-09-29, under EUR 0.20.

### Running aws-web-live

What it needs:
- The key file `~/.config/infrafactory/layer3-aws.env`, mode `0600`. infrafactory reads it
  itself; nothing is sourced into the shell.
- A config, passed with `--config` and kept out of git, holding the `aws` block from Scope setup
  **4. The key file** and `validation.layers.sandbox_deploy.enabled: true`. Off by default in
  `infrafactory.yaml`; it is the only switch.

What refuses, in order. Generation and the gate cost an LLM call, so the checks come in two phases:
1. Before generation, once per command: the config fields (`aws.region`, `aws.account_id` and
   `aws.principal_arn` set, the region a real region name), the credentials file and its mode,
   STS answering exactly `aws.account_id` and `aws.principal_arn`, then the AMI lookup. Every one
   of these refusals is reported as stage `aws_ami_resolve`: `test` writes it as a failure with
   check `ami`, while `run` and `generate` only return an error prefixed `aws_ami_resolve:`. Either
   way, read the detail to tell a config field, the key file, STS and the AMI apart. A refusal here ends the command
   before any model call, takes no claim and leaves nothing to reap.
2. After the gate, before apply: the credentials again (config fields, file, STS), then the stamp,
   the absence of a default VPC and the claim, which refuses while another holder holds it and
   names that holder. A credentials refusal fails stage `preflight`, check `credentials`, alone.
   A stamp, default VPC or claim refusal fails stage `aws_scope_claim` with the check that refused
   (`stamp`, `default_vpc` or `claim`) and also fails stage `preflight`, check `credentials`, whose
   detail says the run could not confirm it holds the claim. Read the `aws_scope_claim` failure
   first: a `preflight` failure next to it is that refusal, not a key problem.

What a failure prints: once the claim is taken, any ending short of a clean sweep keeps the claim
and adds stage `aws_scope_claim_kept`, whose detail names the reap command that takes the claim
over, `infrafactory reap scenarios/training/aws-web-live.yaml --take-over <holder>` (with your
`--config` added when you passed one, and the run's holder in place of `<holder>`). That command
destroys what is left and releases the claim; nothing else does. Plain reap refuses a held claim,
and `--take-over` refuses a claim its holder does not hold, so the run names only the form that
works. When it cannot tell whether it holds the claim (a claim put, read or release that failed
without saying who holds it), it names both: `--take-over` if the run holds the claim, plain reap
if no one does. When another run holds the claim, it names that holder and the `--take-over`
naming it, to use once that run has ended. A run that ends short of its target while the claim
may still be its own prints that advice on stderr too. The first Ctrl-C prints both forms at once,
with the run's holder, in case the process dies before its teardown finishes. The teardown then runs
on a fresh, bounded context the Ctrl-C did not cancel: destroy, sweep and, when the scope is empty,
release. A destroy the Ctrl-C cut short runs again: tofu runs in its own process group, so the
terminal's Ctrl-C reaches only infrafactory, whose cancel gives tofu one interrupt to stop on. Signals stay caught from the start of `run`'s loop to the end of its failure arm, auto-learn
between them included, and a second Ctrl-C abandons the teardown, even one pressed before the
teardown has started. Scaleway `test` and `run` tear down the same way, naming plain reap. Once the
teardown ends, the run prints the settled choice: `--take-over` while the run keeps the claim, plain
reap once its sweep released it. A reap whose own claim attempt has an unknown outcome, or a `reap
--take-over` that deleted the old claim but could not take its own, names the reap to run next; a
`--take-over` that finds no claim names plain reap.

The key's policy, `docs/layer3/aws/iam-policy.json`, grants ssm:GetParameter on the AL2023 public
parameter for the AMI resolve, and what the apply and destroy send. A scope whose policy was applied
before 2026-10-10 lacks both, and refuses the first real AMI resolve: apply the policy again as
Scope setup step 3 says.

### Holding aws-web-live for a read

Last run 2026-10-12: held about 7 minutes (one t3.micro, public IPv4), released clean, under €0.01.

To read the live stack's state, `test --hold` keeps the stack up after a passing run. Order:

1. Generate aws-web-live's HCL with `--config <cfg>`. That only reads from AWS (STS, then the public
   AL2023 SSM parameter), and the HCL must carry the AMI resolved for the run as a literal or the
   Layer 3 gate refuses it. `test --hold` then runs the mock layers before any real apply. The
   out-of-git `<cfg>` must be the current `infrafactory.yaml` with Layer 3 on and your `aws` block:
   a stale copy lacks the aws_* `allow_resource_types` and the gate refuses every type.
2. `infrafactory test scenarios/training/aws-web-live.yaml --config <cfg> --hold`. It holds only a
   passing, uninterrupted run whose copy is written; anything else tears down as a plain `test`
   does. If the copy was written before an interrupt, it is kept, and the stage `aws_hold` says so:
   delete it by hand once the stack is gone, before the next generate, run or test.
3. The copy is `.infrafactory/holds/aws-web-live/terraform-live.tfstate`, never committed, and it
   carries the account id. Modes: directory `0700`, file `0600`.
4. While the copy exists, `generate`, `run` and `test` of aws-web-live are refused.
5. While the stack is held, do the read (epic prod-read-seam).
6. Release: `infrafactory --config <cfg> reap scenarios/training/aws-web-live.yaml --take-over <holder> --release-hold`
   keeps the copy the hold wrote (or writes it if none exists), restores the output dir's state
   from the copy if that state is missing, then destroys, sweeps and releases. It refuses, changing
   neither file, if the copy and the output dir's state differ. `<holder>` is in the hold's
   `aws_scope_claim_kept` stage, which prints this command with the holder filled in: copy that command.
7. If the release refuses, the same command without `--release-hold`:
   `infrafactory --config <cfg> reap scenarios/training/aws-web-live.yaml --take-over <holder>`,
   destroys without the copy.
8. The read also uses the copy after the release (prod-read-seam scrubs it with the recording).
   Delete it by hand once the read is done, which lifts step 4's refusal.

Cost bound: one small instance, its root volume and its public IPv4 address, per hour held. There
is no NAT gateway or load balancer. Nothing expires a hold, so release it the same day.

### The reader key file

The account read (epic prod-read-seam) takes its read-only key from its own file, never from
`layer3-aws.env`: the Layer 3 key holds reap's write actions, the reader's must not. infrafactory
reads the file itself (`AWSReaderConfig`); nothing is sourced into the shell.
- The same file checks and refusal rule as `layer3-aws.env` (§ Layer 3 (AWS), what the code reads).
- KEY=VALUE lines: `AWS_ACCESS_KEY_ID` and `AWS_SECRET_ACCESS_KEY`, plus an optional
  `AWS_SESSION_TOKEN` for a temporary key. Any other key, a repeated key and an empty value are
  refused; an empty `AWS_SESSION_TOKEN=` is refused, not read as absent.

### The reader policy

The reader key belongs to a separate read-only IAM user, `infrafactory-reader`, whose one policy,
`infrafactory-reader-read`, is `docs/layer3/aws/reader-policy.json`.
`internal/harness/aws_reader_policy_test.go` holds its actions to exactly the reader table's rows,
and its resources to ARNs in `ACCOUNT_ID` (in `REGION` or regionless), `*` only on a statement of
ec2 actions pinned to `REGION` or on `sts:GetCallerIdentity` alone, and the public
`/aws/service/*` parameters ARN (ADR-0023, amendment of 2026-10-12). Never apply this file to
`infrafactory-layer3` or `infrafactory-layer3-scope`: as a new version of that policy it strips
the Layer 3 key's permissions, and a reader on that user would also hold reap's writes.

The user runs every IAM command in this section, as an admin of the account being read, which can
be prod; the lead never holds those credentials. `REGION`, `ACCOUNT_ID` and `PROFILE` (the admin
profile) are the account being read's; for the Layer 3 account they are Scope setup step 0's
`REGION` and `ACCOUNT_ID`, and `PROFILE=infrafactory-admin`. Each block below stops at its first
failure, and on an empty variable, which would otherwise write `arn:aws:iam:::role/*` into the
policy.

Once, create the user and attach the policy, and nothing else:

```bash
(set -eu; : "${REGION:?}" "${ACCOUNT_ID:?}" "${PROFILE:?}"
  T=$(mktemp); trap 'rm -f "$T"' EXIT
  sed -e "s/REGION/$REGION/g" -e "s/ACCOUNT_ID/$ACCOUNT_ID/g" docs/layer3/aws/reader-policy.json > "$T"
  aws iam create-user --user-name infrafactory-reader --profile "$PROFILE"
  aws iam create-policy --policy-name infrafactory-reader-read --policy-document "file://$T" \
    --profile "$PROFILE"
  aws iam attach-user-policy --user-name infrafactory-reader \
    --policy-arn "arn:aws:iam::${ACCOUNT_ID}:policy/infrafactory-reader-read" --profile "$PROFILE")
```

A rerun of this block fails first at `create-user`, with `EntityAlreadyExists`. If a run stopped
halfway, run the block again without the commands that succeeded.

After `reader-policy.json` changes (a story that adds a reader-table row), the attached policy
still holds the old actions. Write the policy again and make it the default version. IAM keeps at most
five versions: at five, first delete the oldest non-default one with `aws iam delete-policy-version`.
A policy without `ec2:DescribeVolumes` (one written before it joined the reader table) refuses the
read of each instance's volumes, which aws_instance's read needs: write it again with the block
below.

```bash
(set -eu; : "${REGION:?}" "${ACCOUNT_ID:?}" "${PROFILE:?}"
  T=$(mktemp); trap 'rm -f "$T"' EXIT
  sed -e "s/REGION/$REGION/g" -e "s/ACCOUNT_ID/$ACCOUNT_ID/g" docs/layer3/aws/reader-policy.json > "$T"
  aws iam create-policy-version --policy-arn "arn:aws:iam::${ACCOUNT_ID}:policy/infrafactory-reader-read" \
    --policy-document "file://$T" --set-as-default --profile "$PROFILE")
```

Once, create the user's access key and write it to the key file without printing it. The block
refuses an existing key file and writes nothing unless the key was created:

```bash
mkdir -p ~/.config/infrafactory && (set -eu; : "${PROFILE:?}"; umask 077; F=~/.config/infrafactory/aws-reader.env
  [ ! -e "$F" ]
  KEY=$(aws iam create-access-key --user-name infrafactory-reader --profile "$PROFILE" \
    --query 'AccessKey.[AccessKeyId,SecretAccessKey]' --output text)
  [ -n "$KEY" ]
  printf '%s\n' "$KEY" | awk '{printf "AWS_ACCESS_KEY_ID=%s\nAWS_SECRET_ACCESS_KEY=%s\n",$1,$2}' > "$F")
```

The lead deletes this file after each read, so before handing over its path the user copies the
key into their secret store by importing the file into their password manager directly, never
through the clipboard (it can sync to other devices and is kept by clipboard-history tools). The
key is never echoed in a terminal or transcript anyone else can see.

The IAM user and its key stay between rehearsals; only the local key file comes and goes
(ADR-0023, amendment of 2026-10-12). For each later read, the user writes the key file again from
the secret store: mode 0600, the same two lines, `AWS_ACCESS_KEY_ID=…` and
`AWS_SECRET_ACCESS_KEY=…`. If the key is lost, the user deletes it by id and runs the create block
again: `aws iam list-access-keys --user-name infrafactory-reader --profile "$PROFILE"` shows its id
and never its secret, then
`aws iam delete-access-key --user-name infrafactory-reader --access-key-id <id> --profile "$PROFILE"`.

For each read, the user gives the lead the key file's path, and the lead checks only this:

```bash
KEY_FILE=<the path the user gave>
stat -f %Lp "$KEY_FILE"   # 600 (on Linux: stat -c %a)
awk -F= '{ if ($1 ~ /^AWS_(ACCESS_KEY_ID|SECRET_ACCESS_KEY|SESSION_TOKEN)$/) print $1; else print "line " NR ": not a key name" }' "$KEY_FILE"
```

The second command prints only a key name it recognises, and for any other line only its number,
so a malformed line never puts the secret in the lead's transcript. It must print exactly
`AWS_ACCESS_KEY_ID` and `AWS_SECRET_ACCESS_KEY`.

IAM is eventually consistent, and nothing retries a new key: the read retries only a throttled
request, and the identity check stops at its first error. So on the first read after the key is
created, if the identity check fails with `InvalidClientTokenId`, the lead waits a few seconds and
runs the read once more; a second failure is a bad key.

After each read, once the scrub has produced its fixtures, the lead deletes the local key file, the
raw recording and the pre-destroy state, and checks the key file is absent
(`test ! -e "$KEY_FILE"`). The IAM user and its key stay. Only the scrubbed fixtures are committed.

In the Layer 3 account the SCP (`docs/layer3/aws/scp.json`, `DenyEveryServiceButEC2SSMSTS`) denies
every `iam:*` action to everyone but the admin role, so the policy's `iam:` rows are unreachable
there: the IAM readers are proved against fakeaws only.

### The fakeaws instance

The rehearsal runs its own fakeaws, started with these flags (`fakeaws/cmd/fakeaws/main.go`):

- `-bind <host>` sets the listen host. `-bind 127.0.0.1` refuses every other host. Empty, the
  default, listens on all interfaces, as the Makefile and CI runs always have.
- `-admin-token <t>`, or `FAKEAWS_ADMIN_TOKEN` (the flag wins), makes every `/mock/*` request need
  `Authorization: Bearer <t>`. A `/mock/*` request whose `Origin` is present and not loopback is 403
  before the token is read; an absent `Origin` is allowed. Without a token, `/mock` is unchanged. Prefer
  the env var: a flag value shows in `ps`.
  The rehearsal passes the token by env (`FAKEAWS_ADMIN_TOKEN`), never on argv.
- `-record-requests` keeps an in-memory log of every AWS request. It is refused without a token,
  because the log holds request values. `GET /mock/requests` returns it and `POST /mock/requests/clear`
  empties it; `POST /mock/reset` empties it too. `/mock/state`'s `audit` lists the same requests with
  service, protocol, action, status and class only, no params.

Each logged request carries one class:

| Class | Meaning |
|---|---|
| `answered-from-rows` | Answered from stored rows, including an empty answer read from rows. |
| `not-modelled-404` | The default arm for an action fakeaws does not model. Its 404 looks like a missing row's, so only this mark tells them apart. |
| `unimplemented-501` | The router's 501 for an operation fakeaws does not implement. |
| `filter-refused` | A site refuses what real AWS would accept but fakeaws does not model: a filter, a parameter, a tag resource type, or a required-parameter check stricter than AWS's. The answer is the site's refusal (EC2 501, KMS 501 for DescribeKey by alias, RDS and Route53 400, SSM 501). A refusal AWS makes too, or a filter or parameter fakeaws ignores, stays `answered-from-rows`. |
| `empty-collection` | A Describe of a resource type fakeaws never creates (EC2). Other always-empty answers stay `answered-from-rows`. |

A file-backed database (`-db <path>`), its `-wal` and `-shm`, a snapshot and a restored copy are created
0600, owner-only. An existing database keeps its mode.

## Rehearsal

The `infrafactory rehearse` family (ADR-0042) reads no `infrafactory.yaml`: the account and region
come from flags only, never from the config's `aws:` block or the environment. Its commands are
`read`, `prepare`, the tofu passthrough actions (`plan`, `apply`, `destroy`, `import`, `state mv`)
and `end`.

### Read

```
infrafactory rehearse read --account <id> [--principal <arn>] --region <r> \
  --credentials <file> --read-set <file> --out <dir>
```

One read step (ADR-0041). Nothing else calls it: no run, validate, UI or CI path. It runs from
inside a git repository, which `<repo>` below names, and takes relative paths from the current
directory.

- `--account` (12 digits) and `--region` name the account read; `--principal` is optional, and when
  given the key's ARN must match it exactly.
- `--credentials` is the reader key file (§ The reader key file). The command reads it itself and
  holds the key in its own process only; nothing is sourced or exported.
- `--read-set` is a JSON array of ADR-0041 read-set entries. An entry outside the reader table
  refuses the read, naming every such entry, with nothing sent.
- `--out` must resolve, symlinks included, under `<repo>/.infrafactory/`, and not into
  `.infrafactory/runs` or `.infrafactory/live`, which the UI serves. Anything else is refused
  before any request. Every directory from `.infrafactory` down to `--out` is made `0700` where
  missing; an existing one below `.infrafactory` that is open to group or other, or a symlink, is
  refused, never chmodded. `--out` gets a `.gitignore` of `*` before the read, so git ignores it in
  any repository. The recorded read is `<out>/read.json`, `0600`, overwritten by the next read into
  the same `--out`.

In order: `sts:GetCallerIdentity` (a wrong account or principal stops the read after that one
request), the EC2 DryRun preflight (any answer but `UnauthorizedOperation` stops it, naming the
action), then each entry, then the follow-ups: a touched VPC's EC2 surroundings, a role's policies,
and the image a public AMI parameter names. A refusal (`Refused:`, json `refusal`) exits 1
and writes no recorded read; a missing or malformed flag exits 2. If the account was read but
`read.json` could not be written, it says so (`Not written:`, json `write_failure`, with the
exchange count) and exits 1: the read happened, and must be run again. The raw `read.json` carries
the account id and the read's values: it is never committed; `cmd/aws-read-scrub` turns it into
fixtures that may be.

### Prepare

```
infrafactory rehearse prepare <dir> --account <id> --region <r> [--var-file <file>] [--fakeaws-bin <path>]
```

Copies the code in `<dir>` into a new rehearsal, gates the copy and initialises it. No tofu
command runs on a configuration the gate refused. Run from inside the git repository that holds
`<dir>`. Nothing is planned or applied; a success report names the rehearsal's id, its work dir,
the URL of its fakeaws (`fakeaws_url`, never the admin token), the `hashicorp/aws` version the lock
pins and any warnings.

- `-h`, `--help` prints the usage. `--output human|json` is global and picks the report's format; `--config` is global and ignored, since prepare reads no config.
- `<dir>` must resolve, symlinks included, inside the current directory's repository.
- `--account` (12 digits) and `--region` are required. The gate checks the code against `--region`:
  every `provider "aws"` block must set `region`, as a string literal or a root `var.<name>` the
  var files resolve, and it must equal `--region`. A module's region must be a literal. A root with
  no `provider "aws"` block without an alias is refused, since tofu's implied empty one has no region.
- `--var-file` is a `.tfvars` file, copied to `<rehearsal>/vars.tfvars` outside the work dir and
  parsed by the gate. It overrides the code's `terraform.tfvars` and `*.auto.tfvars`. A `.json`
  var file, or anything but a regular file, is refused. Passthrough passes this copy to tofu
  itself (§ Passthrough).
- `--fakeaws-bin` is an absolute path to a fakeaws binary built at infrafactory's pin; an empty or
  relative one is a usage error. Unset, prepare installs the pin into the user cache. Any other
  binary is refused.

**The fakeaws binary.** Unset `--fakeaws-bin`, prepare runs `go install github.com/redscaresu/fakeaws/cmd/fakeaws@<pin>`
into `<UserCacheDir>/infrafactory/fakeaws/<pin>`, once per pin. It needs Go and the module proxy
the first time; later prepares reuse the install. Whichever binary is used, its build info must
name the pin, or prepare refuses it.

**The overrides and the record.** Prepare writes two files into each root of the copy:
`infrafactory_override.tf` (a `backend "local" {}` naming no `path`) and `infrafactory_aws_override.tf`
(one `provider "aws"` block per root block, with fake keys and an `endpoints` block naming every
key of the locked provider: the rehearsal's fakeaws for modelled services, a dead loopback address
for the rest, S3 included). It records pid, port, token and roots in `<id>/rehearsal.json`, `0600`;
the token reaches fakeaws only as `FAKEAWS_ADMIN_TOKEN` in its environment, never on argv, and is never printed. The report's `fakeaws_url` is where the rehearsal's fakeaws listens.
A dead fakeaws leaves the override pointing at a closed port, so tofu fails there: run prepare again,
since there is no `rehearse start`. Naming every endpoints key makes every plan print
alias-conflict warnings (`Only one of the following attributes should be set`); they are expected.

**Layout.** `<repo>/.infrafactory/rehearsals/<id>/` holds `work/` (the copy of the lowest directory holding `<dir>` and every local module it
calls, at its path relative to the repository: with `../../modules/x` that can be the repository
root, unrelated files included, which the gate does not read), `data/` (`TF_DATA_DIR`),
`home/` (an empty HOME), `state/` and `vars.tfvars`. The `rehearsals` dir has a `.gitignore` of
`*`. Directories are `0700` and files `0600`, except a file with an execute bit (a provider
binary), which is `0700`. The copy leaves out `.terraform`, `.git`, `.infrafactory` and any name
containing `.tfstate`; a symlink or other non-regular file in the copy is refused. A refusal removes
the half-made rehearsal.

**Order.** Gate over the copy, a network-only first init (`-backend=false`), the gate again over
every module the init wrote to `modules.json`, then an offline second init (`-lockfile=readonly`,
`-plugin-dir` at the providers the first one installed, both proxies at a dead address). Prepare
checks that the dead address refuses a connection and refuses the second init on any other result.
Both inits run with a replaced environment: PATH and only named variables, so `TF_ENCRYPTION`,
`TF_CLI_CONFIG_FILE`, `TF_VAR_*`, `TF_CLI_ARGS*` and AWS credential variables never reach tofu.
Each tofu command has a 10-minute deadline; SIGINT cancels, removes the rehearsal and exits 1.

**What is refused, and why.** A refusal (`Refused:`, json `refusal`) exits 1; a missing or
malformed flag exits 2.

- **Encryption.** An `encryption` block, a `cloud {}` block and any other `terraform {}` content
  than `required_providers`, `required_version` and the root's `backend`. `TF_ENCRYPTION` cannot be
  refused in the code, so replace mode never passes it; the suffix rule below closes the files
  that could carry the same settings.
- **File suffixes tofu reads and the gate does not parse:** `.tf.json`, `.tofu`, `.tofu.json`,
  `.tfvars.json`, `.tftest.hcl`, `.tftest.json`, `.tofutest.hcl`, `.tofutest.json`, `.tfmock.hcl`,
  and a `tests` dir. They are refused by name, never skipped. Override files (`override.tf*`,
  `*_override.tf*`) are refused: they merge after infrafactory's overrides and can undo them.
- **Modules.** Only a literal `./` or `../` source inside the repository is admitted. Every other
  source is refused, registry modules included: a registry module is fetched by running `git`
  against github.com, and the system git config can carry a keychain credential helper, so the
  download could use a credential the rehearsal must never touch. A local module takes no
  `version`; a source holding a backslash, crossing a symlink or forming a cycle is refused. A
  `provider "aws"` block inside a child module is refused, because the root override does not
  reach it.
- **`terraform.d`** is refused: tofu reads it as a local provider mirror.
- **Blocks and providers.** Provisioners, connections, `http` and `external` data sources,
  `terraform_remote_state` and providers other than `hashicorp/{aws, random, null, tls, time,
  cloudinit, local, archive}` are refused, as are `provider "aws"` arguments outside the allowlist
  and a file path that is absolute, computed or escapes the module.
- **The lock.** The root's `.terraform.lock.hcl` is required: a missing or unparseable lock is
  refused, because the rehearsal takes every provider version from it. It must pin `hashicorp/aws`,
  one version per provider, from `registry.opentofu.org` or `registry.terraform.io`. The first init
  may not change what it pins. A version must be an exact X.Y.Z release, and `hashicorp/aws` may be
  no later than 5.100.0 and of major 5 (`hashicorp/local` 2.9.1, `hashicorp/archive` 2.8.1, same
  rule): the gate's table of the arguments that read a local file is that release's, so a later or
  other-major release could add one the gate cannot name. A prerelease is refused too.
- **Warnings** (the prepare still succeeds): a `registry.terraform.io` aws entry, which Terraform
  wrote, where `moved`, `import`, `removed` and `ephemeral` semantics may differ under tofu; and a
  lock pinning an earlier 5.x release of `hashicorp/aws` than 5.100.0, the release fakeaws is proven against. The line
  names the version, so a failure under it is filed as a fakeaws story.

### Passthrough

```
infrafactory rehearse plan|apply|destroy|import|state mv <id> [-- tofu args]
```

Runs one tofu action in rehearsal `<id>`, against its private fakeaws, with tofu's output live on
your terminal. Run from inside the repository that holds the rehearsal. Tofu's own arguments go
after `--` (`rehearse plan <id> -- -out=p`); infrafactory's flags go before it. Exactly one `<id>`
goes before `--`, and a second `--` among tofu's args is refused.

- **The actions** are `plan`, `apply`, `destroy`, `import` and `state mv`. Any other is a usage
  error, exit 2, naming it and listing the allowed ones (`rehearse state rm`, `rehearse refresh`).
  `console` is refused with them: `console -input=false` still reads an unset variable from stdin.
  `rehearse state` alone prints help and exits 0.
- **Each action allows only its own tofu flags.** Any other is refused by name, exit 1, before tofu
  starts. That covers `-var` and `-var-file` (the rehearsal passes prepare's copied var file and no
  other), `-input`, and every flag that names a state, backup, config, chdir, plugin or output path.
  `import` and `state mv` take two positional arguments; the rest take none.
- **Injected arguments.** `plan`, `apply`, `destroy` and `import` get `-input=false`, then
  `-var-file=<id>/vars.tfvars` when `prepare --var-file` made one, before your args. Tofu never
  prompts for a variable.
- **apply and destroy need `-- -auto-approve`.** Nothing injects it, and tofu gets no stdin, so
  without it tofu fails at its approval prompt with EOF, exit 1. apply takes no saved plan file,
  and one is refused: the re-gate checks the source, not the configuration inside a plan, so apply
  runs from source. `plan -out` is allowed, to inspect with tofu yourself.
- **`-out` stays inside the work dir.** A relative path is joined onto the root, tofu's cwd. No `..`
  element is allowed; the dir must resolve inside the copy; the file must be absent or a regular
  file with one link, with a name and path the copy's checks admit.
- **One root.** Tofu runs in the record's only root. A record with another number of roots is
  refused and the roots named; there is no `--root` flag.
- **The gate runs first.** Before every command the rehearsal's fakeaws must pass `end`'s proof
  (`/healthz` and the token-guarded `/mock/requests`), and the re-gate runs over the copy,
  admitting infrafactory's override files only when byte-identical to what it writes. A refusal
  stops the command before tofu starts, exit 1. Tofu runs under the walls' replace-mode
  environment, so `TF_VAR_*`, `TF_CLI_ARGS*` and AWS credential variables never reach it.
- **Output is live.** Tofu's stdout and stderr stream to the terminal. In human mode nothing is
  printed after tofu exits. `--output json` is a usage error, exit 2, before tofu runs.
- **Exit codes.** Tofu's own code passes through when tofu ran to completion, so `plan
  -detailed-exitcode` gives 0, 1 or 2 as tofu does. An interrupt, the deadline, tofu not running
  (exit code -1) or a failure to tighten the rehearsal's modes afterwards give exit 1, whatever tofu
  returned. **The two 2s:** infrafactory's 2 is a usage error decided from your command line alone,
  before tofu starts, with a usage message and no tofu output. A 2 after tofu printed anything is
  tofu's.
- **One Ctrl-C** (or SIGTERM or SIGHUP) stops tofu: its process group gets one SIGINT, then SIGKILL
  after a short fallback. Nothing reports success after an interrupt.
- **Deadline.** One run has 2 hours, set against Prepare's 10 minutes per tofu command: an apply
  can legitimately run long, an init cannot. The same stop as Ctrl-C applies when it passes.
- **After a failure the rehearsal stays.** A failed or interrupted run removes nothing, so you can
  read the work dir and run again. The modes are tightened after every run. Only `end` deletes it.

### End

```
infrafactory rehearse end <id>
```

Stops the rehearsal's fakeaws and deletes the rehearsal. `-h`, `--help` prints the usage; it takes
no other flag. Run from inside the repository that holds the rehearsal. Run it after the last
passthrough command: passthrough never deletes the rehearsal, and its state goes with it.

- **The token check comes before any signal.** `end` reads `rehearsal.json`, and signals the
  recorded pid only after `/healthz` answers 200 and the token-guarded `GET /mock/requests`
  answers on the recorded port. Any other answer, or a record that is missing or unreadable,
  signals nothing, says why under `Fakeaws:` and still deletes. The signal is SIGTERM, then
  SIGKILL to the process group.
- **What it deletes:** `<repo>/.infrafactory/rehearsals/<id>/` whole: work dirs, the state-config
  copy, the fakeaws db with `-wal` and `-shm`, the snapshot, recorded reads and state copies. It
  lists each path it removed. Nothing under `.infrafactory/runs` or `live` is touched.
- **It is idempotent.** A missing rehearsal exits 0 and creates nothing; a bad id, or an `<id>`
  that is a symlink, exits 2.
- **`stop failed: pid P port N`** means the port still answered after SIGKILL. `end` exits 1 and
  deletes nothing, so `rehearsal.json` stays; run `end` again once the process is gone.
- **A cancel** before the stop refuses with nothing signalled or deleted; once the stop has
  begun, `end` finishes.
- A `rehearse read --out` outside `<id>/reads` is not the rehearsal's and is not deleted.

## Sibling mocks

Three first-party HTTP-level mocks + one third-party backend live alongside infrafactory:

- **mockway** (`../mockway`, github.com/redscaresu/mockway) — Scaleway mock; runs on `:8080`. Apache-2.0, public.
- **fakegcp** (`../fakegcp`, github.com/redscaresu/fakegcp) — GCP mock; runs on `:8081`. Memorystore + Cloud SQL + GKE + IAM + Storage + DNS + Pub/Sub + Secret Manager + Cloud Run + Cloud KMS (added 2026-05-31 in fakegcp@c7999b5).
- **fakeaws** (`../fakeaws`, github.com/redscaresu/fakeaws) — AWS mock; runs on `:8082`. Ships 12 services across 5 wire formats (IAM, S3, EC2, RDS, DynamoDB, EKS, SQS, Route53, Secrets Manager, KMS, SSM, STS).
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

## The swarm (swarm-dev)

Work is planned and built with [swarm-dev](https://github.com/redscaresu/swarm-dev), a Claude Code
plugin: `/hld`, `/plan-hld`, `/plan-epic`, `/swarm` (the whole chain after the HLD, resuming
wherever it stopped), and `swarm.sh` for panes, builds and conductors. Its
`docs/method.md` is the method: the planning chain, the model and effort per role, scoping an epic,
and building with a conductor. Install it once:

```bash
claude plugin marketplace add redscaresu/swarm-dev
claude plugin install swarm-dev@swarm-dev
```

What is infrafactory's own:

- **Builder rules** are in `.claude/swarm/brief.md`, which `swarm.sh` appends to every story
  brief: no `~/.config/infrafactory/*.env`, no deploy, the `ADR: none` trailer, commit and PR
  trailers.
- **Shared files** that one story at a time may touch, and only the lead: `AGENTS.md`, `STATUS.md`,
  `infrafactory.yaml`, `scenario.schema.json` and `scripts/check_doc_hygiene*`.
- **`risk: high`** covers Layer 3, teardown, safety and hygiene paths.
- **Keep with the lead:** real-cloud runs (a restart mid-apply leaves resources behind),
  credentials, and anything that changes permissions. `kind: lead` stories are these.
- **Story repos:** a story with `repo: <name>` builds in a worktree of `../<name>` (`fakeaws`,
  `mockway`, ...); story worktrees live in `../infrafactory-wt/<slug>`.

## Demo recording

`asciinema` (CLI demo via `./docs/demo/record.sh`) → `.cast` → `agg` → `.gif`; Playwright (UI demo
via `make demo-ui`) → `.webm` → `gifski` → `.gif`. Build-time only; not advertised in user docs.
