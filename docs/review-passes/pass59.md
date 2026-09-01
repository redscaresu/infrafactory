# Codex review pass 59 — S156a

One finding, accepted. **A vocabulary added without updating what enforces it.**

## [P2] The source ratchets still enforced the old enum

`LiveSource = "live"` was added as a persisted value while two repository
ratchets still fenced the corpus to `descriptive` / `fix` / `avoid`:

- `TestPitfallsSourceEnum` — the positive set
- `TestPitfallsNoHumanSeeding` — the "no human authorship" policy

So the moment S156c wrote the first live entry, CI would have rejected the corpus
and the failure would have pointed at the *entry*, not at the ratchet that never
learned about it.

Both updated, and `live` earns its place in the seeding ratchet on the merits: it
is run-derived too, just from a later moment — the run is a service already
deployed rather than an apply. The policy that ratchet enforces is *no human
authorship*, and an observation of a real service is the opposite of that.

## What sweeping for the same class found

`cmd/pitfall-merge` preserves `--keep avoid` across a sweep, and
`scripts/sweep_39.sh` passes exactly that. So **a sweep would have silently
deleted every live entry** — which is precisely what this slice exists to
prevent. Retirement names what it removes; a sweep dropping the same entries on
the floor makes the corpus untrustworthy in the way the reporting was meant to
fix.

Now `avoid,live`, in both the flag default and the sweep.

That one was not in the finding. It was found by asking *where else does this
vocabulary have to be known?* — which is the habit the S155b passes were trying
to teach, applied before the next review rather than after it.

## Nothing declined this pass.
