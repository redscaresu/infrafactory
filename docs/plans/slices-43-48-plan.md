# Plan: Slices 43-48 — fakeaws (AWS API mock + infrafactory integration)

## Context

Slices 43-48 build a third sibling mock for AWS, modelled after mockway (Scaleway) and fakegcp (GCP). LocalStack consolidated into a paid product in April 2026; fakeaws keeps the freedom-to-modify story alive for infrafactory's AWS scenarios.

The full design lives in `/Users/ehsanashouri/go/src/github.com/redscaresu/fakeaws/concepts.md`. This plan covers the per-slice deliverables and exit criteria so the work can be picked up incrementally by any agent.

## Quick Reference

| Key | Value |
|---|---|
| Slices | 43–48 |
| Ticket IDs | S43-T1 through S48-T8 |
| Total tickets | 60 |
| Depends on | Slice 42 (multi-cloud UI), Slice 36 (GCP infra) |
| Out-of-tree work | New repo at `/Users/ehsanashouri/go/src/github.com/redscaresu/fakeaws` |

## Quality bar

Non-negotiable; every slice exit gate enforces this. Detail in `fakeaws/concepts.md` § "Quality guarantees".

The bar that mockway and fakegcp set:
- 280+ handler tests in mockway, 90+ tests + 33 codex review passes for fakegcp.
- Every contract pinned by a test before the feature is considered shipped.
- Every wire shape driven through the live `hashicorp/aws` provider via `tofu apply → plan -detailed-exitcode → destroy` at least once.
- Two consecutive codex `NOTHING_TO_IMPROVE` review passes scoped to each phase's diff.

The mechanism that landed that bar without paying for it post-hoc: gates wired into the workflow *before* writing handler code.

### Per-phase exit gates (S43–S47, ten gates each)

1. CRUD test for every resource in scope (Create → Get → List → Update → Delete → 404).
2. FK violation tests for every cross-resource reference (same-account + cross-account).
3. Cascade / dependent-delete tests for every parent-child FK.
4. Update-path FK tests (post-merge validation, mirror of fakegcp pass 28).
5. State-machine tests where applicable (terminal-state, status transitions).
6. `examples/working/<service>` applies + plans no-op + destroys cleanly.
7. `examples/misconfigured/<service>` fails with the correct error code through tofu.
8. `examples/updates/<service>` reaches v2 in-place via `v1.tfvars → v2.tfvars`.
9. `TestE2E_AWS_<Service>` gated runner (mirror of `runGCPServiceScenario`) green.
10. Two consecutive codex `NOTHING_TO_IMPROVE` passes scoped to phase diff.

### Phase 6 (S48): codex review iteration loop

Slice 48 is dedicated to the same loop that landed fakegcp. Budget: 20–35 passes based on fakegcp's 33. Restart count on any `BLOCKING:` finding; only `NOTHING_TO_IMPROVE` advances the counter. Cross-pollinate findings back to mockway/fakegcp where they apply.

### Standing patterns to seed `regression_test.go` on day one

The pre-seeded coverage drawn from fakegcp's 33-pass findings:

- Cross-account FK rejection (`resolveSameAccountName`)
- Wrong-collection FK rejection (same-account paths)
- Relative-path wrong-collection rejection
- Subnet/VPC pairing on instance / cluster create
- Post-merge PATCH validation
- Bare-name region scoping (zone-derived for instance, location-derived for cluster)
- Region-vs-zone heuristic
- `/mock/reset` cache-baseline lifecycle
- Terminal-state refuses transitions (DESTROYED versions, terminated instances)
- Distinct 409 sentinels (`ErrInUse` vs `ErrTerminalState`)
- Hosted-zone delete refused if non-empty
- Tombstone semantics on parent delete
- Resource-existence gate on child handlers (`requireParentX`)
- Server-stamped fields never trusted from client (PATCH skip-list)

Detail and code references in `fakeaws/concepts.md` § "Standing patterns to seed regression_test.go on day one".

---

## Slice 43: Foundation — IAM + S3 + infrafactory wiring

