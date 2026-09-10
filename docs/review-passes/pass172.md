# Review pass 172 — Play2 admitted, three claims retracted

`codex exec review --base main` on `fix/play2-types-and-retractions`. Clean on
the first pass, nothing raised.

> I did not identify any discrete correctness regression introduced by these
> changes. The code change is narrowly scoped to widening the Layer 3 enum
> allowlist, and the accompanying documentation/pitfall edits do not introduce
> an actionable risk.

The review had little to find because the substance of this change is **removing
three wrong claims**, all added by me in the previous 24 hours and all falsified
by one real-cloud run.

| claim | falsified by |
|---|---|
| the inline `private_network` block destroys cleanly | run `20260910T104418Z` iteration 4 — that exact shape, same teardown error |
| `docker` is a safe image because the marketplace API says DEV1-M is compatible | iteration 2 — `couldn't find a local image for zone fr-par-1 and commercial type DEV1-M` |
| Play2 is cheaper than DEV1-S, so admitting it cuts spend | Scaleway public catalog — `PLAY2-PICO` +59%, `PLAY2-NANO` +207% |

The change is still worth making, on a corrected reason: **iteration cost, not
instance cost.** DEV1 is the superseded range, Play2 is what a model reaches for
when asked for a small instance, and one run spent three of five iterations
proposing `PLAY2-NANO` and `PLAY2-PICO` and being refused before exhausting its
budget. At demo durations the price difference is fractions of a penny; a wasted
iteration is a real apply. `PLAY2-MICRO` (+513%) stays out, so the ceiling
remains near 3× DEV1-S and the gate remains deny-by-default.

## The pattern under all three

Each claim was checked against something cheaper than the thing it was about.

- The inline block was verified against **mockway**, which does not enforce the
  power-off precondition. It reported `7 added`, `7 destroyed`. A mock more
  permissive than reality cannot refute a claim about reality.
- The image was verified against the **marketplace API**, which is not what the
  provider resolves against — it still insists DEV1-M is compatible.
- The price was not verified at all until after it had been acted on.

This is ADR-0028's limit restated with evidence: facts about the real cloud are
too expensive to pin in tests, so they live in prose — and prose written from a
cheap proxy reads exactly like prose written from the real thing.

The `docker` pitfall is the sharpest case. The note it overrode said, in as many
words, that writing a confident remedy for an undiagnosed symptom is how a
symptom becomes a wrong instruction the generator then follows. It was overridden
on marketplace evidence, the generator followed it, and the apply failed. The
warning is restored and no remedy is offered.

## Left open, deliberately

- **No HCL shape destroys.** The fix moves to the teardown path: power the run's
  instances off before `tofu destroy`. ADR-0029 records the refutation so nobody
  reads its Decision and believes the problem is solved.
- **`IsStuck` misses oscillation.** It compares only the immediately previous
  iteration, so alternating failure signatures run to the full budget. Two of
  those five iterations were real applies.
