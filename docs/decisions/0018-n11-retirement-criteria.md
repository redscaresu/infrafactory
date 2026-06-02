# ADR-0018: N11 prompt-rule retirement criteria

## Status
Accepted

## Context

Prompts under `prompts/<cloud>/phase{2,3}_*.md` accumulated dozens of prescriptive
rules over Slices 33–60. Each rule encoded "use HCL pattern X for resource Y because
otherwise the provider/policy rejects it." That worked while the rule count was small,
but doesn't scale:

- Every new resource the LLM trips on adds another rule.
- Rules drift from reality as the LLM, the providers, and the mocks evolve.
- The prompt grows unboundedly; the marginal rule is increasingly noise.

N10 (`ExtractPrescriptiveFix`, ADR-0012 amendment) closes the addition-as-fix gap by
deriving prescriptive HCL snippets from real run diffs and writing them to
`pitfalls/<cloud>.yaml`. N13 (`ExtractPrescriptiveAvoid`, ADR-0012 amendment) closes
the deletion-as-fix gap. Together they make the prompt-rule retirement question
answerable per-rule: "is the rule still needed, given the auto-learning channels are
in place?"

S56–S60 executed nine N11 retirements following a 7-step protocol (defined in
`docs/plans/slices-54-62-plan.md`). The retirements covered a range of patterns:
single-attribute corrections (firewall network-vs-subnetwork), cross-resource
multi-attribute patterns (GKE single-node-pool), and high-stakes structural rules
(VPC + subnetwork). This ADR codifies the criteria that emerged.

## Decision

A prescriptive prompt rule retires through one of three exit paths from the 7-step
protocol:

### Category A: Redundant — delete with no follow-up

The rule was load-bearing only as a hint, but the LLM produces the correct HCL even
without it. The auto-correction channel (provider validate errors, OPA policy
failures, mock-server responses) carries the load through the dynamic feedback loop.

**Signal**: the affected scenario passes iter 1 (or iter 2 with self-correction) after
the rule is deleted, with no matching pitfall in place.

**Examples**:

- GCP phase2 rule 11 (firewall network vs subnetwork). `tofu validate` reports
  "Unsupported argument" — the LLM corrects in one cycle.
- GCP phase2 rule 13 (GKE single-node-pool). `tofu plan` reports "default node pool
  conflict" — same shape.
- GCP phase2 rule 14 (Cloud SQL teardown + private IP). Three distinct failure modes
  (destroy gating, region restriction, no_public_sql policy), all machine-readable.
- GCP phase2 rule 15 (GCS test setup).
- AWS phase3 rule 3 sub-bullet (RDS `deletion_protection = false`). Provider default
  is already `false` — the rule was preventing a non-existent regression.
- Scaleway phase3 rule 7 (RDB `private_network` + LB `assign_flexible_ip` conflicts).

**When**: prefer this category for any rule whose violation produces a clear
provider / validator / policy error message containing the offending identifier.

### Category B: Replaced by `learned_from_diff` pitfall

The rule is load-bearing — deleting it without a backup causes the scenario to fail —
but an N10-derived pitfall in `pitfalls/<cloud>.yaml` (`source: learned_from_diff`,
`source: learned_from_diff_avoid`, or a pre-N10 `source: learned` entry) already
encodes the same prescriptive pattern. The pitfall replaces the prompt rule.

**Signal**: with the prompt rule deleted AND the matching pitfall blanked, the
scenario fails. With the pitfall restored, the scenario passes.

**Examples**:

- GCP phase2 rule 10 (VPC + subnetwork wiring). The auto-learned
  `google_compute_instance` + `google_container_cluster` entries in
  `pitfalls/gcp.yaml` already encode "always declare an explicit VPC + subnetwork."
  They were auto-learned in an earlier loop session and carry the rule.

**When**: prefer this category for cross-resource patterns where the LLM needs more
than a single validator error to converge — typically multi-attribute or
multi-resource patterns.

### Category C: Load-bearing — keep

The rule is load-bearing AND no equivalent pitfall exists OR the pattern doesn't
auto-derive cleanly. Categories include:

- **System / contract rules**: provider version pin, file structure, "use hardcoded
  values from the architecture plan", "do NOT use data sources." These apply from
  iter 1 onwards with no preceding failure for N10 to learn from.
- **Scenario-bound rules**: region restrictions ("allowed list: us-central1,
  europe-west1, ..."), naming conventions. These are scenario or system intent,
  not a fix the LLM derives.
- **Rules whose violation has no machine-readable signal**: e.g., a rule encoding
  a subtle architectural preference (deletion-order, IAM least-privilege) where
  `tofu validate` says nothing.

**Examples** (post-arc):

- GCP phase2 rules 1–9 (system + ADR-0014 escape-prevention guidance for the
  hand-applied retirements of `google_project_service` / `google_project_iam_member`
  pending N13 organic re-derivation).
- GCP phase2 rules 16 (region) + 17 (naming).
- GCP phase2 rule 12 (Service accounts and IAM, kept because IAM principal-format
  is one of those "no machine-readable signal" edges; the LLM hits an obscure
  ACCESS_TOKEN_TYPE_UNSUPPORTED with no actionable detail).

**When**: a rule resists retirement after multiple sweep iterations OR is
foundational to every scenario (system rules).

## Consequences

- **Prompt size collapses over time**. The 2026-05-23 baseline of GCP phase2 had 17
  rules. After the sustain-arc retirements (CMEK + firewall + GKE + SQL + GCS + VPC,
  six retirements), it has 11 rules. Each future "Category A" retirement is
  detectable from a single scenario re-run.
- **Pitfalls files become the living artifact**. `pitfalls/<cloud>.yaml` is now the
  prescriptive source of truth for resource-specific gotchas — not the prompts. The
  M91 no-human-seeding ratchet + the M97 mock-actionable classifier + the S55-T3
  size cap together prevent the file from re-acquiring junk.
- **Adding a new prescriptive rule is rarely necessary**. The default path for "this
  scenario fails on a new pattern" is: let it run, let N10/N13 capture the diff,
  the pitfall becomes the rule. Only Category C rules belong in prompts.
- **Future audit**: any prompt rule whose violation produces a machine-readable
  error is a Category A candidate. Periodic protocol runs (one rule per session)
  will collapse the prompt further.
- **Risk**: an over-aggressive retirement could mask a fragile recovery path
  (e.g., the LLM converges 90% of the time but stuck-detection kills iteration on
  the 10% bad runs). Mitigation: the protocol's step 5 re-run is necessary, not
  sufficient — periodic deterministic sweeps (S54 cadence) catch regressions.

## Follow-up

- Retire GCP phase2 rule 12 (Service accounts and IAM) once a sweep produces a
  `learned_from_diff_avoid` for `google_project_iam_member` (N13 organic
  validation).
- Audit AWS phase3 and Scaleway phase3 for the remaining prescriptive bullets
  every 1–2 sweeps; expect 1–2 more retirements per cloud.
- N10's hand-written `cmd/n10extract` tool (used + removed in 2026-06-02 loop
  session) could land as a permanent CLI command if forced extraction becomes a
  routine retirement step.
