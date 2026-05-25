# Spike: ministack as fakeaws replacement / supplement

**Date**: 2026-05-25
**Status**: Eval complete, decision pending

## Question

Should we switch the AWS backend from `fakeaws` (our 1st-party Go mock) to
[`ministack`](https://github.com/ministackorg/ministack) (open-source
LocalStack alternative that just shipped)?

## Methodology

1. Pulled `ministackorg/ministack:latest` (light edition, ~270MB image, ~50MB
   RAM idle).
2. Ran the same 14-resource composition that `TestE2E_AWSFullStack`
   exercises against `fakeaws`: VPC + 2 subnets + 2 IAM roles + EKS
   cluster + node group + S3 bucket + SSE config + RDS subnet group +
   parameter group + Postgres DB instance + Secrets Manager secret +
   version.
3. Measured `apply` → `plan -detailed-exitcode 0` → `destroy` lifecycle.

## Findings

### Wire compatibility — excellent

All 14 resources applied + destroyed cleanly on first try, with **zero
wire-shape patches**. Specifically:

| Quirk fakeaws needed | ministack |
|---|---|
| M61: SHA-1 `DbiResourceId` hash to avoid identifier-derived lookup collisions | not needed — handled natively |
| M61: `DeleteDBInstance` envelope (`<DeleteDBInstanceResult><DBInstance>`) | not needed |
| M61: `Filters.Filter.N.Name=dbi-resource-id` lookup parser | not needed |
| M61: Service-specific 404 codes (`DBInstanceNotFound`, `DBSubnetGroupNotFoundFault`, etc.) | not needed |
| M61: User-supplied field persistence (MasterUsername, AllocatedStorage, etc.) | not needed |
| M62: ARN-or-name `SecretId` dual lookup | not needed |
| M62: `CreatedDate`/`LastChangedDate`/`DeletionDate` as JSON-number epoch | not needed |
| M62: `VersionIdsToStages` population | not needed |
| M62: `GetResourcePolicy` / `ListSecretVersionIds` / no-op `TagResource` | not needed |
| M59: SeaweedFS for S3 bucket sub-resource reads | not needed (ministack S3 handles bucket policy/tagging/SSE natively) |
| M51: Query-RPC envelope rewrite | not needed |
| M57: Per-resource Read-flow field parity (EC2/EKS/RDS) | not needed |

In short: **every patch we landed across M51/M57/M59/M61/M62 (~600 LoC across
6 commits, 4 weeks of intermittent work) is unnecessary against ministack.**

### Performance — comparable

| Phase | fakeaws (M61/M62 era) | ministack |
|---|---|---|
| Apply (14 resources) | ~88s (TestE2E_AWSFullStack: 232s for full pipeline with mock + assertions) | **88s** |
| Plan post-apply | No changes | **No changes** |
| Destroy | All clean | **66s, all clean** |
| Image size | n/a (single Go binary, ~36MB) | ~270MB (light edition) |
| Memory idle | n/a (no daemon) | **50MB RAM, 0% CPU** |
| Startup | sub-second | **~2s** |

The RDS 1m21s apply wait is identical across both — that's
terraform-provider-aws's built-in `Delay: 30s + 5 × MinTimeout: 10s = 80s`
state-change wait, not a mock-side latency.

### Service coverage — 7× expansion

| Backend | Service count | Examples |
|---|---|---|
| fakeaws | 9 | IAM, S3, EC2, RDS, DynamoDB, EKS, SQS, Route53, Secrets Manager |
| ministack | **62** | All 9 above PLUS Lambda, KMS, ACM, API Gateway, Step Functions (states), Cognito, CloudFormation, ECS, ECR, ElastiCache (Redis), Athena, Glue, SNS, SSM, Kinesis, Firehose, WAF, CloudWatch (logs + monitoring), CloudTrail, OpenSearch, EFS, EMR, Route53, CodeBuild, IoT, ELB/ALB, Backup, Batch, Organizations, Transfer, Scheduler, SES, AppSync, AppConfig, Resource Groups, Service Discovery — and 20+ more |

This is the headline. Every scenario we'd otherwise have to block on
fakeaws gaining a handler (Lambda, KMS, etc.) just works against ministack
today.

### Admin-surface incompatibility — real integration cost

ministack uses `/_ministack/*` namespace for admin endpoints, not our
`/mock/*` convention:

| Endpoint | fakeaws/fakegcp/mockway | ministack |
|---|---|---|
| Health | (none — port responds) | `GET /_ministack/health` (JSON: services map + edition + version) |
| Reset | `POST /mock/reset` | `POST /_ministack/reset` (different body shape: `{"reset": "ok"}`) |
| State dump | `GET /mock/state` (full /v1 schema) | **not supported** — `/mock` is interpreted as an S3 path lookup → 404 NoSuchBucket |
| Snapshot/Restore | `POST /mock/snapshot` / `/mock/restore` | **not supported** |

