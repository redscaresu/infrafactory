# Next Session

Self-contained brief for a fresh Claude / engineer starting in this repo
after the 2026-05-30 → 2026-05-31 self-learning-sweep session.

## Session context (TL;DR)

We ran a self-learning sweep across all 39 infrafactory training
scenarios, treating every failure as either:

1. a **mock-server gap** → fix at source in fakeaws / fakegcp / mockway,
   never seed `pitfalls/*.yaml`, or
2. an **LLM-generated HCL mistake** → let the auto-learning pipeline
   capture a pitfall; if it produced a descriptive-only rule, add an
   M97-style prescriptive template.

End state:

- **Single-shot 39/39.** Each scenario passed in a clean re-run with
  mocks reset between scenarios.
- **Full validation sweep 31/39.** Some single-shot passes don't
  survive re-runs; the LLM is non-deterministic and some fixes
  uncovered deeper gaps. See `Open follow-ups` below.

## Open follow-ups (next session work)

Tickets are roughly ordered by impact. Each is 1-4 hours.

### 1. `aws-full-stack` — IAM user policy persistence (fakeaws)

**Symptom:** `aws_iam_user_policy_attachment` create succeeds, then
read fails with `empty result` because `ListAttachedUserPolicies`
returns an empty list.

**Why it broke:** I added `AttachUserPolicy` / `PutUserPolicy` /
`GetUserPolicy` as no-ops in `fakeaws/handlers/iam.go`. CREATE
passes, but the provider's READ path enumerates attachments to
confirm — and gets nothing back.

**Fix:** Add proper persistence in `fakeaws/repository/iam.go`:

- `user_policy_attachments` table (account_id, user_name, policy_arn).
  Same shape as `role_policy_attachments`.
- `user_inline_policies` table (account_id, user_name, policy_name,
  document JSON).
- Wire the existing handlers to insert/list/delete from those tables.

Tests to add in `fakeaws/handlers/iam_test.go`:
- Create+List+Delete round-trip for managed-policy attachments.
- Create+Get+Delete round-trip for inline policies.

### 2. GCP `sql_custom_endpoint` path duplication

**Symptom:** `gcp-cloud-sql` and `gcp-full-stack` 501 on
`POST /sql/v1beta4/sql/v1beta4/projects/...` (double `sql/v1beta4`).

**Why:** Our template emits `sql_custom_endpoint = "%s/sql/v1beta4/"`
and the v5 provider also prepends `sql/v1beta4/...` — same shape
bug as the `iam_custom_endpoint` and `service_usage_custom_endpoint`
ones I fixed by dropping the trailing path.

**Fix:** In `internal/cli/generate_command.go::buildGoogleProviderBlock`,
change `sql_custom_endpoint` to drop the trailing path. Verify by
running `gcp-cloud-sql` against fakegcp.

**Suspected sibling endpoints with same pattern:** `dns_custom_endpoint
= "%s/dns/v1/"`, `cloud_run_v2_custom_endpoint = "%s/v2/"`. Test each
against the v5 provider source-tree to confirm before changing.

### 3. fakegcp `Service Networking` endpoint missing

**Symptom:** `gcp-cloud-sql` private-IP path 401s against
`servicenetworking.googleapis.com`.

**Fix:** Add `service_networking_custom_endpoint` to
`generate_command.go` template; add routes in fakegcp:

- `GET  /v1/services/{service}/connections` (returns empty list)
- `POST /v1/services/{service}/connections` (200 + synthesised resource)
- `PATCH /v1/services/{service}/connections` (200)
- `DELETE /v1/services/{service}/connections/{name}` (200)

### 4. fakegcp DNS / SQL / NodePool plugin crashes

**Symptom:** `gcp-dns`, `gcp-gke-cluster`, `gcp-full-stack` see
"Plugin did not respond" on `google_dns_record_set`,
`google_container_node_pool`, `google_sql_database_instance`.

**Why:** fakegcp returns JSON that mismatches what the v5 provider's
parser expects. Provider crashes with nil-deref on missing fields.

**Fix path:** For each resource:
1. Run a single create against fakegcp with verbose terraform logging
   (`TF_LOG=DEBUG`) to see the exact request + response.
2. Compare response shape to the real GCP API spec for that resource.
3. Identify the missing/wrong field. Often it's an embedded sub-block
   the provider tries to dereference (e.g. `nodePool.config`,
   `dnsRecordSet.rrdatas`).

These need actual reading of provider source, not pattern-matching.

### 5. fakegcp Secret Manager `SecretVersion` 404

**Symptom:** `gcp-secret-manager` 404s on
`GET /v1/projects/.../secrets/.../versions/.../data` (or similar
read path) even right after create.

**Fix:** Inspect `fakegcp/handlers/iam.go` (Secret handlers — the
file is misnamed, secrets live there) for the SecretVersion lookup.
Likely a key-mismatch (version IDs not persisted as the provider
expects) or path-param parsing issue.

