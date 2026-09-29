# Codex review pass 53 — S155b `live upgrade`

Two findings, both accepted.

## [P1] The project came from the record, not the marker

`sandboxCommandEnvForProject(runtime, d.ProjectID)` took the project from the
deployment record — the half a stale or hand-edited file can change. This call
**applies real infrastructure** into whatever it names.

`live teardown` learned this in pass 37 and prefers the marker; I did not carry
the lesson into new code. A disagreement is now refused outright rather than
resolved in the record's favour, because applying into a project this workdir
does not own could change infrastructure belonging to another deployment.

**Applying deserves at least the care destroying gets.** That it took a review to
notice, in a slice written days after the guard that established the rule, is the
finding worth remembering.

## [P2] Verification could pass by checking that nothing changed

With no `--tag`, the record still names the **old** version, so `upgrade_verify`
compared the old tag against the service — which trivially confirms, and reported
`now confirms nginx:1.27` as though a transition had been proven. A green built
from checking that nothing changed.

Now `tagChanged` gates the wording: without a new tag the stage says the service
is *unchanged rather than upgraded*, which is what the evidence actually supports.
Setting `--tag` to the value already recorded counts as unchanged too.

## Nothing declined this pass.
