# ADR-0019: Learning-system vocabulary — concept names over slice IDs

## Status
Accepted (2026-06-04)

## Context

The auto-learning loop's vocabulary leaked internal slice IDs (N3, N10, N13, M97) into user-facing surfaces:

- Code function names (`IsMockActionable`, `ExtractPrescriptiveFix`, `ExtractPrescriptiveAvoid`, `ExtractLearnedPitfall`).
- `pitfalls/*.yaml` `source:` enum values (`learned`, `learned_from_diff`, `learned_from_diff_avoid`).
- Sweep harness output (`N13_EMISSIONS=N`).
- Four READMEs across the project family (infrafactory + mockway + fakegcp + fakeaws), AGENTS.md, the auto-learning explainer, ADRs.

The slice IDs were the only labels that existed when each component was introduced; they never got renamed. As infrafactory moves toward OSS visibility (all four repos Apache-2.0, pending public flip) this was real onboarding tax — a new reader can't parse "N13 silent this sweep" without first decoding which slice introduced N13.

ADR-0012 (dynamic pitfalls) and ADR-0015 (classifier routing) carry the original architecture decisions but use the slice-ID vocabulary throughout.

## Decision

Rename the active vocabulary from slice IDs to concept-based names. Atomic single-PR cutover, no grace period.

| Layer | Was | Now |
|---|---|---|
| Classifier | `IsMockActionable` | `IsMockServerBug` |
| Fix extractor | `ExtractPrescriptiveFix` (N10) | `ExtractFixPitfall` |
| Avoid extractor | `ExtractPrescriptiveAvoid` (N13) | `ExtractAvoidPitfall` |
| Descriptive fallback | `ExtractLearnedPitfall` | `ExtractDescriptivePitfall` |
| Binary | `cmd/n10extract` | `cmd/extract-pitfall` |
| YAML `source:` | `learned` / `learned_from_diff` / `learned_from_diff_avoid` | `descriptive` / `fix` / `avoid` |
| Sweep keep-flag | `--keep learned_from_diff_avoid` | `--keep avoid` |
| Sweep summary | `N13_EMISSIONS=N` | `AVOID_EMISSIONS=N` |
| CI ratchet | `TestPitfallsNoMockActionableSeeds` | `TestPitfallsNoMockServerBugSeeds` |

Slice IDs remain in commit history, ARCHIVE entries, prior ADRs, and memory pointers as historical attribution — they were the names when each piece was introduced, and the changelog shouldn't churn for a rename.

Alternatives considered:
- **Dual-accept grace period** — loader reads both old + new source values for one arc, then drops old. Rejected because there are no external consumers of `pitfalls/*.yaml` outside this repo; an atomic cutover is simpler and avoids dead-code retention.
- **Keep slice IDs, document the mapping** — would have left every doc reader to chase the mapping. The mapping table still exists as a footnote in `docs/auto-learning-loop.md`, but the live vocabulary is now self-documenting.
- **Per-component naming** (e.g. `fix` vs `prescribe`, `avoid` vs `prohibit`) — Fix/Avoid won because they already appeared in `--mode fix|avoid` on the `extract-pitfall` binary and matched the conceptual labels used throughout code comments.

## Consequences

**Benefits**
- OSS-ready vocabulary: a new reader of `docs/auto-learning-loop.md` doesn't need to chase commit history to know what each component does.
- The `source:` enum value matches the function that emits it (`ExtractFixPitfall` → `source: fix`), removing one level of indirection.
- The sweep summary line names the actual signal (`AVOID_EMISSIONS`) rather than its slice number.

**Tradeoffs**
- `git blame` on `internal/generator/pitfalls_learn.go` now points at S104 for every line, masking the original introducing-slice for some readers. Historical attribution lives in commit messages + ADR-0012 / ADR-0015 / ADR-0018.
- ARCHIVE entries and ADRs continue to reference slice IDs; readers need to map between the two vocabularies when reading historical context. The mapping table in `docs/auto-learning-loop.md` is the canonical reference.

**Follow-up**
- None mandatory. If a future arc introduces a fourth extractor or a new pitfall source value, it should use concept names from day one — no slice ID in the public vocabulary.

## Amendment, 2026-09-01 (S156a): `live`, and the first source with a lifetime

The vocabulary gains a fourth value: **`live`**, for a pitfall learned from a
service that was already running rather than from a failed apply.

It is the first source with a **lifetime**. The others do not need one: a rule
extracted from a reproducible apply failure does not stop being true because
nobody hit it lately. A live rule does, once the thing it describes is fixed —
and learning used to be bounded, because a run emits at most
`repair_iterations_max` failures and a scenario that stops failing stops
emitting. Live observation removed that bound.

