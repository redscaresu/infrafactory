# Codex review pass 6 — docs/layer3-coverage.md

Documentation-only. 5 passes, 3 findings, all accepted — unusually
productive for a doc, because the doc makes factual claims that drive
real-money decisions.

| Pass | Findings | Outcome |
|---|---|---|
| 1 | 1 (P2) | accepted in part, refuted in part |
| 2 | 1 (P2) | accepted |
| 3 | 1 (P2, same issue) | accepted |
| 4 | none | — |
| 5 | none | **converged** |

## Pass 1 — half right, and the half that was right mattered

Codex claimed `scaleway_ipam_ip` and the VPC gateway types did not appear
in the artifacts at all. **Refuted**: `ipam_ip` is emitted by
`compute-lb-multi-paris` and `web-app-paris`, the gateway trio by
`web-app-paris`. The table was accurate.

**But the prose was overstated**, and that part was correct: it said "any
scenario with private networking pulls it in", when 10 scenarios declare
private networking and only 2 emit IPAM.

Checking it surfaced something worth more than the correction: because the
LLM writes the HCL, topologically similar scenarios diverge.
`mysql-ha-paris` and `private-lb-db-paris` both attach servers to a private
network and pulled in no IPAM; `compute-lb-multi-paris` did. The table is a
snapshot of what the generator *has* produced, not a guarantee of what it
*will*. Now stated in the doc.

## Passes 2 and 3 — the refresh command contradicted its own table

Same finding twice. The documented refresh scanned only each run's final
`generated/*.tf`, while the table came from that plus
`iterations/*/generated/*.tf` — so following the procedure would silently
drop real blockers, including the two IPAM/gateway rows above.

A document whose refresh contradicts its own table is worse than no
document: the next reader trusts the command over the prose.

Scanning iteration snapshots is also the correct semantics rather than a
reproducibility patch. A scenario that converged on iteration 3 still
generated the types from iterations 1 and 2, and Layer 3 would have
attempted to apply them. The gates must cover what the generator might emit
*on the way to* converging, not only what it landed on.

## Note

Three findings on a document with no executable code. Worth remembering
before treating docs-only PRs as loop-exempt — this one asserted facts
about which real cloud permissions to widen.