Boot the repo. Land `awsproto/`, IAM as the foundational service every other resource depends on, S3 with bucket-level CRUD (no object payload store), and the infrafactory hooks (`StartFakeaws`, schema enum, `cloudMockStateRouter` dispatch) so the next slice doesn't re-derive the wiring.

### Tickets

| id | title | priority | deps |
|---|---|---|---|
| S43-T1 | fakeaws: repo scaffold (cmd, handlers, models, testutil, Makefile, README, .gitleaks.toml, AGENTS.md, go.mod) — mirror of fakegcp layout | P1 | — |
| S43-T2 | fakeaws: `awsproto/` helper package — XML response writer, JSON 1.0/1.1 helpers, x-amz-target parser, query-RPC parser, `ErrInUse`/`ErrTerminalState` → AWS error wire mapping | P1 | S43-T1 |
| S43-T3 | fakeaws: `repository/repository.go` skeleton — modernc.org/sqlite, schema migrate, FK enforcement, snapshot/restore lifecycle, `models.Err*` sentinels | P1 | S43-T1 |
| S43-T4 | fakeaws: `handlers/admin.go` (/mock/reset, /mock/snapshot, /mock/restore, /mock/state, /mock/state/{service}) + admin_test.go | P1 | S43-T3 |
| S43-T5 | fakeaws: repository support for s3_buckets + s3_bucket_configs (versioning / encryption / policy / public-access-block / ownership-controls) | P1 | S43-T3 |
| S43-T6 | fakeaws: `handlers/s3.go` (Bucket CRUD + versioning + encryption + tagging + policy; Object PUT/HEAD/DELETE/List, payload discarded) + handlers_test coverage | P1 | S43-T5 |
| S43-T7 | infrafactory: `scenario.schema.json` adds aws cloud enum; `cloudMockStateRouter` dispatches aws to fakeaws; `StartFakeaws` helper in `internal/e2e/helpers.go` (mirror of `StartFakegcp`) | P1 | S43-T1 |
| S43-T8 | fakeaws: `handlers/iam.go` (Role / Policy / InstanceProfile / User / AccessKey CRUD + AttachRolePolicy / DetachRolePolicy) + handlers_test coverage | P1 | S43-T6 |
| S43-T9 | fakeaws: `examples/working/iam_role` + `working/s3_bucket`; smoke through tofu apply→destroy | P1 | S43-T8 |
| S43-T10 | fakeaws Phase 1: gated `TestE2E_AWS_IAM` + `TestE2E_AWS_S3` wired up in `infrafactory/internal/e2e` | P1 | S43-T8, S43-T9 |

### Acceptance criteria

