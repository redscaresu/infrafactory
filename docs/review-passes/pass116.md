# Codex review pass 116 — S162a

Two findings, both accepted. The second is one I introduced **against a rule I
had written in the same file, one commit earlier.**

## [P2] `cloud: genesys` scenarios previewed as creating nothing

Genesys scenarios declare `routing`, `identity`, `architect` and `full_stack` in
`scenario.schema.json`. The Go `Resources` struct carries none of them, so those
scenarios unmarshal into an **empty resource set** — and the preview reported an
empty component list and €0.00.

That is the most reassuring possible way to be wrong, on the screen somebody reads
before agreeing to spend money.

Verified before accepting: the four blocks are in the schema at lines 69–72, and
`grep` finds no corresponding field in `scenario.go`.

The fix is a `Modelled` flag rather than adding the fields, because adding them is
a change to the scenario model that the generator also reads — out of scope for a
preview slice. **`Modelled` distinguishes "nothing to create" from "nothing I can
see"**, and a scenario that genuinely declares no resources stays the harmless
case it is.

## [P3] Free components made a complete estimate look incomplete

Pass 115 added VPC and private network to the component list — through the
*unpriced* path. So a Scaleway scenario with a VPC reported `Complete: false` and
"AT LEAST", despite every component being perfectly well understood.

`CostComponent.Priced` exists precisely because **free and unknown are different
facts**; the type's doc comment says so in those words. I then added free things
through the unknown path four lines below it.

The cost of that is not the flag. It is that "AT LEAST" appearing on estimates
that are not uncertain teaches a reader to ignore it, and the one time it means
something is the one time it needs to be read.

`addFree` is now a separate path: listed in the blast radius, priced at zero,
`Complete` untouched.
