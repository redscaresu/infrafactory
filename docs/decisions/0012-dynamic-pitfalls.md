# ADR-0012: Dynamic Pitfalls by Cloud Provider

## Status
Accepted

## Context
Provider pitfalls (e.g., "don't use `ip_id`, use `ip_ids`") were hardcoded in `prompts/phase2_generate_hcl.md`. Every new pitfall required a code change. The LLM feedback loop could self-correct within a single run, but forgot the fix between runs — rediscovering the same mistake every time.

## Decision
Externalize pitfalls into `pitfalls/{cloud}.yaml` files loaded at runtime based on the scenario's `cloud` field. Implement auto-learning: when a run self-corrects (iteration N fails, N+1 succeeds), extract the error pattern and append it to the pitfalls file.

1. **File-per-provider**: `pitfalls/scaleway.yaml`, `pitfalls/gcp.yaml`, etc. Optional `pitfalls/common.yaml` merged into all providers.
2. **Runtime injection**: `LoadPitfalls(dir, cloud)` renders pitfalls as markdown, injected via `{{.Pitfalls}}` in phases 2 and 3.
3. **Auto-learning**: `ExtractLearnedPitfall` parses failure details for actionable patterns (password constraints, unsupported arguments, missing config). `AppendPitfall` writes to YAML with deduplication and `source: learned`.
4. **Conservative extraction**: Only specific, actionable errors produce pitfalls. Vague errors ("test checks failed") are ignored.
5. **Best-effort**: Learning errors are logged but never break the run.

## Consequences
**Benefits**:
- New pitfalls added by editing a YAML file — no code changes.
- System gets smarter over time — each self-correcting run teaches future runs.
- Multi-provider ready — add `pitfalls/gcp.yaml` when GCP support lands.
- Deduplication prevents pitfall bloat.

**Tradeoffs**:
- Learned pitfalls may be noisy if extraction patterns are too broad.
- YAML file grows over time — may need periodic human review to promote `learned` → `static` or prune low-value entries.
- Conservative extraction means some learnable patterns are missed.

## 2026-06-02 amendment — diff-based prescriptive extractor (N10)

The original `ExtractLearnedPitfall` is symptom-only: it captures
the failure detail verbatim as the pitfall rule. That's enough to
teach the LLM WHAT failed but not HOW to fix it. The 2026-06-02
sweep made this concrete: `gcp-storage` learned `"missing
encryption.default_kms_key_name"` after every failed iteration but
never converged because the rule never told it to declare a
`google_kms_crypto_key` + reference the `.id` via an `encryption {}`
block.

This amendment adds a second extraction path, `ExtractPrescriptiveFix`
in `internal/generator/prescriptive_extractor.go`. It triggers on
the success path (`terminal_reason == target_reached`) rather than
the stuck/budget path, walks adjacent `(iter[N-1], iter[N])` pairs,
and for each failure cleared between them diffs the HCL bodies of
the failing resource to produce a snippet that the LLM can lift
verbatim on the next iteration.

Discriminator: `LearnedPitfall.Source = PrescriptiveSource`
("learned_from_diff"). Stored in `pitfalls/<cloud>.yaml` alongside
the legacy `source: learned` entries; the existing `AppendPitfall`
writer handles both via the `pitfallSource` defaulting helper.

Scope intentionally narrow:
- Only validate / apply failures with a parseable resource address.
- Only the failing resource's body changes + new sibling resources
  referenced from those new attributes.
- Skip whitespace-only diffs (the `normalizeLine` collapser).
- Skip when the failing block didn't change between iterations.
- Skip cross-cloud (resource type prefix must match scenario cloud).
- 600-byte cap on the snippet to keep prompt injection bounded.

**Why this matters for the prompt strategy.** Prescriptive rules
13–16 in `prompts/gcp/phase2_generate_hcl.md` (and parallels for
aws / scaleway) are hand-written translations of "successful HCL
patterns observed when the LLM eventually got it right." N10 makes
those translations a build artifact: the system re-derives them
from real runs. Ticket N11 in `docs/NEXT_SESSION.md` covers the
follow-up retirement of these prompt rules once their N10 counterparts
have proven stable.

## Amendment (2026-06-02, S55 audit fixes)

The first S55 production audit surfaced three issues with the N10
wiring that this amendment captures:

