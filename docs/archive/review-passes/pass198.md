# Review pass 198 — S191 stale private-NIC claims

`codex exec review --base main`, 2026-09-20.

## Findings

### [P2] Update the persisted Scaleway pitfall too — ACCEPTED

> For Scaleway runs that load `pitfalls/scaleway.yaml`, the generator will still receive
> the already-learned rule saying not to use `scaleway_instance_private_nic` because it
> "cannot be destroyed" / is only deletable while powered off. Changing this denial text
> does not stop that false premise from being taught on fresh runs.

Correct, and **worse than stated**. The reviewer says the pitfall is taught "before a new
policy failure is even emitted"; in fact the corpus is fed to the generator on *every*
run, failure or not. So the stale claim had a wider reach than the denial message I was
fixing — the message only appears when the rule fires, the pitfall appears always.

I fixed the message, the gate, and two ADRs, and missed the one input that is
unconditional. That is the same shape as the bug itself: a claim frozen in a place nobody
re-reads.

Two entries fixed:

- an auto-learned entry that is a **truncated copy** of the old denial message, ending
  mid-sentence at "and th...". Rewritten as the positive instruction only.
- a hand-written entry asserting "Neither shape destroys cleanly ... POWERED OFF", which
  ADR-0029's Refutation disproved on 2026-09-10 and which survived that correction.
  Replaced with the 2026-09-20 measurement.

The truncated entry is worth noting separately: auto-learning captured a denial message
verbatim and clipped it, so a *fragment* of a sentence was being taught as a rule. Not
this slice's problem to fix, but it means the corpus can inherit any error in a denial
message, which raises the cost of getting those messages wrong.

### [P3] Align the gate documentation with the new reason — ACCEPTED (pass 199)
### [P3] Correct the stale teardown rationale — ACCEPTED (pass 200)

Two more, both the same thing one paragraph apart: the emitted message now says
standardisation while the docblock above it still argued destroy-safety, and then the
allowlist-contrast paragraph still said this gate "answers can this be destroyed".

Accepted rather than declined despite being P3s. A stale rationale sitting beside the
code that gets edited is **exactly** the mechanism that let this claim survive three
corrections — the message got fixed in September twice and the comment explaining it did
not. Leaving it would have been a bet that the next reader is more careful than the last
three were.

The docblock now records the demotion explicitly (safety → standardisation), carries the
2026-09-20 measurement, and ends with a line telling future readers **not** to restore
the power-state explanation, with the ADR that refutes it. The function is still called
`layer3UndestroyableResourceProblems`; renaming it touches the audit tests, so the
comment says the name is historical rather than evidence.

## Pass 201 — clean

> The changes consistently update the Layer 3 refusal text, OPA denial message, and
> persisted Scaleway pitfalls to stop teaching the stale destroyability premise. [...] I
> did not find a correctness issue introduced by the diff.

## What this slice actually was

Not a code change — a **fact** change. The logic was already right: `vpc_required` has
accepted both shapes all along, and the gate still refuses the same resource it always
did. Everything edited here was prose that a machine reads: a denial message fed to the
repair loop, a gate refusal recorded verbatim as a pitfall, two corpus entries loaded on
every run, and the comments that explain them.

Four separate places held the same expired claim, and three of them were found by review
rather than by me.