### 6. `gcp-cloud-run` — LLM hallucinates `deletion_protection`

**Symptom:** LLM writes `deletion_protection = false` on
`google_cloud_run_v2_service` which the v5 provider doesn't accept.

**Why this is interesting:** It's an LLM-generated HCL mistake the
auto-learning pipeline should catch, not a mock gap. The descriptive
fallback fires but the rule isn't actionable enough; the system
keeps making the same mistake.

**Fix:** Add a M97-style prescriptive template in
`internal/generator/pitfalls_learn.go` for "Unsupported argument
NAME on RESOURCE_TYPE" that gives the LLM canonical removal
guidance. Already pattern-similar to `matchUnsupportedAttribute`.

### 7. Mock-state reset built into `infrafactory run`

**Status:** My sweep script resets mockway/fakegcp/fakeaws between
scenarios. Belongs in `internal/cli/run_command.go` so any sequential
caller (sweep, CI batch, retry) gets it automatically.

**Fix:**
- Add `--reset-mocks` flag (default `true` when `run_mode=clean`).
- Before iter 1 starts, POST to each configured mock's reset endpoint.

### 8. fakeaws `aws_subnet.MapPublicIpOnLaunch` doesn't persist

**Symptom:** Provider's wait-loop polls `DescribeSubnets` after
`ModifySubnetAttribute` and waits 5 min for `MapPublicIpOnLaunch=true`,
times out. (My `ModifySubnetAttribute` is no-op; the attribute
isn't stored.)

**Fix:** Store subnet-level scalar flags in
`fakeaws/repository/ec2.go::EC2Subnet` struct, persist on
ModifySubnetAttribute, surface on DescribeSubnets.

## Workflow / harness improvements

Things I want to land before they're forgotten in a fresh session:

### A. `make up` / `make down` shipped this session

Brings up mockway + fakegcp + fakeaws + SeaweedFS + UI in one
command. See `Makefile` head — added 2026-05-31.

### B. Add `infrafactory sweep` subcommand

The 5+ bash sweep scripts I wrote (`/tmp/sweep-*.sh`) replicate the
same pattern: iterate scenarios, reset mocks between each, log
per-scenario output, tally pass/fail at the end. This is the right
ergonomic to land in-tree.

**Spec:**
```
infrafactory sweep [--scenarios FILE.txt | --all] [--continue-on-fail] [--reset-mocks=true]
```

- Default `--all` runs every `scenarios/training/*.yaml`.
- `--scenarios FILE.txt` reads scenario paths from a file (one per line).
- `--continue-on-fail` keeps going past failures (default stop on first).
- Emits a final summary table to stdout.

Suggested location: `internal/cli/sweep_command.go`.

### C. Sub-session checkpoints

Long sessions like this one (40+ commits of fixes across 4 repos)
benefit from intermediate "save progress" markers. Convention I'd
suggest: after every ~5 mock-source fixes, run `git stash` or a
WIP commit. Avoids losing a chain of fixes to one bad change.

### D. `pitfalls/*.yaml` doesn't survive the `--reset` flow

Currently the auto-learning loop appends to pitfalls files in the
repo working tree. A `make reset-pitfalls` target would clear all
three to `pitfalls: []` for clean sweeps (useful when validating the
learning loop itself rather than relying on accumulated knowledge).

### E. Visual regression test runs page

Made the runs page screenshot viewport-only with the table masked
(2026-05-30). Should review whether other dynamic pages (compare,
diagnostics) have the same growing-content fragility.

## Commit landmarks (this session)

Across 4 repos:

- **fakeaws** `348322d` — broad EC2/IAM/KMS/Route53/DynamoDB coverage
  (~17 fixes).
- **fakegcp** `c7999b5` — Cloud KMS stubs for CMEK-required scenarios.
- **infrafactory** `7728658` — M98 after-apply-reference policy work
  (pre-staged); `bf3727e` — feedback pipeline + generator templates +
  policy exemptions + provider wiring.

## Memory pointers

- `feedback_sweep_protocol.md` — "fix at source, never seed
  pitfalls; pruning stale entries after a mock-source fix is OK."
  This is load-bearing — re-read it before making decisions about
  `pitfalls/*.yaml` editing.
- `feedback_mock_design.md` — "mocks optimise for fast feedback, not
  realism." Drives decisions like AMI auto-seed, terminated-instance
  GC, KMS hard-delete on schedule.

## How to verify the current state

```bash
make up                                        # all mocks + UI
make test                                      # Go + UI + Playwright
/tmp/infrafactory-new run scenarios/training/block-paris.yaml --config infrafactory.yaml
# Expect: terminal_reason: target_reached, completed_iterations=1
```

If you need to reproduce the full sweep:

```bash
# Use the script from this session
bash /tmp/sweep-full.sh   # if still on disk, else regenerate from this file's sweep order
```
