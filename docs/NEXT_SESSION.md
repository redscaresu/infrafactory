# Next Session

Self-contained brief for a fresh Claude / engineer starting in this repo.

## S79–S83 arc complete (2026-06-02)

All five slices landed. 38/39 deterministic sweep (+1 from S76's 37/39). Full close-out narrative in `docs/status/ARCHIVE.md` § "2026-06-02 S79–S83".

- ✅ **S79** — fakeaws#5 — KMS tag persistence.
- ✅ **S80** — #59 — `cmd/s3router/` shim (architectural correction caught pre-execution).
- ✅ **S81** — 38/39 sweep. N3 GCP-escape carve-out validated organically.
- ✅ **S82** — #60 — N2 OPA-duplication ratchet + 3 dup deletions.
- ✅ **S83** — this PR — arc close-out documentation.

## READ FIRST (next session)

**Persistent gcp-full-stack failure.** The S81 sweep's only failure was gcp-full-stack `repair_budget_exhausted` on `google_service_networking_connection` ACCESS_TOKEN_TYPE_UNSUPPORTED.

**Important architectural finding from S83 close-out:** `service_networking_custom_endpoint` IS already injected by `internal/cli/generate_command.go` (line 274). So the provider-block override exists. The fact that the request escapes anyway means one of:

1. The LLM-generated HCL is defining a per-resource provider block / alias that overrides the global config.
2. The provider's preflight `Projects.GetProject` is calling a different endpoint than the override targets (despite the comment block at lines 201-216 + `user_project_override = false` + `resource_manager_v3_custom_endpoint` already being set).
3. fakegcp's `cloudresourcemanager.v1.Projects.GetProject` route is returning 401 itself for some reason.

**Suggested first action:** read iterations/1..5 of `.infrafactory/runs/gcp-full-stack/<last>/iterations/N/generated/main.tf` (and related .tf files) and inspect whether the LLM is generating per-resource provider blocks or aliases. If yes — that's the LLM-side fix path (a pitfall rule against per-resource provider blocks). If no — the bug is in `generate_command.go`'s provider-config injection or fakegcp's preflight handler.

Either way, this is a **focused debugging slice**, not a sibling-mock fix. The standing learning loop has already deposited two `source: learned` pitfalls naming the escape resources; the next sweep will exercise them.

## Suggested next arc

**Planned**: `docs/plans/slices-84-88-plan.md` — five slices, ~8-14 focused hours:

- **S84**: gcp-full-stack provider-config investigation (2-hr timebox).
- **S85**: Land the gcp-full-stack fix (scope from S84 — LLM-side pitfall, infrafactory-side injection fix, or fakegcp-side handler fix).
- **S86**: Triage the 4-5 fakegcp `plugin did not respond` mock-gaps entries.
- **S87**: Fix the highest-impact fakegcp panic (one PR; rule-of-three for the rest).
- **S88**: Post-fix 39-scenario sweep + arc close-out. Target: 39/39 deterministic.

Autonomous-execution loop prompt at the bottom of the plan file.

**Sweep entry point**: `make sweep-39`. Output lands in `/tmp/sweep-39/`.

## Open mock-gaps

`docs/mock-gaps.md` is a git-untracked runtime artifact. Stale entries from prior sweeps (`aws_kms_key rotation`, `aws_subnet MapPublicIpOnLaunch`, `aws_route53_record empty result`) all now pass in the S81 sweep — they'll naturally drop when the file is regenerated. No action needed.

The persistent GCP entries from S79–S83 era are addressed by the N3 carve-out routing them out of mock-gaps and into pitfalls (S78 + S81). The remaining `plugin did not respond` entries on fakegcp (`google_kms_crypto_key_iam_member`, `google_container_node_pool`, `google_compute_instance`, `google_sql_database_instance`) are the next sibling-mock arc candidates — likely fakegcp panics on a specific request shape.

## Recent arcs (full close-outs in `docs/status/ARCHIVE.md`)

- **S79–S83 arc** (2026-06-02): sibling-mock drainage + carve-out validation. 4 PRs.
- **S74–S78 arc** (2026-06-02): AWS/Scaleway phase3 collapse + `make sweep-39` + N3 carve-out. 5 PRs.
- **S68–S72 arc** (2026-06-02): N3 coverage + M96/M98 close-outs + `cmd/n10extract` CLI. 5 PRs.
- **S63–S67 arc** (2026-06-02): 39/39 deterministic sweep, `infrafactory mock reset` CLI. 5 PRs.
- **S54–S62 arc** (2026-06-02): 9 GCP retirements + ADR-0018 retirement framework. 9 PRs.
