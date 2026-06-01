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

Final state after the 2026-05-31 evening close-out session:
**33/39 pass, 6 fail** in the sweep proper, but follow-up clean
runs closed ALL SIX outstanding scenarios: `aws-full-stack` (A+B),
`web-app-paris` (seeded scaleway pitfall), `gcp-cloud-sql` (D-2+E),
`gcp-full-stack` (D-2 alone, iter 1), `gcp-gke-cluster` (E-2 KMS
dual-prefix, iter 3), and `gcp-storage` (also benefits from E + E-2
— the "intermittent LLM non-determinism" label was lazy; the real
failure was the same KMS double-path bug that E fixed). Realistic
pass rate: **39/39**.

Lessons from this session worth carrying forward:
  - Stale "intermittent" labels from prior sweeps deserve a 30-second
    verification before being inherited. The gcp-storage failure was
    a concrete oscillation, not noise.
  - The NodePool "Plugin did not respond" wasn't a real plugin crash
    — it was downstream of an iter 1 escape. Provider-side error
    categorizations are not always trustworthy.
  - Resource Manager v1 vs v3 endpoint flags are separate; v5
    provider uses v3 for newer code paths (Ticket D-2 finding).
  - KMS uses two URL prefixes against the same endpoint (lib client
    /v1/projects/..., template /projects/... — Ticket E-2 finding).
  - awsproto's marshalInnerXML rejects anonymous multi-field structs
    — second time this caught me out (T1 follow-up + Ticket A
    follow-up). Worth a vet rule.

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
| `aws-full-stack` | **Tickets A + B closed (fakeaws `b7db72d`, `fd8e5d1`, `ff0c38d`) — validated end-to-end 2026-05-31 20:00, `target_reached` iter 2** | Closed |
| `gcp-cloud-sql` | **Tickets D-2 + E closed (infrafactory `3b32364`, fakegcp `d9c6545`) — validated end-to-end 2026-05-31 21:45, `target_reached` iter 5** | Closed |
| `gcp-full-stack` | **Closed by D-2 — validated 2026-05-31 22:00, `target_reached` iter 1** | Closed |
| `gcp-gke-cluster` | **Closed by E-2 KMS dual-prefix (fakegcp `a3b1ea8`) — validated 2026-05-31 22:25, `target_reached` iter 3.** NodePool "Plugin did not respond" was downstream of an earlier escape and didn't recur once the LLM converged. | Closed |
| `gcp-secret-manager` | **T5 closed (fakegcp `c6165b1` AccessSecretVersion handler)** | Closed |
| `gcp-storage` | **Closed by E + E-2 (same KMS bugs as gke) — validated 2026-05-31 22:40, `target_reached` iter 4.** Prior "intermittent" label was based on a pre-fix sweep where both failure modes (CMEK destroy 501, no-CMEK policy gate) were unresolvable; both are now fixed. | Closed |
| `web-app-paris` | **Closed by seeded prescriptive pitfall (`0a7efe5`)** | Closed |

### Tickets C + E — CLOSED, Ticket D NEW (2026-05-31 ~22:00)

After A+B validated, ran gcp-cloud-sql to test T3-rest. Surfaced three more issues:

**Ticket C — fakegcp Projects.GetProject — CLOSED.**
fakegcp `e51d5de` added a synthetic GetProject handler at
GET /v1/projects/{project} returning lifecycleState=ACTIVE.
Before: 501-not-implemented for the route, which the v5 SDK
surfaces as a confusing 401 ACCESS_TOKEN_TYPE_UNSUPPORTED that
looks like a real-cloud escape. Now: the GetProject preflight
called by getProject helpers across many resources resolves to
fakegcp.

**Ticket E — kms_custom_endpoint double-path — CLOSED.**
infrafactory commit after `a0768c2` (this session) flipped
kms_custom_endpoint from `%s/v1/` to `%s/`. Same shape as T2
(sql_custom_endpoint) and T11 (dns_custom_endpoint): the v5 cloudkms
lib prepends v1/projects/... to BasePath itself, so trailing /v1/
doubled to /v1/v1/projects/... which fakegcp 501'd. Surfaced in
gcp-cloud-sql iter 5.