- S43-T1: `go build ./cmd/fakeaws` succeeds. `go test ./...` passes (no tests yet, but the skeleton compiles).
- S43-T2: `awsproto.WriteAWSError(w, ErrInUse)` produces the correct wire shape per protocol (XML, JSON 1.0, JSON 1.1, REST). Helpers tested in `awsproto/awsproto_test.go`.
- S43-T3: Reset/snapshot/restore lifecycle pinned by `TestRepositoryAdminLifecycle` (analog of fakegcp's `TestResetClearsDNSChangeCache`).
- S43-T4: `/mock/state` returns the full SQLite contents as JSON keyed by service. Reset clears all tables and the snapshot baseline.
- S43-T5–T6: Working bucket Create+Get+List+Versioning+Tagging+Encryption+Delete via testutil. FK violation tests where applicable. Object endpoints accept PUT, return ETag, but discard the payload (documented in `handlers/s3.go` header comment).
- S43-T7: `scenario.schema.json` validates a minimal `cloud: aws` scenario. `StartFakeaws` boots fakeaws on a free port, ready to receive requests.
- S43-T8: IAM CRUD complete with FK validation (RolePolicyAttachment can't reference missing role/policy).
- S43-T9: `cd examples/working/iam_role && tofu init && tofu apply && tofu plan -detailed-exitcode && tofu destroy` succeeds end-to-end against fakeaws.
- S43-T10: `INFRAFACTORY_ENABLE_E2E=1 go test ./internal/e2e -run TestE2E_AWS_IAM` and `TestE2E_AWS_S3` both green.

### Key files

- `fakeaws/cmd/fakeaws/main.go`
- `fakeaws/handlers/{handlers,admin,iam,s3}.go`
- `fakeaws/handlers/awsproto/`
- `fakeaws/repository/repository.go`
- `fakeaws/models/models.go`
- `fakeaws/testutil/testutil.go`
- `fakeaws/examples/working/{iam_role,s3_bucket}/`
- `infrafactory/internal/e2e/helpers.go` (extend `StartFakegcp` pattern)
- `infrafactory/internal/e2e/aws_services_test.go` (new)
- `infrafactory/scenario.schema.json` (cloud enum + AWS resource defs)

---

## Slice 44: Networking + compute (EC2)

Land VPCs, subnets, security groups, route tables, internet gateways, EIPs, NAT gateways, and finally instances. EC2 is XL scope but well-defined; FK chains are the load-bearing complexity.

### Tickets

| id | title | priority | deps |
|---|---|---|---|
| S44-T1 | Phase 2 design note: EC2 query-RPC wire format vs SDK expectations; FK chain VPC→Subnet→Instance and SG→Instance | P1 | S43-T2 |
| S44-T2 | fakeaws: `awsproto` query-RPC parser + XML response writer for EC2 (Action=Foo / Version=YYYY-MM-DD) | P1 | S43-T2 |
| S44-T3 | fakeaws: repository support for ec2_vpcs + ec2_subnets + ec2_security_groups + ec2_route_tables + ec2_internet_gateways + ec2_eips with FK | P1 | S43-T6 |
| S44-T4 | fakeaws: `handlers/ec2_network.go` (VPC, Subnet, InternetGateway, RouteTable, Route, EIP, NAT gateway) | P1 | S44-T3 |
| S44-T5 | fakeaws: `handlers/ec2_security.go` (SecurityGroup + ingress/egress rules, AuthorizeSecurityGroupIngress / Revoke...) | P1 | S44-T4 |
| S44-T6 | fakeaws: repository support for ec2_instances + ec2_key_pairs + ec2_amis (read-only fixture set) | P1 | S44-T4 |
| S44-T7 | fakeaws: `handlers/ec2_instance.go` (Instance create/describe/modify/terminate, KeyPair, AMI fixture data) | P1 | S44-T6 |
| S44-T8 | fakeaws: handlers_test for EC2 (CRUD across all resources, FK validation, cascade, instance state transitions) | P1 | S44-T7 |
| S44-T9 | fakeaws: regression coverage for instance create/modify/terminate + ENI attachment + EIP lifecycle | P1 | S44-T7, S44-T8 |
| S44-T10 | infrafactory: `scenarios/training/aws-vpc-network.yaml` + `aws-instance.yaml` + loader update | P1 | S44-T9, S43-T7 |
| S44-T11 | fakeaws: `examples/working/basic_instance` + `working/vpc_network` + `misconfigured/instance_missing_subnet` + `updates/update_security_group_rules` | P1 | S44-T9 |
| S44-T12 | fakeaws Phase 2: gated `TestE2E_AWS_VPC` + `TestE2E_AWS_Instance` + `TestE2E_AWS_SecurityGroup` in infrafactory | P1 | S44-T9, S44-T11 |

### Acceptance criteria

- S44-T1 design note: pinned in `fakeaws/PLAN.md` covering query-RPC body parsing, XML response writing, and the four primary FK chains (VPC→Subnet, VPC→InternetGateway, VPC→SecurityGroup, Subnet→Instance).
- All ten phase exit gates from "Quality bar" green for S44.
- `aws-vpc-network.yaml` and `aws-instance.yaml` scenarios validate against the schema.
- `examples/working/basic_instance` fully exercises VPC → subnet → SG → instance creation through the AWS provider.
- `examples/misconfigured/instance_missing_subnet` fails apply with the correct AWS error code (the provider must surface fakeaws's 404 as a Terraform error containing `InvalidSubnetID.NotFound`).
- `examples/updates/update_security_group_rules` flips ingress rules via `v2.tfvars` without recreating the SG.

---

## Slice 45: Stateful data (RDS + DynamoDB)

RDS shares the query-RPC protocol with EC2, so the parser from S44 carries through. DynamoDB is its own JSON dialect with `x-amz-target` routing.

### Tickets

| id | title | priority | deps |
|---|---|---|---|
| S45-T1 | Phase 3 design note: RDS query-RPC + DynamoDB JSON; collapsed state machines; private-network FK with EC2 | P1 | S43-T2 |
| S45-T2 | fakeaws: repository support for rds_instances + rds_clusters + rds_subnet_groups + rds_parameter_groups (FK on subnet group → EC2 subnets) | P1 | S43-T6, S44-T2 |
| S45-T3 | fakeaws: `handlers/rds.go` (DBInstance + DBCluster + DBSubnetGroup + DBParameterGroup + ClusterParameterGroup) | P1 | S45-T2 |
| S45-T4 | fakeaws: repository support for dynamodb_tables + dynamodb_items (item PK index) | P1 | S43-T6 |
| S45-T5 | fakeaws: `handlers/dynamodb.go` (Table CRUD + minimal item PutItem/GetItem/UpdateItem/DeleteItem/Query/Scan) | P1 | S45-T4 |
| S45-T6 | fakeaws: handlers_test for RDS + DynamoDB (CRUD, FK, cascade, basic item ops) | P1 | S45-T3, S45-T5 |
| S45-T7 | fakeaws: regression coverage for RDS read-replica chain + DynamoDB GSI/LSI + table-state transitions | P1 | S45-T5, S45-T6 |
| S45-T8 | infrafactory: `scenarios/training/aws-rds.yaml` + `aws-dynamodb.yaml` + loader update | P1 | S45-T7, S43-T7 |
| S45-T9 | fakeaws: `examples/working/rds_instance` + `working/dynamodb_table` + matching `misconfigured` + `updates` dirs | P1 | S45-T7 |
| S45-T10 | fakeaws Phase 3: gated `TestE2E_AWS_RDS` + `TestE2E_AWS_DynamoDB` in infrafactory | P1 | S45-T7, S45-T9 |

### Acceptance criteria

- All ten phase exit gates green.
- RDS state machine: instance lifecycle states (creating → available → modifying → deleting) collapsed to "always available" except where the AWS provider expects to wait — pin the exact subset in tests.
- DynamoDB Query/Scan returns paginated, attribute-projected responses matching the SDK's expectations on `Count`, `ScannedCount`, `LastEvaluatedKey`.
- RDS read-replica via `CreateDBInstance` with `SourceDBInstanceIdentifier` pinned by a regression test (mirror of fakegcp's parent-FK rebinding tests).

---

## Slice 46: Containers + queues (EKS + SQS)

EKS is JSON-REST (modern flavour). SQS is JSON 1.0 with `x-amz-target`.

### Tickets

| id | title | priority | deps |
|---|---|---|---|
| S46-T1 | Phase 4 design note: EKS JSON-REST and SQS x-amz-target wire formats; cluster/nodegroup state-machine simplification | P1 | S43-T2 |
| S46-T2 | fakeaws: repository support for eks_clusters + eks_node_groups + eks_addons with FK cascade + IAM/EC2 cross-resource validation | P1 | S43-T6, S43-T8, S44-T2 |
| S46-T3 | fakeaws: `handlers/eks.go` (Cluster + NodeGroup + AddOn; FK against IAM roles + EC2 subnets + security groups) | P1 | S46-T2 |
| S46-T4 | fakeaws: repository support for sqs_queues + sqs_messages with at-least-once visibility-timeout collapsed | P1 | S43-T6 |
| S46-T5 | fakeaws: `handlers/sqs.go` (Queue + minimal SendMessage / ReceiveMessage / DeleteMessage) | P1 | S46-T4 |
| S46-T6 | fakeaws: handlers_test for EKS + SQS (CRUD, FK, cascade, message lifecycle) | P1 | S46-T3, S46-T5 |
| S46-T7 | fakeaws: regression coverage for EKS cluster→nodegroup→addon dependencies + SQS DLQ + visibility-timeout edge cases | P1 | S46-T5, S46-T6 |
| S46-T8 | infrafactory: `scenarios/training/aws-eks.yaml` + `aws-sqs.yaml` + loader update | P1 | S46-T7, S43-T7 |
| S46-T9 | fakeaws: `examples/working/eks_cluster` + `working/sqs_queue` + matching `misconfigured` + `updates` dirs | P1 | S46-T7 |
| S46-T10 | fakeaws Phase 4: gated `TestE2E_AWS_EKS` + `TestE2E_AWS_SQS` in infrafactory | P1 | S46-T7, S46-T9 |

### Acceptance criteria

- All ten phase exit gates green.
- EKS cluster create FK-validates the IAM role ARN, the subnet ARNs, and the security group IDs in one transaction.
- SQS visibility-timeout collapsed to in-memory tracking (no real timeout enforcement); the test pins the response shape, not the actual timing.
- DLQ semantics: a queue whose `RedrivePolicy` references a non-existent DLQ fails create with the right AWS error code.

---

## Slice 47: DNS + secrets (Route53 + Secrets Manager)

Mirrors fakegcp's DNS and Secret Manager almost line-for-line — same atomic changes API for Route53, same DESTROYED-is-terminal contract for Secrets Manager.

### Tickets

| id | title | priority | deps |
|---|---|---|---|
| S47-T1 | Phase 5 design note: Route53 XML wire format + Secrets Manager JSON-1.1 in awsproto, validate against terraform-provider-aws expectations | P1 | S43-T2 |
| S47-T2 | fakeaws: repository support for route53_hosted_zones + record_sets, FK + non-empty-zone delete refusal | P1 | S43-T6 |
| S47-T3 | fakeaws: `handlers/route53.go` (HostedZone + ResourceRecordSet via `ChangeResourceRecordSets`, transactional) | P1 | S47-T2 |
| S47-T4 | fakeaws: repository support for secretsmanager_secrets + versions, with state column + FK from versions → secrets | P1 | S43-T6 |
| S47-T5 | fakeaws: `handlers/secretsmanager.go` (Secret + Version state machine; DESTROYED terminal; tagging) | P1 | S47-T4 |
| S47-T6 | fakeaws: handlers_test for Route53 + Secrets Manager (CRUD, FK, cascade, change-id scoping) | P1 | S47-T3, S47-T5 |
| S47-T7 | fakeaws: regression coverage for Route53 changes API atomicity + Secrets Manager :destroy / :enable terminal-state contract | P1 | S47-T5, S47-T6 |
| S47-T8 | infrafactory: `scenarios/training/aws-route53.yaml` + `aws-secrets-manager.yaml` + loader | P1 | S47-T7, S43-T7 |
| S47-T9 | fakeaws: `examples/working/route53` + `working/secrets_manager` + matching `misconfigured` + `updates` dirs (v1/v2 tfvars) | P1 | S47-T7 |
| S47-T10 | fakeaws Phase 5: gated `TestE2E_AWS_Route53` + `TestE2E_AWS_SecretsManager` in infrafactory/internal/e2e | P1 | S47-T7, S47-T9 |

### Acceptance criteria

- All ten phase exit gates green.
- Route53 `ChangeResourceRecordSets` is transactional: a batch with one bad change rejects the whole batch with no partial state. Mirror of fakegcp's pass-1 DNS atomicity fix.
- `GetChange` poll endpoint scoped by (account, hosted-zone, change-id) tuple — change ids from one zone don't leak across zones. Mirror of fakegcp's pass-17 `(project, zone, id)` keying.
- Secrets Manager `RestoreSecret` after `:destroy` returns 409 with `InvalidRequestException` (the AWS-spec terminal-state code).
- `examples/working/route53` exercises the changes API with both A and AAAA records; updates example flips TTL via `v1.tfvars` → `v2.tfvars`.

---

## Slice 48: Polish + codex review iteration loop

Not a feature slice. This exists to run the same 33-pass-style review loop that landed fakegcp at quality. Until two consecutive `NOTHING_TO_IMPROVE` returns, fakeaws v1.0 is not shippable.

### Tickets

| id | title | priority | deps |
|---|---|---|---|
| S48-T1 | fakeaws Phase 6 / polish: `regression_test.go` scaffolding + first round of pinned regressions across all phases | P1 | S43-T6, S44-T6, S45-T6, S46-T6, S47-T6 |
| S48-T2 | fakeaws Phase 6 / polish: `harness.countOrphans` extension to ignore aws operations / log tables analogous to fakegcp | P1 | S43-T6 |
| S48-T3 | fakeaws Phase 6 / polish: cross-resource FK validators (`resolveSameAccountName` helper, mirror of fakegcp's `resolveSameProjectName`) | P1 | S47-T10 |
| S48-T4 | fakeaws Phase 6 / polish: codex review iteration loop until 2 consecutive NOTHING_TO_IMPROVE passes (mirror of fakegcp 33-pass loop) | P1 | S47-T10 |
| S48-T5 | fakeaws Phase 6 / polish: README + AGENTS + PLAN + BACKLOG docs in fakeaws repo | P1 | S43-T1 |
| S48-T6 | fakeaws Phase 6 / polish: `working/` examples for every in-scope service (one each, must apply→destroy clean) | P1 | S43-T10..S47-T10 |
| S48-T7 | fakeaws Phase 6 / polish: `misconfigured` + `updates` examples for every service (one each per service in scope) | P1 | S48-T1 |
| S48-T8 | fakeaws Phase 6 / polish: pre-commit hook + gitleaks config + Makefile sweep matching fakegcp | P1 | S48-T1 |

### Acceptance criteria

- S48-T1: `regression_test.go` exists with the 14 standing patterns from `fakeaws/concepts.md` § "Standing patterns to seed regression_test.go on day one" pre-pinned.
- S48-T2: `internal/harness/destroy.go::countOrphans` extension recognises and ignores fakeaws's operation/log tables (analog of fakegcp's `operations` exclusion).
- S48-T3: `resolveSameAccountName` helper lives in `fakeaws/repository/`, with subtests covering all four cross-resource FK validation rules pre-tested in fakegcp passes 27–30 (cross-account, wrong-collection, relative-path, bare-name region scoping).
- S48-T4: 2 consecutive `NOTHING_TO_IMPROVE` codex passes documented in commit messages, with the prompts archived under `fakeaws/docs/review-passes/`.
- S48-T5: README, AGENTS, PLAN, BACKLOG all populated; AGENTS.md is the "fresh agent" entry point, PLAN.md tracks which phases are landed, BACKLOG.md is the open-work tracker.
- S48-T6: every service in scope has at least one `examples/working/<svc>` that passes the apply → plan → destroy gate.
- S48-T7: every service has at least one `misconfigured` and one `updates` directory; the misconfigured one fails through tofu with the AWS-shaped error code.
- S48-T8: `.git/hooks/pre-commit` runs `make test` + `gitleaks detect`; `.gitleaks.toml` allowlists `examples/.*\.tf` placeholder credentials.

---

## Cross-pollination back to mockway and fakegcp

Findings from S48 that reveal a class of bug shared by the older mocks must land back in mockway/fakegcp before the relevant fakeaws phase exits. Concrete instances likely:

- New cross-resource FK validators discovered during the EC2 phase translate to mockway's VPC↔private-network FK chain.
- AWS state-machine refinements (RDS modifying-state) may surface gaps in mockway's RDB read-replica handling.
- Wire-format error-shape consistency tests apply to all three mocks.

Track cross-pollination tickets as `M<n>` (Maintenance) entries in `BACKLOG.md` rather than re-opening fakegcp/mockway slices.

---

## Open questions

Tracked in `fakeaws/concepts.md` § "Open questions". Resolutions will land in this plan as they're answered.

1. Account namespacing: synthetic `000000000000` for v1; multi-account is v2.
2. Auth: accept Bearer + SigV4 without validation; v2 may add syntactic validation.
3. ARN format: helper in `fakeaws/handlers/awsproto/arn.go` — `arn:aws:<service>:<region>:<account>:<resource-type>/<id>`.
4. terraform-provider-aws version pin: TBD; pin during S43-T1, document in `fakeaws/AGENTS.md`.
5. S3 object payload: write-only at v1, body discarded; revisit if a scenario needs object content.
6. `concepts.md` long-term home: fold into `fakeaws/PLAN.md` once the repo has shape; until then it's the load-bearing planning doc.
