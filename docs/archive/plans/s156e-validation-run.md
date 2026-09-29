# S156e — the validation run

Planned 2026-09-03. This is not a code slice. It is the experiment the whole
live-learning arc exists to justify, and it is the only thing that can tell the
difference between "the machinery runs" and "the machinery works".

## What must be shown

> One live-sourced pitfall that is **prescriptive**, **attributable**, and
> **demonstrably prevents a repeat** — proven by generating the scenario with the
> pitfall absent and observing the failure, then with it present and observing the
> failure gone.

Everything from S154 to S163 is machinery in service of that sentence. None of it
has been shown to work end to end against reality.

## What would falsify it, written before the run

The run **fails** — and that is a real result, not a setback — if any of these
hold:

1. **The failure never promotes.** The probe records it but the gate does not
   reach its threshold, or normalisation splits one failure into several.
2. **Nothing attributable comes out.** `live learn` writes a rule with no resource,
   or refuses because the deployments do not agree on one.
3. **The rule is descriptive only.** The upgrade produces no attributable diff, so
   the corpus gains a symptom and no remedy — which is S156c's output, not
   S156e's bar.
4. **The rule changes nothing.** Generation with it present produces the same HCL
   as without. This is the most likely failure and the most important to report
   honestly: a corpus entry that does not alter generation is a corpus entry that
   costs prompt tokens and buys nothing.
5. **It cannot be told from noise.** The LLM produces different HCL on every run
   anyway, so "different with the pitfall" proves nothing unless the difference is
   the specific one the rule describes, reproducibly.

(5) is the one that most needs guarding against. Generation is non-deterministic,
so a single pair of runs is an anecdote. The comparison has to be repeated, and
the *shape* of the difference has to match the rule.

## The failure to manufacture

A live failure, not a terraform failure — the distinction the whole arc rests on.
The apply must SUCCEED and the service must still be wrong, because that is the
class nothing else in this project can see.

The chosen shape: a service whose **health path does not exist**. The load
balancer comes up, the instance runs, `tofu apply` reports success and the orphan
sweep is clean — and the service answers 404 on the path the scenario declared.
Every existing signal calls that green.

That is deliberately a shape the generator *can* get right when told, which is
what makes step 4 meaningful.

## Sequence

1. A scenario with a health path the served application does not answer.
2. `deploy`, then `live observe` until the promotion gate has enough evidence
   (3 consecutive probes on one deployment, or 2 deployments).
3. `live learn` — inspect what it wrote. Stop here and report if it is not
   attributable.
4. `live upgrade` onto configuration that serves the declared path, observe it
   healthy, `live learn` again for the **prescriptive** rule.
5. Generate the scenario **without** the corpus entry, N times. Record how often
   the declared health path is served.
6. Generate **with** it, N times. Compare.
7. Tear down; verify the account is clean; report the cost.

## Cost

Bounded-TTL deployments of the `lb-serving-paris` shape at €0.042/hour, minutes
each. Under the standing authorisation's no-ask threshold. The generation half
costs LLM tokens rather than cloud money.

## What gets written down either way

The result, and the falsification condition it was measured against. **A negative
result closes the arc as honestly as a positive one** — if a live-sourced pitfall
does not change generation, that is the single most useful thing this arc could
discover, and burying it would make every future slice built on the assumption
worse.