1. **iterationHistory only contained failed iterations.** The
   `run_command.go` extractor loop iterates iter pairs by index but
   the success branch broke without recording the passing iter, so
   `len(iterationHistory)` never exceeded the failure count. A
   2-iter "1 fail → 1 pass" scenario had `len == 1` and the loop's
   `len > 1` guard skipped extraction entirely — only multi-failed
   runs ever produced entries. Fix: also append the passing iter
   (with empty failures) before breaking. The N10 design assumed
   iterationHistory tracked ALL iterations; that assumption is now
   true.

2. **`trimSnippet` cut at the last newline, not a block boundary.**
   The 600-byte snippet cap is intended to keep the rule small
   enough to inject into prompts, but a mid-line slice produced
   unbalanced HCL (`depends_on = [` left open). Refinement: prefer
   cutting after a column-0 `}` (top-level resource close); fall
   back to line-trim only if no block boundary fits within the cap.
   The truncation marker `# ... (truncated)` is preserved either
   way.

3. **M91 no-human-seeding ratchet rejected `learned_from_diff`.**
   M91's existing whitelist of `source: learned` excluded N10's
   `PrescriptiveSource = "learned_from_diff"` value, which would
   have CI-failed every legitimate N10 entry as if it were a human
   seed. Whitelist extended to include `PrescriptiveSource`.

A new ratchet `TestPitfallsLearnedFromDiffSnippetCap` enforces the
600-byte cap at the on-disk artifact level (rule length bounded by
`snippetMaxBytes + 400`), catching future trim-logic regressions.

These are wiring fixes, not architectural changes — the core
mechanism described above remains correct. The amendment exists so
a fresh reader can connect the on-disk behaviour (now-correct N10
firing on every passing post-failure iter pair) to the original
design intent.

## Amendment (2026-06-02, N13 deletion-as-fix)

N10's `ExtractPrescriptiveFix` captures **addition-as-fix** patterns:
the LLM cleared a failure by ADDING HCL. The 2026-06-02 sweep
surfaced the dual case repeatedly — failures cleared by REMOVING
HCL (an attribute the provider rejected, a resource type that
escapes to real cloud). The clearest motivating cases:

- `deletion_policy = "DELETE"` on `google_cloud_run_v2_service` —
  the LLM hallucinated this attribute; tofu validate rejects it;
  the fix is dropping the line.
- `google_project_service` / `google_project_iam_member` — these
  resources escape fakegcp to real `cloudresourcemanager.googleapis.com`;
  the fix is removing them entirely (PR #18 + #23 hand-retired the
  prompts; N13 makes those retirements re-derivable from runs).

`ExtractPrescriptiveAvoid` (this amendment) implements the dual:

- New source tag `PrescriptiveAvoidSource = "learned_from_diff_avoid"`
  distinguishes the avoid form from the add form in
  `pitfalls/<cloud>.yaml`.
- Two attribution paths:
  - **Attribute-level**: the LLM removed an attribute from the
    failing resource's body AND the attribute name appears verbatim
    in the failure detail. The strictest signal — provider/policy
    named the offending attribute, the LLM dropped it, the failure
    cleared.
  - **Resource-level**: ALL instances of a top-level resource type
    were dropped between iters AND that type name appears in the
    failure detail. Partial removals (some instances retained)
    skip — ambiguous between "the type is forbidden" and "the LLM
    narrowed legitimately."
- Rule shape: `"<failure summary> Do NOT use <attribute|resource
  type> on <resource> — observed in scenario X to cause the failure
  above."`
- M91 no-human-seeding ratchet + S55-T3 size ratchet both
  whitelist `PrescriptiveAvoidSource` alongside `PrescriptiveSource`
  — same provenance, different rule shape.

The two extractors run together per cleared-failure iter pair:
either or both may emit. The N10 addition extractor's logging
event remains `prescriptive_pitfall_learned`; the N13 avoid
extractor emits `prescriptive_avoid_learned` so per-event triage
stays clean.

**Why now (post-N11 instead of as part of N10).** N11's prompt-rule
retirements landed before N13 because the manual deletions
(prompts/aws/phase3 RDS, prompts/scaleway/phase3 RDB+LB,
prompts/gcp/phase2 rules 9 + 12 + 16 + 11 + 13 + 14 + 15 + 10)
proved the auto-correction channel carries the load. N13 makes
those retirements RE-DERIVABLE — any future "do NOT use this
attribute / resource" pitfall is one passing run away from the
file. The N10→N11→N13 sequence converts hand-written prescription
to a self-maintaining artifact for both addition and removal
patterns.