So `live` entries carry `last_seen`, and `pitfalls retire` removes those not seen
within a retention window. Three rules, all of them about refusing to delete
learning on weak evidence:

- **No timestamp is never retired.** Absence means nobody recorded when the rule
  was last true, which is not evidence it stopped being true. An unparseable
  value counts as absent, because treating it as zero would make every malformed
  entry infinitely stale and delete it.
- **The boundary is exclusive**, so a 14-day window does not delete something last
  seen 14 days ago to the second.
- **A non-positive window is refused**, on the dry-run path as well as the real
  one. A mistyped flag would empty the corpus in one command, and a dry-run that
  accepts what the real run rejects teaches the wrong thing about what is safe.

Removal is reported, never silent — the rule the D6 purge established. A corpus
that quietly drops entries is indistinguishable from one that never learned them.

Adding a source value is not a three-line change. `live` had to be learned by the
two corpus ratchets, the sweep's preservation list, and the path guard on the
maintenance command — four places, found across four review passes, three of them
mechanisms that already existed in the repository. **A vocabulary is only as real
as the things that enforce it**, and the next value added here should start by
enumerating them rather than by adding the constant.

## Amendment, 2026-09-01 (S156b): reproduction is the gate, not severity or provenance

An observation becomes a candidate lesson when it has **reproduced** — N
consecutive probes on one deployment, or ≥2 distinct deployments. Not when it is
severe, and not when Terraform also failed.

That last one is worth stating because it is the intuitive answer and it is
wrong. A Terraform failure is already learned from by the run loop; live
observation exists for the failures Terraform reports as **success**. The
strongest lesson this arc produced came from an apply that passed while the
service kept serving the old version — under a terraform-failure gate it would
produce nothing at all.

Four things the gate refuses, each a way it could be wrong rather than
incomplete:

- **A recovery breaks the run.** A healthy probe between two failures means the
  service recovered, which is the blip the gate exists to reject.
- **A different failure breaks it**, because only one thing can be true of a
  service at a given probe.
- **`unhealthy` and `unreachable` never merge.** One of each is not two of either.
- **A rule with no thresholds promotes nothing.** A misconfigured gate that opens
  is worse than one that closes.

Attribution is **recorded, not filtered**. Something was broken whether or not
the running version was confirmed, but a rule blamed on a version nobody verified
is a falsehood — so the fact travels with the candidate and the extractor decides.

The gate produces candidates and not pitfalls, deliberately: it can then be
judged on whether it promotes the right things without simultaneously arguing
about rule text, which is the harder and far more subjective half.

## Amendment, 2026-09-01 (S156c): a live lesson is attributed to what was probed

The corpus is keyed by resource. A live observation names none — *"the thing at
this address returned 503"* is about an endpoint — and
`ExtractDescriptivePitfall` refuses such a detail on purpose: **skip rather than
fabricate**.

The resource a live lesson belongs to is **the one the probed address was
resolved from**. `LiveEndpointResource` reports it and `deploy` records it, at
the only moment it is a fact rather than an inference. Where it cannot be
established, or where the deployments exhibiting a candidate disagree about it,
nothing is written and the operator is told — a corpus that looks complete when
it is not is worse than one that admits a gap.

Two further rules follow from the corpus's own shape:

- **A candidate spanning two clouds writes nothing.** The corpus is per-cloud, so
  either choice files half the evidence where it does not apply.
- **Every live entry is stamped with `last_seen`.** S156a never retires an
  unstamped entry, so writing one without a timestamp would make it immortal —
  the inflow silently undoing the outflow. Re-learning refreshes rather than
  duplicates.

And the rule text stays descriptive: it states what was observed and what the
evidence was, and does not invent a remedy. A descriptive rule that fabricates a
fix is worse than one admitting it has none; the prescriptive form comes from an
upgrade diff (S156d).

A `source: live` entry's identity is its **`observed_key`** — the normalized
detail the promotion gate grouped by — and not its rule text.

The text is unstable by design: a live rule states its evidence, and the evidence
grows as the same failure keeps being observed. Identifying an entry by its text
therefore makes one lesson look new on every pass, so the corpus gains a copy per
cron tick while appearing to refresh. Everything that identifies a live entry —
the append path and the sweep merge — keys on `observed_key`, and a refresh
rewrites the text so the corpus carries the strongest evidence rather than
whichever was written first.

An entry without a key is refused: it could never be recognised again, and
writing something unmaintainable is worse than writing nothing.
