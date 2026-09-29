# Slice 53 — Go-test-level idempotency walkers for mockway + fakegcp

Status: in_progress (2026-05-23)
Owner: claude + codex

## Motivation

`tofu apply` against `mockway` and `fakegcp` was being smoke-tested via shell scripts (`mockway/scripts/test-examples.sh`, `fakegcp/scripts/e2e.sh`) gated behind env vars (`FAKEGCP_ENABLE_E2E=1`) that nobody runs in normal CI. Six examples drift on second apply:

- **mockway**: `basic_instance`
- **fakegcp**: `basic_instance`, `dns`, `gke_cluster`, `iam`, `storage`

Each is a real mock-fidelity bug (second `tofu plan` reports drift after `apply`). They've been failing silently because the harness wasn't in any blocking CI gate.

`fakeaws` already has the right pattern: `examples/provider_smoke_test.go` walks `examples/working/`, `examples/misconfigured/`, `examples/updates/` and runs `tofu init → apply → plan -detailed-exitcode → destroy` per dir, gated by an env var but surfaced as named Go test failures. The auto-discovery hooks into the coverage_matrix audit so every working-dir entry has to pass idempotency or it's flagged.

S53 lifts that pattern to mockway and fakegcp, plus adds a `known_broken.yaml` allowlist so the current failures don't block the gate while they're being debugged one-by-one.

## Design — `known_broken.yaml`

YAML at `examples/known_broken.yaml` in each repo. Schema:

```yaml
# examples/known_broken.yaml — known-broken examples allowlisted from
# the provider_smoke_test idempotency gate. Each entry is a single
# example dir + the failure symptom + a tracking ticket id.
#
# Removing an entry is the closing step of the matching fidelity-fix
# ticket. New broken entries require a code-review comment justifying
# why the fix is being deferred.

entries:
  - dir: working/basic_instance
    symptom: "second apply reports drift on field X"
    ticket: M42

  - dir: working/dns
    symptom: "..."
    ticket: M43

  # ...
```

Each entry's `dir` is a relative path under `examples/`. Tests treat
entries on the list as *expected failures* — they record the result but
don't fail the test. If a dir on the list starts PASSING, the test
fails ("congratulations, remove this entry"). This is the
ratchet-only-tighten pattern.

## Tickets

| id | title | priority | deps |
|---|---|---|---|
| S53-T1 | mockway: `examples/provider_smoke_test.go` Go walker + `examples/known_broken.yaml` seeded with `basic_instance`. Gated by `MOCKWAY_ENABLE_E2E=1`. | P1 | — |
| S53-T2 | fakegcp: same walker pattern + `examples/known_broken.yaml` seeded with 5 entries (basic_instance, dns, gke_cluster, iam, storage). Gated by `FAKEGCP_ENABLE_E2E=1`. | P1 | — |
| S53-T3 | Tighten ratchet: surface known_broken entries in CI logs even when test passes overall (so the list shrinks visibly across PRs). | P2 | S53-T1, S53-T2 |
| M42–M47 | Per-failure fidelity tickets (one per known-broken entry). Each ticket: capture the exact `tofu plan -json` diff, identify the handler field/path, fix, remove from known_broken.yaml. | P2 | S53-T1/T2 |

## Walker contract (mirrored from fakeaws/examples/provider_smoke_test.go)

For each directory under `examples/working/`:
1. Build the mock from source on a free port (no shared mock state across dirs).
2. `tofu init -input=false -no-color`.
3. `tofu apply -auto-approve -input=false -no-color`.
4. `tofu plan -detailed-exitcode -input=false -no-color`. **Exit code 2 = drift = fail** (unless dir is in known_broken).
5. `tofu destroy -auto-approve -input=false -no-color`.

For `examples/misconfigured/`: same flow but **expect apply to fail with a documented AWS/GCP/Scaleway error code**.

For `examples/updates/`: apply v1 → plan → apply v2 → plan → destroy.

## Exit criteria (S53)

- `MOCKWAY_ENABLE_E2E=1 go test ./examples/...` runs the full walker; known_broken entries skip the idempotency gate but log a warning.
- `FAKEGCP_ENABLE_E2E=1 go test ./examples/...` same.
- coverage_matrix entries (from S52) and walker results agree — every `examples/working/<dir>/` is either covered by a matrix entry OR documented in `*_exempt`.
- M42–M47 opened, each pointing at the symptom field in known_broken.yaml.

## Why this matters

Without the structural gate, future PRs can add new working examples that quietly drift on second apply and nobody notices. The current 6 failures snuck in because the harness was opt-in shell. Lifting it to Go-test + CI-gated turns "we hope it's idempotent" into "it cannot regress."
