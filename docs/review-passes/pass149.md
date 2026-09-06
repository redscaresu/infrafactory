# Review pass 149 — S163e, fifteenth `/code-review` round

**10 findings, 7 accepted, 3 declined.** Two accepts are round fourteen's own
fixes, and one is a claim I put on middleware that had no business making it.

## The cleanup raced the page it was cleaning up for

`loadDetail` is a round trip. A deploy started just before a navigation can END
during it — a 423 lock refusal answers at once — so the arrival hook, which asks
"is this finished now that detail has arrived?", deleted a refusal **before it had
ever rendered**. The button reverted to "Deploy…" as though the click had not
landed, which is precisely the defect moving refusals into the store was meant to
close.

The hook now retires only what was ALREADY finished when the navigation began, from
a snapshot taken synchronously in `afterNavigate`. Reproducing it in a test needed
the POST and the scenario fetch interleaved by hand; a first attempt passed against
the broken code and proved nothing.

## Dismissing a report took a success banner with it

Round fourteen made `dismissReport` delete the entry when the last report went, on
the argument that "the outcome that produced this report is not rendered anywhere
once the report is gone". True only for outcomes that ARE reports. Fail, retry,
succeed, dismiss — and the retry's "Deployed. It is listed on the Deployments page
until its TTL expires." vanished along with the leak the reader had just dealt
with.

## A deploy-specific claim on middleware that refuses everything

`guardCrossOriginRequests` wraps every endpoint, so making it a `writeRefusal`
stamped `started_nothing` — a claim about whether an APPLY created cloud
infrastructure — onto a refused `GET /api/runs` and onto a refused teardown, where
it is a claim about the wrong verb. Reverted to a plain error. A deploy client
reads a cross-origin 403 as "we do not know", which errs in the safe direction, and
a page the server did not serve is not one whose reader is watching a deploy.

## The rest

- `readJSON` was added so a truncated body could not surface as "Unexpected end of
  JSON input" — and applied to `request`'s error branch only, leaving the defect the
  comment describes alive on every 2xx.
- `deploying` was read off the wire as `payload?.deploying || []`, so a server
  predating the field licensed the page's only permitted emptiness claim.
  `knownEmpty` takes a `deployingKnown` flag now, and refuses without it — the same
  absent-vs-empty distinction `already_live` is given two files away, on the same
  contract.
- Past the cap, `appendProgress` rebuilt the whole array on every line. Trimming in
  batches makes it amortised; the cap exists for responsiveness during a long apply,
  and trimming a thousand-element array several thousand times only bounds the stall
  it was meant to remove.
- A leftover `get` import and a docstring describing an implementation that had been
  replaced.

## Declined

- **`knownEmpty`'s `deploying = []` default.** The wire-boundary half of this is
  accepted above. A caller omitting the argument is a programming error, not a
  condition the server can produce, and every current caller passes it.
- **The teardown half of `mayHaveCreated`.** Declined in round fourteen and recorded
  in STATUS as a named follow-up: a different verb, a different store shape and a
  different page.
- **`estateSummary` derives the applying count twice.** Declined in rounds eleven,
  twelve and fourteen.