**Ticket D — google_project_service Read escapes — CLOSED via D-2.**
Root cause: terraform-provider-google v5.45.2 uses Resource Manager
**v3** for the getProject preflight inside google_service_networking_connection
+ several other newer code paths. We were only setting v1's endpoint
(`cloud_resource_manager_custom_endpoint`), so v3 calls escaped to
real cloudresourcemanager.googleapis.com and surfaced as misleading
401 ACCESS_TOKEN_TYPE_UNSUPPORTED errors. Found via binary-strings
hunt on the v5.45.2 provider:
`GOOGLE_RESOURCE_MANAGER_V3_CUSTOM_ENDPOINT` is a distinct env-var
override.

Fix (Ticket D-2):
  - infrafactory `3b32364` added `resource_manager_v3_custom_endpoint`
    to the provider template (host-only per the strip-regex pattern).
  - fakegcp `d9c6545` added `GET /v3/projects/{project}` handler
    (v3 response shape differs: "name" is "projects/{id}", state
    field is "state" not "lifecycleState", parent is a string).
  - Validated end-to-end: `gcp-cloud-sql` closes target_reached
    iter 5 after self-learning loop convergence.

### Tickets A + B — CLOSED 2026-05-31 evening, validated end-to-end

**Ticket A — fakeaws destroy-preflight no-op handlers — CLOSED.**
First wave (fakeaws `b7db72d`): empty-list handlers for
ListSSHPublicKeys, ListServiceSpecificCredentials, ListMFADevices,
ListSigningCertificates. Pre-emptively added all four to avoid
one-by-one discovery.
Follow-up (fakeaws `fd8e5d1`): converted result types from
anonymous structs to named structs (awsproto encoder's
marshalInnerXML rejects anonymous multi-field structs — same gotcha
as `iamGetUserPolicyResult`). Without this, live responses had
`<!-- marshal error -->` in the result wrapper.
Second wave (fakeaws `ff0c38d`): ListVirtualMFADevices (account-
level virtual-MFA — distinct from user-level ListMFADevices),
DeleteLoginProfile + GetLoginProfile (returns NoSuchEntity via
WriteServiceError, NOT the generic ResourceNotFoundException that
WriteAWSError emits — provider keys off NoSuchEntity exactly).

**Ticket B — managed-policy orphan filter — CLOSED.**
fakeaws `b7db72d` `gatherIAMStateReal` filters
`arn:aws:iam::aws:policy/*` from the `/mock/state` iam.policies
output. AWS-managed stubs (SeedManagedPolicy inserts) no longer
count toward infrafactory's orphan gate; tenant policies
(`arn:aws:iam::<account>:policy/*`) still surface.

**Validation (2026-05-31 ~20:00):** `aws-full-stack` closes
`target_reached` in iter 2 with empty `/mock/state` (0 collections
populated). Iter 1 hit `orphan_check detected 1 orphaned resources`
on an `aws_secretsmanager_secret` in `PendingDeletion` (default
30-day recovery window). The auto-learning loop fed that failure
back to the LLM; iter 2's HCL added `recovery_window_in_days = 0`
(or `force_destroy`) and destroy completed cleanly. The self-
learning pipeline worked — no static pitfall needed.

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

## Next-session candidates (proposed 2026-05-31 23:50 after 39/39)

Now that the sweep is at 39/39 realistic, the remaining work is
durability + cleanup rather than scenario-closure. Six candidates
ranked by value-per-hour:

### N1. Full 39-scenario deterministic sweep — ~30-40 min

**Why:** The session closed all six previously-failing scenarios
via *one-off* validations. That proves each one CAN pass in
isolation but NOT that the whole set stays green simultaneously
under shared mock state. A clean sweep is the actual evidence for
the 39/39 claim.

