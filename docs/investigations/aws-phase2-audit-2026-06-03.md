# AWS phase2 audit per ADR-0018 (S91 + S92)

Date: 2026-06-03
Slices: S91 (audit) + S92 (retirements)
Outcome: **0 retirements. All 10 rules are Category C.** AWS phase2 was designed lean from the start.

## Audit table

| # | Rule (abbreviated) | Category | Rationale |
|---|---|---|---|
| 1 | "Generate complete, runnable Terraform/OpenTofu HCL for the planned architecture." | C | Task framing; no machine-readable signal. Removing it leaves the LLM with no top-level instruction. |
| 2 | "Use `terraform { required_providers { aws = "hashicorp/aws", version = "~> 5.70" } } }`." | C | System contract — provider pinning is an environmental constraint, not a learnable pattern. A pitfall couldn't encode "use this version" because the version isn't a failure-mode signal. |
| 3 | "Do NOT use `data` sources — the mock environment does not support data queries." | C-leaning (initially flagged B) | Data-source failures have machine-readable signals (`data.aws_X.Y not found`), and the auto-learning loop COULD eventually capture them. But: data sources are diverse (`aws_caller_identity`, `aws_region`, `aws_availability_zones`, etc.), each with distinct failure modes. The blanket prompt rule covers all variants in one line. A pitfall-driven replacement would need 5+ entries, each scenario-specific. Net: leave as C — pre-emptive blanket cheaper than per-shape learning here. |
| 4 | "Follow every applicable pitfall above…" | C | Meta-instruction; describes pitfall usage rather than encoding a learnable rule. |
| 5 | "Use account-synthetic, run-scoped names for globally-unique resources (S3 buckets); use predictable names for VPC-scoped resources." | C | Naming convention that encodes mock-environment knowledge. Failures ("bucket name in use") would be scenario-specific and the rule itself doesn't have a learnable failure path — it's prescribing behaviour the LLM otherwise wouldn't know about. |
| 6 | "Organise files logically (e.g., main.tf, network.tf, iam.tf…)" | C | Organisational; no machine-readable signal. |
| 7 | "Include a `providers.tf` with the required_providers block plus provider 'aws'…" | C | System contract. Missing provider config fails `tofu validate`, but the failure mode is too coarse for a pitfall (it would say "add provider block" — which is too generic). |
| 8 | "Include a `variables.tf` with any configurable values. Every variable MUST have a `default` value." | C-leaning (initially flagged B) | The "no default value" `tofu plan` error IS machine-readable, and the auto-learning loop could capture it. But: the rule is short, concise, and applies to every scenario uniformly. A learned-from-diff pitfall would say the same thing in more words. The N11 retirement test would: delete the rule → expect the LLM to occasionally omit defaults → learn back into a pitfall → confirm the pitfall is at least as effective. Realistic but low-ROI: ship a multi-iteration regression for a rule that's already small and right. Skip. |
| 9 | "Include `outputs.tf` with useful outputs…" | C | Optional in real terraform; the "useful outputs" criterion is non-mechanical. |
| 10 | "Ensure all resources reference each other correctly via OpenTofu references." | C | General best-practice that the LLM already knows. Failure mode (using string literals instead of references) is rare. |

## Comparison with prior collapses

- **GCP phase2** (S54–S73): collapsed 17 → 9 rules. The original 17 included prescriptive resource-specific patterns (CMEK on storage buckets, deletion_protection on Cloud SQL, VPC requirements on GKE, etc.) that the auto-learning loop subsequently replaced.
- **AWS phase3** (S74): retired 2 sub-bullets from a `self_review.md` checklist (DB subnet group ordering, SG cycle avoidance).
- **Scaleway phase3** (S75): retired 1 sub-bullet (private NIC requirement).

AWS phase2 differs: it was authored AFTER the auto-learning loop existed (per the AWS support arc in 2026-05), so the original author already kept it system-contract-only. The 10 rules are all what ADR-0018 calls "Category C in retrospect" — they wouldn't have been added as prescriptive in the first place.

## S92 outcome

**No retirement candidates.** Per `slices-89-93-plan.md` S92 plan: "may close as 'no retirement candidates' if S91 finds AWS phase2 is all Category C — that's a valid outcome."

S92 closes as documented. No PR-worthy code change; this audit doc is the artifact.

## Notes for future

- If a future sweep surfaces a `data.aws_X.Y` failure pattern across 3+ scenarios, revisit rule 3 — that's the rule-of-three threshold for promoting a prompt rule to a learnable pitfall.
- If `tofu plan` "no default value" failures recur across 3+ scenarios, revisit rule 8 similarly.
- Until then: the prompt is at the right shape.
