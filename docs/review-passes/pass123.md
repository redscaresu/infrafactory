# Codex review pass 123 — S162c

One finding, **P1**, and the sharpest of the session.

## [P1] The confirmation could describe one scenario and create another

SvelteKit reuses this component across scenario routes. An open confirmation from
scenario A survives navigation to scenario B, and `confirmDeploy` posted
`detail.name` — which by then is B.

So a person could read A's shape, A's cost, A's expiry and A's blast radius, click
*"Deploy and keep it running"*, and **create B's infrastructure**.

That is worse than having no confirmation at all. A confirmation that describes
one thing and does another converts a careful person into a confident one: they
have checked, and what they checked was not what happened.

## Fixed on both sides, because either alone leaves a hole

- `confirmDeploy` posts `preview.scenario` — the scenario the displayed text came
  from. The action now matches the description by construction, whatever the page
  is showing.
- `afterNavigate` clears the confirmation anyway. Accepting a stale one would now
  be harmless, but leaving it on screen invites the reader to trust a dialog about
  something they are no longer looking at.

## Where it came from

The confirmation was built to satisfy ADR-0027 §2 — state cost, lifetime and blast
radius before the click — and it did, correctly, for the scenario it was opened
on. The defect is not in what it says but in the **binding between what it says
and what it does**, which the ADR did not think to require because a CLI cannot
get it wrong: `infrafactory deploy <scenario>` names its target in the same
sentence as the decision.

Worth noting as a category: a UI can separate a description from its action in a
way a command line structurally cannot, and every confirmation dialog in this
project inherits that risk.