**Fix:** Run `bash /tmp/sweep-revalidate.sh` (or regenerate from
this session's pattern). Reset mocks between scenarios. Tally
pass/fail. If any scenario regresses, treat the regression as a
new ticket — likely something we missed in the one-off validations
(e.g., scenario-to-scenario state leak that mock-reset doesn't
catch).

**Effort:** 30-40 min wall-clock, ~15 min active.

### N2. Prune stale verbatim mock-quirk pitfalls — ~1 hr

**Why:** `pitfalls/aws.yaml` and `pitfalls/gcp.yaml` accumulated
~10 verbatim mock-quirk entries during the 2026-05-31 sweep
(`aws_kms_key` DateType, `aws_iam_user_policy` empty-result wait
loop, `aws_subnet` MapPublicIpOnLaunch, `google_dns_record_set`
Plugin-did-not-respond, `google_container_node_pool` "not
implemented", `google_kms_crypto_key_iam_member`, etc.). Every one
of these underlying bugs was FIXED this session (T-A, T-B, T-C,
T-D-2, T-E, T-E-2, and the recovery commits). The pitfall entries
are now stale misinformation polluting future LLM prompts — they
teach the LLM to avoid resources that now work fine.

**Fix:** Per `feedback_sweep_protocol.md`: "Pruning stale entries
is OK after a mock-source fix." For each verbatim entry, verify
the underlying mock bug is fixed (run the relevant scenario, see
the resource now works), then delete the entry. Update the
`m91_no_seeding` ratchet test if needed.

**Effort:** ~1 hr (5-10 min per entry, ~10 entries).

**Pair with N3 for max effect** — if T12 lands first, future
sweeps won't accumulate these in the first place, and N2 becomes
one-time cleanup.

### N3. T12 — `isMockActionable` classifier predicate — ~half day

**Why:** The auto-learning pipeline currently writes ANY recurring
failure to `pitfalls/*.yaml` regardless of whether the failure is
LLM-actionable (real provider/cloud constraint) or mock-actionable
(mock-server gap that the LLM can't work around). The mock-actionable
ones are the verbatim mock-quirks N2 has to prune by hand. Without
T12 they keep accumulating every sweep.

**Fix:** Already specified in detail in section 12 below
(`Failure classifier — keep mock quirks OUT of pitfalls`).
Signals: `501 Not Implemented`, `Plugin did not respond`,
`OAuth ... access token`, `couldn't find resource (N retries)`,
`404 ... ResourceNotFoundException` on a Describe* path. When the
predicate fires, append to `docs/mock-gaps.md` instead of
`pitfalls/<cloud>.yaml`. Extend the M91 ratchet to fail CI if any
learned entry matches the predicate.

**Effort:** ~half day. Strict superset of the existing pipeline;
detection rules are simple substring/regex matches.

### N4. awsproto anonymous-struct compile-time guard — ~1-2 hr

**Why:** `WriteQueryRPCResponse(w, "Action", &struct{...}{})` with
a multi-field anonymous struct silently emits
`<!-- marshal error: xml: unsupported type: struct{...} -->`
inside the result wrapper. The handler returns 200 and looks fine
in unit tests if the assertions are loose; only live curl reveals
the broken output. Bit me TWICE this session:
  - T1 `iamGetUserPolicy` initially used an anon struct (fixed
    inline + documented on `iamGetUserPolicyResult`).
  - Ticket A first wave used anon structs for 4 new destroy-preflight
    handlers (`fd8e5d1` named-struct fix + tightened tests).
The single-field anon struct in `iamListGroupsForUser` happens to
slip through, which masks the pattern further.

**Fix:** Two options:
  (a) Refactor `WriteQueryRPCResponse` to require a non-anonymous
      typed argument (use a marker interface or method receiver).
      Compile-time enforcement.
  (b) Add a `go vet` analyzer that walks calls to
      `awsproto.WriteQueryRPCResponse` and flags anonymous-struct
      args with >1 field.
(a) is cleaner; (b) is less invasive. Either prevents the next
person from re-discovering the bug via a confused 30-min debug
session.

**Effort:** ~1-2 hr. (a) needs refactoring all existing call sites;
(b) is a new analyzer in `internal/tools/`.

### N5. `make restart-fakegcp` / `make restart-fakeaws` targets — ~15 min

**Why:** `make up` starts mocks via `go run ./cmd/fakegcp`, which
compiles ONCE at boot. After a commit, the running mock is on the
OLD binary. `kill $(cat pidfile)` only kills the `go run` wrapper
(captured by `$!`), not the compiled child process. Caused a 20-min
misdiagnosis this session on Ticket D-2 ("v3 endpoint flag isn't
working" → no, the binary is stale).

**Fix:** Add Makefile targets that do `pkill -f "fakegcp --port 8081"`
(matches the actual binary by command line, not pid) + restart via
`go run`. Mirror for fakeaws (`pkill -f "fakeaws --port 8082"`) and
mockway. Optionally a single `make restart-mocks` that does all three.

**Effort:** ~15 min.

### N6. Mockway README mirror demo — ~15 min

**Why:** fakeaws + fakegcp READMEs both have a "One-shot demo
(with sibling repos)" subsection pointing at infrafactory's
`make up`. Mockway hasn't been touched yet. Cross-repo polish item.

**Fix:** Copy the existing fakeaws/fakegcp subsection text into
mockway's README, adjust the example scenario path to a Scaleway
one (e.g. `scenarios/training/block-paris.yaml`).

**Effort:** ~15 min.

### N7. `make clean-bg` target + session-close convention — ~30 min

**Why:** Background-task hygiene bit me again this session. The
prior incident (documented in the existing "Session-close hygiene"
section below) covered `Monitor()` calls. This session's lingering
task was an auto-backgrounded `Bash` command — specifically the
`make up` from session start, which the harness backgrounded
because `make` spawns long-running children (mockway, fakegcp,
fakeaws, UI). The wrapper shell stayed in the task tracker for
the entire 4-hour session, triggering the "Background work is
running" prompt at exit.

The broader rule: **any harness task with `run_in_background: true`
needs an explicit `TaskStop` when its purpose is served**, not
just `Monitor()`. This includes:
  - `Bash` with `run_in_background: true` (validation runs,
    background builds).
  - `Bash` commands the harness auto-backgrounds because of
    long-running children (`make up`, `nohup ...`).
  - `Monitor()` calls (already covered in the existing convention).

**Fix:** Two parts:
  (a) **`make clean-bg`** (or `make clean`) Makefile target that
      kills any lingering `bash /tmp/sweep-*.sh`, `tail -F
      /tmp/sweep-*.log`, and per-mock `go run` wrappers from prior
      sessions. Cheap, removes friction at session start when the
      prior session crashed mid-run.
  (b) **Convention update**: extend the existing close-out checklist
      to call out `Bash run_in_background` tasks too, not just
      `Monitor`. A future session that runs a long sweep via
      `Bash(run_in_background=true)` should pair it with a
      `TaskStop` when the sweep finishes (or when bailing out on
      first failure).

(b) is a memory-update; (a) is a small Makefile addition.

**Effort:** ~30 min total.

---

## Recommended order

If picking ONE thing: **N1 → N2** together (~2 hr total). The
sweep proves 39/39 deterministic, and the prune cleans the pitfalls
file based on what the sweep revealed as actually-recurring vs.
fixed.

If picking the most durable thing: **N3** (T12 classifier) —
prevents future sweeps from re-polluting the pitfalls files,
making N2 a one-time job rather than a recurring cleanup.

Quick wins: N5 + N6 + N7 together (~75 min).

---

## Open follow-ups from prior sessions (mostly closed)

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

## Session-close hygiene (lessons from 2026-05-31 + 2026-06-01 closes)

Long sessions accumulate persistent background harness tasks that
survive across sub-sessions and only die at session exit. They
don't hurt anything but they cause the harness to prompt
"Background work is running" when you try to exit. Always call
**TaskStop** on every backgrounded task before closing the session.

**Scope** (broadened after 2026-06-01 incident): the rule covers
NOT JUST `Monitor()` calls but ALSO:
- `Bash` with explicit `run_in_background: true`.
- `Bash` commands the harness auto-backgrounds because of
  long-running children (`make up`, anything `nohup`-style).

Workflow improvements proposed in [[N7]] above:

- **Convention**: every backgrounded task gets a paired `TaskStop()`
  when its purpose is done — don't leave them armed "just in case."
  Especially after a sweep finishes (or stops on the first failure
  for the bail-out pattern) — stop the monitor/wrapper that was
  tailing its stdout immediately.

- **Tooling**: `make clean-bg` (or `make clean`) target that finds
  and kills any lingering `bash /tmp/sweep-*.sh` + `tail -F
  /tmp/sweep-*-stdout.log` processes the previous session left
  running. Cheap, removes friction. (See N7 for full spec.)

Specific background tasks that survived prior-session closes
(stopped manually before exit):
- 2026-05-31: `bzlwxl814` (sweep-progress monitor), `bek3umbwi`
  (sweep 25 reset monitor), `b2s4vo2i5` (revalidation-sweep monitor).
- 2026-06-01: `b7qym91sk` (the `make up` wrapper shell that stayed
  alive the entire ~4-hour session because make spawned long-running
  child mocks).
