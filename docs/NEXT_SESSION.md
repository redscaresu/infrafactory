# Next Session

Self-contained brief for a fresh Claude / engineer starting in this repo.

## Read first

**🎯 Baseline: 39/39 deterministic, sustain-validated across two arcs.** S95 (3 sweeps): 39/39 + 39/39 + 32/39 (transport tail). S100 (3 sweeps): 39/39 + 38/39 + 39/39 (the 38/39 was a tofu init 502 — provider-registry transport, not a regression). S101 in-loop retry now recovers both Claude CLI rate-limits AND OpenTofu provider-registry blips.

**First organic N13 emission preserved**: `pitfalls/aws.yaml` now carries `aws_subnet` "do NOT use `map_public_ip_on_launch` — observed in aws-eks". S94's selective discard worked as designed.

**Scaffold shape (Option C)** — goal-named, variable-length arcs.

## Last arc complete

`docs/plans/sustain-under-renamed-vocab-plan.md` — fifth Option C arc. Full close-out: `docs/status/ARCHIVE.md` § "2026-06-05 sustain under renamed vocabulary".

- ✅ **S105** (this PR): three consecutive `make sweep-39` runs under the renamed vocabulary. All 39/39 deterministic (117/117 total). Zero panics, zero transport retries needed. Zero hits for legacy source-enum literals (`learned` / `learned_from_diff` / `learned_from_diff_avoid`) in post-merge `pitfalls/*.yaml`. Zero hits for legacy summary names (`N13_EMISSIONS` / `learned_from_diff* lines` / `N13 durability`) in master logs. Classifier routed an organic mock-gap (aws-secrets-manager KMS DescribeKey 404) in sweep 3, confirming `IsMockServerBug` live. One stale-vocab leak found + fixed: local `docs/mock-gaps.md` header (the writer code already uses `IsMockServerBug`; only the boilerplate I wrote during S103 was pre-rename text).

## Suggested next arc

- **fakeaws aws-secrets-manager KMS DescribeKey 404 after key destroy** — S105 surfaced this as a mock-gap. ~30-60 min single-slice arc against fakeaws.
- **Layer 3 real-cloud validation** — open since S93. Genuinely deploys to real AWS/GCP/Scaleway. Big arc (cloud credentials, money, cleanup discipline). High value but high coordination cost.

## Open tickets

None — `docs/tickets/rename-learning-system.md` closed by S104.

## Sweep entry point

`make sweep-39`. Output: `/tmp/sweep-39/summary.tsv` + `panics.log` + per-scenario logs. New summary lines from this arc:
- `PASS=X / TOTAL=Y (deterministic: X/Z; transport_failed: W)` (S97)
- `PANIC_LINES=N` (S87)
- `N13_EMISSIONS=N` (S94)
- `RETRY_TRANSPORT=N` (S101 attempted retries)
- `RETRY_RECOVERED=M` (S101 succeeded on retry)
- `TRANSPORT_FAILED=N` (S97 end-of-sweep classification, post-retry)

## Recent arcs (full close-outs in `docs/status/ARCHIVE.md`)

- **sustain re-validation + transport retry** (2026-06-04): 2 PRs. First organic N13 entry; transport-retry shipped.
- **post-sustain tightening** (2026-06-03): 4 PRs + 1 fakeaws. aws-route53 + classifier + rule #13 + prompts ratchet.
- **sustain + N13 durability** (2026-06-03): 2 PRs. First Option C arc.
- **S89–S93** (2026-06-03): 🎯 39/39 first deterministic. 3 PRs.
- **S84–S88** (2026-06-03): gcp-full-stack convergence + panic gate. 3 PRs.
- **S79–S83** (2026-06-02): sibling-mock drainage + carve-out validation. 4 PRs.
- **S74–S78** (2026-06-02): AWS/Scaleway phase3 collapse + `make sweep-39` + N3 carve-out. 5 PRs.
- **S54–S73**: GCP phase2 collapse, sustain ratchets, N3/N10/N13 architecture build-out. ~22 PRs.
