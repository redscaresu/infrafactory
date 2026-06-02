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
