# Codex review pass 118 — S162a

**Clean.** No correctness issues; the full Go suite passes.

Six passes, nine findings, and the through-line is one thing: **this endpoint's
job is to tell somebody the truth at the moment they decide to spend money, and
almost every way to get it wrong is a way of being reassuring.**

- an unpriced component summed as zero → cheaper than reality
- a Genesys scenario with no modelled resources → "creates nothing, costs nothing"
- `iam` omitted from the shape list → a smaller blast radius than reality
- a private load balancer billed a public IP → over-warning, and a warning that
  fires on everything gets skipped
- a compute-only scenario reported not internet-facing → **less exposed than
  reality**, at the confirmation step
- an undeployable preview expiring in the year 1 → a date where "none" belongs

Only two of the nine were mechanical. The rest were the estimate quietly
flattering the thing being estimated.

The two structural fixes are the ones worth keeping: `InternetFacing()` derived
from the component list so the bill and the warning cannot disagree, and
`TestEveryDeclaredResourceBlockAppearsInThePreview` walking the struct by
reflection so a new resource type cannot silently vanish from the confirmation.
Both replace a hand-written condition that had already been wrong once.