This is the load-bearing gap. infrafactory's Layer 2 ("mock deploy")
validation relies on `/mock/state` to:
1. Derive topology graphs for connectivity / http_probe acceptance criteria
2. Verify no-orphans on destruction
3. Run OPA `deny_state` policies against post-apply state

To swap fakeaws for ministack we'd need to either:
- (A) Write a polyfill that synthesises `/mock/state` from ministack's
  introspection APIs (walk `ListBuckets` + `DescribeInstances` +
  `ListSecrets` + ... per service in scope). Estimated ~400 LoC; tracks
  changes as ministack adds services.
- (B) Redesign Layer 2 to use direct AWS-SDK probes instead of `/mock/state`
  — a much larger architectural shift across all three clouds (mockway +
  fakegcp + fakeaws all share the contract).
- (C) Skip topology checks on ministack-backed scenarios — Layer 2
  degrades to OPA-on-plan + apply-succeeded; no orphan check, no
  connectivity verification.

### License + maintenance — solid

- MIT-licensed (vs our Apache-2.0 family — fine, compatible)
- ~3000 stars, just pushed today (2026-05-25)
- Explicit positioning as the LocalStack-community replacement (LocalStack
  killed their community tier earlier this year — documented in our
  `project_oss_readiness.md` memory)
- Active project with momentum, but very new — fewer production
  battle-test hours than fakeaws's targeted set

### Other observations

- ministack requires a Docker daemon (fakeaws is a single Go binary). Not a
  new tax in our stack — SeaweedFS already requires Docker for S3.
- Python implementation vs our Go — different perf characteristics under
  load; not measured here.

## Trade-offs summary

| Axis | fakeaws | ministack | Winner |
|---|---|---|---|
| Service coverage | 9 | 62 | **ministack** (7×) |
| Wire-shape parity | requires patches | clean out of the box | **ministack** |
| Maintenance burden | ours | external (MIT org) | **ministack** |
| `/mock/state` admin contract | native | needs polyfill or redesign | **fakeaws** |
| Cross-cloud symmetry (mockway/fakegcp/fakeaws as 1st-party set) | preserved | breaks the pattern | **fakeaws** |
| Battle-test hours for our use case | months + 17 codex passes | days | **fakeaws** |
| Design principle: "fast feedback, not realism" ([feedback_mock_design.md](../../../.claude/projects/-Users-ehsanashouri-go-src-github-com-redscaresu-infrafactory/memory/feedback_mock_design.md)) | aligned | introduces real Postgres containers for RDS (light edition uses synthetic; full edition uses real) | **fakeaws** (slight) |
| Lines of code we own | ~4000 (handlers) + 600 (M51/M57/M61/M62 patches) | 0 (vendor) | **ministack** |

## Recommendation: parallel backend (option A from the earlier list)

Don't replace fakeaws; **add ministack as an opt-in parallel backend** for
services fakeaws doesn't cover, mirroring how SeaweedFS supplements
fakeaws's S3.

Concretely:
1. Add `ministack: { url: "http://127.0.0.1:4566", auto_reset: true }`
   block to `infrafactory.yaml` (sibling to `s3` and `fakegcp`/`fakeaws`
   blocks).
2. Extend `cloudMockStateRouter.pick(service string)` to dispatch
   `aws + <unimplemented-service>` to ministack instead of erroring.
3. Write a thin `/mock/state` polyfill that synthesises the AWS section
   from ministack's introspection APIs **only for services it owns** —
   keeps fakeaws's `/mock/state` for services it already covers. ~200 LoC
   versus the full 400 of a complete polyfill.
4. Add a training scenario that exercises a ministack-only service
   (Lambda + API Gateway is a good first one) to prove the integration
   end-to-end.

This:
- Preserves all fakeaws work (no rewrite)
- Unblocks Lambda / KMS / etc. scenarios that were previously waiting on
  fakeaws to grow handlers
- Keeps the cross-cloud symmetry story intact
- Doesn't depend on ministack being perfectly stable — fakeaws is the
  fallback for everything it covers
- Defers the "switch entirely?" decision until we have data on ministack's
  long-term reliability against terraform-provider-aws

If ministack proves rock-solid over 6+ months and we find ourselves
maintaining fakeaws less and less, we can revisit and consider full
deprecation. **The reversible move is the parallel-backend integration; the
irreversible move is rewriting around `/mock/state` removal.** Pick the
reversible move.

## Cost estimate

- Parallel-backend integration: ~1 week (config block + router dispatch +
  /mock/state polyfill for ministack-owned services + a Lambda training
  scenario + cross-repo e2e test).
- Documentation: README cloud-coverage table update + CONCEPT.md
  third-party-mock section gets a ministack subsection.
- Tracked as a new M-ticket: M63 (parallel ministack backend).
