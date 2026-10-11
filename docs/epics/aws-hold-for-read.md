---
status: active
hld: 2026-09-30-import-production-state-into-the-mocks
depends_on: []
---

# An AWS path applies aws-web-live under one Layer 3 claim, keeps its pre-destroy tfstate, holds the stack until a read is done, then destroys and reaps, so Rollout 0's real recording has a command to run (today `--keep` is refused for every cloud but Scaleway, internal/cli/layer3_cloud.go).

**Done when:**
- One command (a dedicated one, or `--keep` widened for this path only; decided in the first story) applies aws-web-live on the Layer 3 account under the claim, writes the tfstate under `.infrafactory/` (0700 dir, 0600 file), prints that the stack is held and how to release it, and refuses a second claim while held.
- Release destroys, runs the sweep and reaps to empty, and releases the claim; an interrupt or a failed hold still reaches destroy, sweep and release (the existing interrupt rules, #436/#438).
- The held state is never under `runs/` or `live/` (the UI serves neither) and never committed; the account id stays out of committed files and PR bodies.
- ADR-0040 amended for the hold; docs/operations.md runbook names the command and its cost bound.
- Proved once on real AWS by the lead: apply, hold, release, sweep empty, cost recorded.

**Out of scope:** the read itself (prod-read-seam); holding any scenario but aws-web-live; Scaleway's `--keep`.

**Constraints:** ADR-0040 whole-scope ownership and claim rules; ADR-0023 containment unchanged; €5 cap, serial real-AWS runs; new paths inherit the existing path's unwritten rules (S156d).

**Your part:** none: the lead runs the real proof under the €5 cap.

**Areas:** `internal/cli/ (layer3_cloud.go, run_keep.go or a new hold command, golden files)`; `internal/harness/ (claim, sweep, reap reuse)`; `docs/decisions/0040-*`; `docs/operations.md`
