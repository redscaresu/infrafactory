# Codex review pass 114 — S162a

One finding, raised as **P3**, accepted anyway.

## [P3] Two files sharing a stem could load the wrong scenario

Discovery matches on the scenario's **name** and returns an extension-less path.
So `shared-stem.yaml` and `shared-stem.yml` holding *different* scenarios would
let the loader answer about whichever suffix it tried first.

Accepted despite the priority because the priority measures the wrong thing here.
The trigger is unlikely; the consequence is a confirmation dialog showing the
cost, lifetime and blast radius of a **different scenario**, immediately before
somebody agrees to spend money. A rare path into a wrong-and-confident answer at
the exact moment of an irreversible decision is not a P3 outcome.

The fix is also cheap enough that arguing about the priority would have cost more
than doing it: the loader now checks that the scenario it loaded is the one that
was asked for. That closes the case **generally** — any future mismatch between
path resolution and name resolution is caught — rather than closing the one
filename layout that exposed it.