## Amendment (2026-06-02, S64 — case-insensitive attribution)

The S63 first-production sweep surfaced a false-positive shape that
N13's `strings.Contains(failureDetail, attr)` attribution couldn't
handle: the AWS API error for the `aws_subnet`
`map_public_ip_on_launch` update timeout echoes the JSON-side field
name `MapPublicIpOnLaunch` (camelCase) verbatim. The HCL attribute is
snake_case, so the strict contains-check returned false even though
the iter-pair diff plainly showed the LLM had removed
`map_public_ip_on_launch` to clear the failure. Result: N10's addition
heuristic instead grabbed unrelated added attrs (`cidr_block`,
`availability_zone`) and emitted a `learned_from_diff` pitfall that
didn't actually encode the fix.

The amendment introduces `attributeAppearsInDetail(detail, attr)`
which tries three matches in order:

1. Literal substring (existing behaviour).
2. Case-insensitive substring on the snake_case form. Catches errors
   that echo back the attribute with different casing but the same
   underscores.
3. camelCase variant via `snakeToCamel(attr)`. Tries both
   PascalCase (`MapPublicIpOnLaunch`) and lower-camelCase
   (`mapPublicIpOnLaunch`).

The motivating regression test is
`TestExtractPrescriptiveAvoid_CamelCaseAttributeInFailureDetail` —
real AWS error shape, snake_case HCL, asserts N13 attributes the
removal correctly. Existing four tests remain green; no shape
change to the other code paths.

This is a wiring fix, not a design change. The N13 extractor remains
strict (only emits when there's an attribution path between the
deletion and the failure) — it now just recognizes one more legitimate
attribution path.

## Amendment (2026-06-02, S69 — M96 close-out + extractor layering)

M96 (BACKLOG, filed 2026-05-30) flagged that `ExtractLearnedPitfall`
produced descriptive rules ("X failed because…") rather than
prescriptive ones, and proposed two paths: (1) template-based
rewrites, or (2) a small LLM post-extraction call that synthesises a
prescriptive rule from converged HCL.

The S69 audit (after Slices 54-67) found that M96's question was
already answered architecturally — not by either of the proposed
paths, but by a layered set of extractors that each cover a
different convergence state:

**Layer 1 — N10 `ExtractPrescriptiveFix` (addition-as-fix)**:
fires when `terminal_reason == target_reached` and at least one
prior iter failed. Diffs the last-failing iter's HCL against the
first-passing iter's HCL, emits a `learned_from_diff` rule with the
minimal HCL snippet that resolved the failure. The strongest signal
when available.

**Layer 2 — N13 `ExtractPrescriptiveAvoid` (deletion-as-fix)**:
same trigger as N10 but for the dual case — the LLM cleared the
failure by REMOVING HCL (an attribute the provider rejected, a
resource that escapes to real cloud). Emits a `learned_from_diff_avoid`
rule of the shape "do NOT use X — observed to cause the failure
above."

**Layer 3 — M97 templates (in `ExtractLearnedPitfall`)**: fire on
every failure, not just on target_reached. Pattern-match common
error shapes (missing subnetwork, CMEK, unsupported argument,
destroy blockers, etc.) and emit prescriptive rules without needing
a converged run to learn from. Cover the still-stuck-runs niche
where N10/N13 can't fire yet.

**Layer 4 — descriptive fallback (in `ExtractLearnedPitfall`)**:
captures the raw failure detail when no template matches. Last
resort. Symptom-only, but still better than no learning for novel
failure shapes.

The four layers are *complementary*, not competing:
- A converging run produces an N10 + possibly an N13 entry. The
  M97 template that may have fired earlier in the same run is
  silently superseded (the M91 ratchet doesn't drop it, but the
  `learned → learned_from_diff` dedupe path replaces it).
- A still-stuck run produces an M97 entry on the offending
  iteration, which lets the LLM see the prescriptive shape on
  the next attempt.
- A run with a novel failure shape (no template match, no
  convergence) produces a descriptive fallback entry that surfaces
  the issue for a human to look at.

M96 closes as superseded — no code change. The architectural
answer was the N10→N11→N13 sequence, not a path-1 vs path-2
choice on `ExtractLearnedPitfall`.
