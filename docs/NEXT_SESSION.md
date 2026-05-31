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

## Core design principle: mock quirks are tickets, not pitfalls

**Pitfalls encode real provider/cloud constraints** the LLM needs to
know about (e.g. "scaleway_instance_server exposes `private_ips`
plural, not `private_ip`"; "Route53 zone names must be DNS-valid").

**Mock-server gaps are bugs in fakeaws / fakegcp / mockway**. They
must be fixed in the mock, not papered over with a pitfall that tells
the LLM "avoid this resource". A pitfall that says "google_kms_key_ring
escapes to the real API" is anti-pattern: it teaches the LLM to NOT
use a perfectly valid resource because OUR mock is incomplete. That
makes the system narrower over time instead of broader.

Tickets 12, 13, 14 enforce this principle (classifier, self-healing,
backfill). Until they land, the rule is manual: if you see a learned
pitfall that describes a mock-server failure (`501`, `Plugin did not
respond`, OAuth-escape, missing-handler 404), file a ticket against
the appropriate mock repo and prune the pitfall — don't leave it.

## Sweep coverage map

Final state after the 2026-05-31 evening close-out session: **33/39
pass, 6 fail** (up from 31/39).

### Closed this session

- **T2** — `sql_custom_endpoint` host-only (infrafactory `8566033`).
- **T3** — `service_networking_custom_endpoint` added (infrafactory `8566033`).
- **T6** — `matchUnsupportedArgument` template + verbatim→prescriptive
  upgrade path (infrafactory `8566033`).
- **T7** — `--reset-mocks` flag (infrafactory `8566033`).
- **T9** — `CONTRIBUTING.md` make up (infrafactory `8566033`).
- **T11 (partial)** — `dns_custom_endpoint` host-only (infrafactory `8566033`).
- **T1** — fakeaws IAM user policy persistence (fakeaws `c92e323`).
  Validated via direct round-trip: AttachUserPolicy →
  ListAttachedUserPolicies returns the attached ARN (previously
  returned empty list).

### Still open

| Failing scenario | Closes when these tickets land | Confidence |
|---|---|---|
| `aws-full-stack` | **Ticket A** (fakeaws ListSSHPublicKeys handler) + **Ticket B** (fakeaws managed-policy orphan-counting) | High — both surfaced during T1 validation |
| `gcp-cloud-sql` | Ticket 3 fakegcp routes (endpoint flag landed, routes pending) | High |
| `gcp-full-stack` | Tickets 3 + 4 (plugin crash on `google_sql_database_instance`) | Medium |
| `gcp-gke-cluster` | Ticket 4 (NodePool plugin crash) | High |
| `gcp-secret-manager` | Ticket 5 (SecretVersion 404) | Medium |
| `gcp-storage` | Intermittent — LLM non-determinism | n/a |
| `web-app-paris` | Regression introduced this session — see notes | Unknown |

### Tickets A + B (NEW — uncovered during T1 validation 2026-05-31 19:00)

**Ticket A — fakeaws ListSSHPublicKeys / ListServiceSpecificCredentials**.
`aws_iam_user` destroy preflight enumerates SSH keys + service-
specific credentials. fakeaws returns 404 → destroy fails. Fix:
add no-op handlers returning empty lists in `fakeaws/handlers/iam.go`
similar to `iamListGroupsForUser`. ~15 min.

**Ticket B — fakeaws auto-seeded managed policies count as orphans**.
`SeedManagedPolicy` (called by AttachRolePolicy/AttachUserPolicy)
inserts into `iam_policies`. `/mock/state` exposes them and
`countOrphans` in infrafactory `internal/harness/destroy.go` then
counts them. Two reasonable fixes: (a) seed managed policies into a
separate `iam_managed_policy_seeds` table excluded from `/mock/state`,
or (b) filter `arn:aws:iam::aws:policy/*` from the iam.policies
output. (b) is simpler. ~30 min.

**Recommended order** (lowest-cost-per-closed-scenario first):

| # | Ticket | Effort | Closes |
|---|---|---|---|
| 1 | **Ticket A** — fakeaws ListSSHPublicKeys etc | ~15 min | `aws-full-stack` destroy |
| 2 | **Ticket B** — managed-policy orphan filter | ~30 min | `aws-full-stack` orphan-count |
| 3 | Ticket 5 — Secret Manager version 404 | ~1 hr | `gcp-secret-manager` |
| 4 | Ticket 3 (rest) — fakegcp Service Networking routes | ~1-2 hr | `gcp-cloud-sql` rest |
| 5 | Ticket 4 — plugin-crash family (NodePool + SQL) | ~3-4 hr per resource | `gcp-gke-cluster`, `gcp-full-stack` |
| 6 | Ticket 11 (rest) — audit other `*_custom_endpoint`s | ~1 hr | preventive |
| 7 | Ticket 10 — mirror "one-shot demo" in mock READMEs | ~15 min | (docs) |
| 8 | Ticket 8 — fakeaws subnet MapPublicIpOnLaunch persistence | ~1-2 hr | latent |
| 9 | `web-app-paris` regression investigation | unknown | regression |

**Realistic total to 39/39 deterministic**: ~6-8 hour session,
mostly mock-source work. Ticket 4 is still the deep one (provider
source-reading per resource); everything else is pattern-match.

## Open follow-ups (next session work)

Tickets are detailed below in the same order they appeared during
the sweep — but the map above is the order to *work* them.

### 1. `aws-full-stack` — IAM user policy persistence (fakeaws) — CLOSED

**Closed by:** fakeaws `c92e323` (2026-05-31 evening session).
`user_policy_attachments` + `user_inline_policies` tables added in
`repository/iam.go`; handlers in `handlers/iam.go` switched from
no-op stubs to persistence-backed implementations; round-trip tests
landed in `handlers/iam_test.go`
(`TestIAM_AttachDetachUserPolicy`, `TestIAM_PutGetDeleteUserPolicy`).

Verified live: `curl AttachUserPolicy` + `curl ListAttachedUserPolicies`
returns the attached ARN.

aws-full-stack does NOT yet close end-to-end though — destroy now
fails on `ListSSHPublicKeys` 404 and orphan-check fires on
auto-seeded managed policies. See Tickets A + B above.

### 2. GCP `sql_custom_endpoint` path duplication — CLOSED

**Closed by:** infrafactory `8566033`. `sql_custom_endpoint` flipped
to host-only in `internal/cli/generate_command.go::buildGoogleProviderBlock`
(inline comment explains v5 provider's strip-regex behaviour on
http:// endpoints). `dns_custom_endpoint` also flipped to host-only
in the same commit (closes `gcp-dns`).

### 3. fakegcp `Service Networking` endpoint missing — PARTIAL

**Partially closed by:** infrafactory `8566033` —
`service_networking_custom_endpoint` added to the provider template
so requests stop escaping to the real cloud. **Pending:** fakegcp
routes still need to land so the requests have somewhere to land:

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

### 6. `gcp-cloud-run` — LLM hallucinates `deletion_protection` — CLOSED

**Closed by:** infrafactory `8566033`. `matchUnsupportedArgument`
template in `internal/generator/pitfalls_learn.go` handles wrapped
`Unsupported argument` diagnostics. The verbatim→prescriptive
upgrade path in `AppendPitfall` lets later prescriptive rules
REPLACE earlier raw-stderr entries for the same resource (otherwise
the descriptive dump would permanently shadow the prescriptive form
via dedup). First non-cloud-run firing: `google_redis_instance` got
a clean prescriptive rule for `deletion_protection` in the same
sweep.

### 7. Mock-state reset built into `infrafactory run` — CLOSED

**Closed by:** infrafactory `8566033`. `--reset-mocks` flag (default
true on clean runs) fans `POST /mock/reset` to every configured
mock URL (mockway/fakegcp/fakeaws). Replaces the per-script curl
fan-out the sweep scripts used to do externally.

### 8. fakeaws `aws_subnet.MapPublicIpOnLaunch` doesn't persist

**Symptom:** Provider's wait-loop polls `DescribeSubnets` after
`ModifySubnetAttribute` and waits 5 min for `MapPublicIpOnLaunch=true`,
times out. (My `ModifySubnetAttribute` is no-op; the attribute
isn't stored.)

**Fix:** Store subnet-level scalar flags in
`fakeaws/repository/ec2.go::EC2Subnet` struct, persist on
ModifySubnetAttribute, surface on DescribeSubnets.

### 9. Update `CONTRIBUTING.md` to reference `make up` — CLOSED

**Closed by:** infrafactory `8566033`.

### 10. Mirror "make up" demo in fakegcp + mockway READMEs

**Status:** `fakeaws/README.md` got a "One-shot demo (with sibling
repos)" subsection pointing at infrafactory's `make up`. fakegcp and
mockway READMEs should get the same blurb so a user landing on any
mock repo's GitHub page sees the consistent entry point.

### 11. `cloud_resource_manager_custom_endpoint` and others — PARTIAL

**Closed for `dns_custom_endpoint`:** infrafactory `8566033`. The
remaining endpoints still need a per-endpoint audit:
`cloud_resource_manager_custom_endpoint` (currently `/v1/`),
`cloud_run_v2_custom_endpoint` (currently `/v2/`),
`pubsub_custom_endpoint` (currently `/v1/`),
`storage_custom_endpoint` (currently `/storage/v1/`),
`secret_manager_custom_endpoint` (currently `/v1/`),
`redis_custom_endpoint` (currently `/v1/`).

For each: trace through the matching scenario and verify the wire
URL fakegcp receives matches the registered route. Pattern is
"if the v5 provider's lib client uses RemoveBasePathVersion or a
ReplaceAll-based path strip that doesn't fire on http:// endpoints,
the trailing path doubles."

### 12. Failure classifier — keep mock quirks OUT of pitfalls

**Status:** The auto-learning pipeline currently writes ANY recurring
failure to `pitfalls/*.yaml`, including ones that describe mock-server
gaps (`501 Not Implemented`, `Plugin did not respond`, OAuth-escape,
404 from a Describe* on a resource we just created). This violates
the "mock quirks are tickets, not pitfalls" principle — it teaches
the LLM to AVOID valid resources because the mock is incomplete.

**Fix:** Extend `internal/generator/pitfalls_learn.go` with a
`isMockActionable(detail)` predicate. Detection signals:

- `501 Not Implemented` anywhere in the body
- `Plugin did not respond`
- `OAuth ... access token` / `Request had invalid authentication credentials`
- `couldn't find resource (N retries)` (wait-loop after Describe* miss)
- `404 ... ResourceNotFoundException` on a Describe* path

When the predicate fires, instead of writing to `pitfalls/<cloud>.yaml`,
append a structured entry to `docs/mock-gaps.md` (or open a GitHub
issue against the right mock repo). Also extend the M91 no-seeding
ratchet (`TestPitfallsNoHumanSeeding`) to assert no learned entry
matches the mock-actionable predicate — turning the principle into
CI enforcement.

Effort: ~half-day. Strict superset of the existing pipeline; the
detection rules are simple substring/regex matches.

### 13. Self-healing mocks (second agent loop) — multi-session direction

**Status:** Ticket 12 is the lightweight "detect + document" version.
The heavy version is a second agent loop with read+write access to
the mock repos. When a mock-actionable failure is detected, it:

1. Reads the failing request from infrafactory's run artifacts.
2. Locates the matching handler in fakeaws / fakegcp / mockway — or
   detects that no handler exists.
3. Proposes a patch (adds a route + handler, or fixes a response
   shape) using the provider source as ground truth.
4. Runs the mock's existing tests to make sure nothing else breaks.
5. Restarts the mock + retries the failing scenario.
6. If green, opens a PR on the mock repo; if red, leaves a tracked
   ticket.

This closes the principle's loop end-to-end: mock quirks don't just
get TICKETS, they get fixed. Risk: less bounded than the current
LLM-driven pipeline because mock changes affect every scenario — needs
guardrails (per-PR scope, sibling-scenario regression run, human
approval for the patch).

Effort: multi-session. Worth scoping in its own slice before
starting. ADR-level decision (cross-repo write authority).

### 14. Audit + prune existing mock-quirk entries from pitfalls/*.yaml

**Status:** Per the principle above, almost every current entry in
`pitfalls/aws.yaml` (5/5) and many in `pitfalls/gcp.yaml` (~6/10) are
mock-quirk fingerprints written before the classifier existed. They
should be removed and their underlying gaps tracked as tickets against
the matching mock repo.

**Entries currently violating the principle** (as of 2026-05-31):

`pitfalls/aws.yaml` (all 5 entries):
- `aws_subnet` MapPublicIpOnLaunch wait-loop timeout → ticket 8.
- `aws_iam_role_policy_attachment` 404 → already fixed in fakeaws
  `348322d` (managed-ARN auto-seed), pitfall is stale.
- `aws_iam_role_policy` 404 → already fixed in fakeaws `348322d`
  (PutRolePolicy handler), pitfall is stale.
- `aws_vpc` "Failed to instantiate provider" → transient network
  (registry-download timeout), not learnable.
- `aws_db_instance` wait-loop "couldn't find resource" → fakeaws
  RDS create→read shape mismatch, new ticket.

`pitfalls/gcp.yaml`:
- KEEP: `google_compute_instance` + `google_container_cluster` VPC
  declaration rules (real GCP-side requirement).
- KEEP: `google_storage_bucket` no CMEK (real policy requirement —
  this is a Layer-1 gate).
- KEEP: `google_cloud_run_v2_service` `deletion_protection`
  (legitimate "the v5 provider doesn't accept this arg").
- PRUNE: `google_service_account` "escaping" → fixed by v5 pin +
  `iam_admin_v1` work, stale.
- PRUNE: `google_project_service` "escaping" → fixed by
  service_usage endpoint fix, stale.
- PRUNE: `google_service_networking_connection` "escaping" → fakegcp
  gap, ticket 3.
- PRUNE: `google_kms_key_ring` "escaping" → fixed by KMS handler
  (fakegcp `c7999b5`), stale.
- PRUNE: `google_container_node_pool` "Plugin did not respond" →
  ticket 4.
- PRUNE: `google_dns_record_set` "Plugin did not respond" → ticket 4.

**Fix:** Manual prune of stale entries (those that reference
already-fixed gaps), file mock-repo tickets for the rest, document
the principle in the auto-learning loop comments. Effort: ~1 hr.

Pair with ticket 12 so the principle is enforced going forward, not
just retroactively.

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

## Session-close hygiene (lesson from 2026-05-31 close)

Long sweep sessions accumulate persistent harness Monitors that
survive across sub-sessions and only die at session exit. They don't
hurt anything but they cause the harness to prompt "Background work
is running" when you try to exit. Always call **TaskStop** on every
Monitor you launched before closing the session.

Workflow improvements to land in the next session:

- **Convention**: every `Monitor()` call gets a paired `TaskStop()`
  when its purpose is done — don't leave them armed "just in case."
  Especially after a sweep finishes (or stops on the first failure
  for the bail-out pattern) — stop the monitor that was tailing
  its stdout immediately.

- **Optional tooling**: an `infrafactory clean` (or just `make clean`)
  target that finds and kills any lingering `bash /tmp/sweep-*.sh`
  + `tail -F /tmp/sweep-*-stdout.log` processes the previous session
  left running. Cheap, removes friction.

Specific background tasks that survived this session's close
(stopped manually before exit):
- `bzlwxl814` — Sweep-progress monitor (initial sweep run)
- `bek3umbwi` — Sweep 25 mock-state-reset monitor
- `b2s4vo2i5` — Revalidation-sweep monitor
