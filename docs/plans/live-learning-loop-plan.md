# Closing the loop: teaching infrafactory from what it deployed (S154–S156)

Planned 2026-08-30. Driver: S151–S153 made infrastructure that outlives its run.
Until this arc lands, that produces **logs, not learning** — a live service can
degrade, flap or fail an upgrade and infrafactory is none the wiser.

This is the arc the live-services work exists for. Everything before it was
plumbing.

## Where learning comes from today

One place. `internal/cli/run_command.go` takes `failure.Detail` strings raised
inside a run's stages and routes them to three extractors
(`internal/generator/pitfalls_learn.go`, `prescriptive_extractor.go`):

| extractor | input | produces |
|---|---|---|
| `ExtractDescriptivePitfall(detail, scenario)` | one failure string | a symptom rule |
| `ExtractFixPitfall(failedDir, passingDir, …)` | **HCL diff** between a failing and a passing iteration | prescriptive "do this" |
| `ExtractAvoidPitfall(…)` | the same diff, inverted | prescriptive "never this" |

Everything lands as a `LearnedPitfall{Resource, Rule, Source, DiscoveredFrom}`
appended to `pitfalls/<cloud>.yaml`, with `Source` one of `descriptive` / `fix` /
`avoid` (ADR-0019).

So infrafactory can currently learn exactly one kind of lesson: **the cloud
refused this at apply time**, attributed to a resource, ideally with a diff
showing what fixed it.

## Why live signals do not simply plug in

Four structural differences, and each one is a design problem rather than a
detail.

**1. They arrive after the run is over.** The existing loop is synchronous:
fail → extract → repair → next iteration. A soak failure lands 40 minutes later
with no run in progress, no iteration to repair, and no process waiting for it.
Ingestion has to be out-of-band.

**2. There is no diff to learn from.** `ExtractFixPitfall` is the good
extractor — the one that produces prescriptive rules — and it works by comparing
the HCL of a failing iteration against a passing one. A live failure has no
"next iteration that fixed it". Without a second data point, live signals
degrade to `descriptive`, which is the weakest class.

**3. Attribution is harder.** A run failure belongs to the HCL just applied. A
degradation at t+40min could be the HCL, the image tag, the cloud, or load.

**4. The signal is unbounded.** Run failures are capped by
`repair_iterations_max`. A live failure can repeat every probe, forever. The
existing corpus has never faced a firehose.

## Design decisions

### 1. Live failures enter through the existing extractors, in the existing shape

The extractors take a detail string. An observation that emits in that shape
inherits `NormalizeDetail`, oscillation detection, dedup, holdout validation and
YAML append **unchanged**. The new work is producing the signal and deciding
what deserves to become one — not building a second learning system.

Live-sourced entries carry `source: live`, extending the ADR-0019 vocabulary.

### 2. An observation is not a lesson until it is structural

The promotion gate is the heart of this arc. An observation becomes a pitfall
candidate only when it is **reproduced**, defined as either:

- the same normalized detail across **N consecutive probes** of one deployment
  (persistent, not a blip), or
- the same normalized detail across **≥2 distinct deployments** (a property of
  the shape, not of one machine).

A single 502 never becomes a pitfall. Without this gate, one broken deployment
emits the same lesson forever and the corpus rots — see Risks.

### 3. A pitfall must be actionable for generation, or it is noise

`pitfalls/*.yaml` exists to steer the generator. "The service returned 502" tells
it nothing. "A backend that takes longer than the health-check grace period to
boot leaves the LB reporting the frontend down — declare an explicit grace
period" is guidance it can act on.

This is the hardest part, and the honest position is that **symptom text alone is
not enough**. Where a live failure is later followed by a deployment of the same
scenario that does *not* exhibit it, the two HCL trees form the diff
`ExtractFixPitfall` needs — so S155's upgrade path is what makes S156 produce
prescriptive rules rather than descriptive ones. That dependency is the reason
these three slices are one arc.

### 4. What is running must be verified before it is blamed

The 2026-08-30 canary exposed that `deploy` records the **declared** image
without checking what is serving: the record said `nginx:1.27` while the instance
ran `python3 -m http.server`. A learning loop that attributes a failure to
`nginx:1.27` on that basis is learning a falsehood. **Verifying the running
version is a prerequisite for attribution**, not a nicety, and it lands in S155.

## Slices

| id | slice | produces |
|---|---|---|
| S154 | Soak observation — `live observe` probes every live deployment's health path on a schedule and records the result against its deployment record | the first post-apply signal; no learning yet |
| S155 | Upgrade / rollout — deploy v1→v2 against a running service, **and verify the running version matches the record** | change-time signals, attribution, and the diffs S156 needs |
| S156 | Promotion + extraction — reproduced observations become `source: live` pitfalls through the existing extractors | the loop, closed |

S154 and S155 are independent of each other; S156 depends on both. S156 also has
a hard prerequisite that is not currently scheduled — see below.

## The prerequisite nobody has scheduled: pruning

`docs/plans/pitfall-pruning-automation-plan.md` was **shelved**. The corpus has
no automated way to shed entries that are stale, superseded, or wrong.

That was tolerable while learning was bounded: a run produces at most
`repair_iterations_max` failures, and a scenario that stops failing stops
emitting. Live observation removes the bound. A deployment left running with a
broken image can emit the same normalized failure on every probe for its whole
TTL, across every deployment of that scenario, indefinitely.

The promotion gate (decision 2) limits the *rate*. It does not provide a way to
remove a live-sourced pitfall once the underlying cause is fixed — and a pitfall
that steers generation away from something no longer broken makes every future
generation worse, silently.

**S156 should not merge without at least a retirement path for `source: live`
entries** — an expiry, a supersession rule, or a re-validation against the
holdout set. Unshelving the full pruning plan is the larger option and may be the
right one.

## Risks

| risk | mitigation |
|---|---|
| **Corpus pollution.** Unbounded signal into a corpus with no pruning. | promotion gate; retirement path required before S156 merges |
| **Learning a falsehood.** Attributing a failure to a version that was never running. | verify the running version (S155) before any attribution |
| **Descriptive-only output.** Live signals with no diff produce weak symptom rules. | pair with S155's upgrade diffs; accept `descriptive` only where no pair exists, and mark it |
| **Observation costs money.** Probing requires the deployment to still exist. | observation rides the TTL the deployment already has; no deployment is kept alive to be observed |
| **Noise looks like signal at low N.** | thresholds stated in config, not hard-coded, so they can be raised when the first real corpus arrives |

## What would make this arc succeed

One live-sourced pitfall that is **prescriptive, attributable, and demonstrably
prevents a repeat** — validated the way the project validates everything else, by
running it: generate the scenario with the pitfall absent and observe the
failure, then with it present and observe the failure gone.

One such entry is worth more than a hundred symptom lines, and is the honest bar
for saying the loop is closed.

## Out of scope

Drift detection (periodic re-plan against live state) — cheapest of the three
signal classes and worth doing, but it reuses the plan harness rather than
teaching anything new, so it does not belong in the arc that closes the loop.
Multi-cloud live observation: Scaleway only, as everywhere else in this work.
