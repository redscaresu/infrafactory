# M95 findings — multi-pass auto-learning study

Ran `gcp-full-stack` 5 times back-to-back to study whether the auto-
learning loop (M86 + M90 + M91 + M92) converges over time. Results:

| Pass | Iters | Time | Terminal | Learned before | Learned after | Δ |
|---|---|---|---|---|---|---|
| 1 | 2 | 283s | stuck | 7 | 7 | 0 |
| 2 | 3 | 530s | stuck | 7 | 7 | 0 |
| 3 | 4 | 670s | stuck | 7 | 7 | 0 |
| 4 | 4 | 717s | stuck | 7 | 7 | 0 |
| 5 | 2 | 376s | stuck | 7 | 7 | 0 |

## The mechanism is firing — the rules are inert

Every pass emits 2 `oscillation_pitfall_learned` events
(M86+M90 working correctly), but `pitfalls/gcp.yaml` doesn't
grow because the learned rules are already present from earlier
runs. `isDuplicate` correctly suppresses them.

So the loop CAN learn. The problem is **what** it learns.

## The real finding: rules are descriptive, not prescriptive

The 7 learned entries in `pitfalls/gcp.yaml` look like:

> `google_compute_instance.api_server has no
> network_interface.subnetwork — must be attached to an explicit
> VPC subnetwork`

Compare to a hand-authored seed (M91 stripped them):

> `A google_compute_network and at least one
> google_compute_subnetwork MUST exist before any
> google_compute_instance or google_container_cluster is created.
> Reference the subnetwork via network_interface { subnetwork =
> google_compute_subnetwork.NAME.self_link }.`

The seed is **prescriptive** — tells the LLM what HCL to write.
The learned rule is **descriptive** — echoes the error verbatim.
An LLM seeing the descriptive form knows it failed last time but
doesn't necessarily know what to write differently.

## Implication

The auto-learning loop is a real signal (it detects what's
broken) but not a complete fix-generator. To get a system that
actually converges over time without human help, ExtractLearnedPitfall
needs to transform descriptive errors into prescriptive rules.

Two paths considered for M96:
1. **Template-based**: pattern-match common error shapes (no
   subnetwork → "add network_interface.subnetwork = ...") and
   emit the prescriptive form. Fast, deterministic, limited
   coverage.
2. **LLM-rewritten**: a small post-extraction LLM call that
   reads the failure detail + the converged HCL from the
   eventual success and writes a prescriptive rule. Slower,
   more general, real LLM cost per learning.

User principle from M91: "seeding is cheating." Path 2
preserves that — the prescriptive rule is still derived
from the system's own observations, just rephrased for
actionability.

## What this session proved

- M86+M90+M91+M92 closes the *mechanical* loop: when the LLM
  makes a mistake the system extracts a record of it.
- The mechanical loop is **necessary but not sufficient** —
  records need to be in a shape the LLM can act on.
- M96 is the next-level work: make records actionable.

## Reproducibility

```
bash scripts/m95_multipass.sh
# defaults: SCENARIO=gcp-full-stack PASSES=5
```

Logs in `/tmp/m95_logs/`. Results TSV: `docs/m95-multipass-results.tsv`.
