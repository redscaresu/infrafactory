# S158: an end-to-end test for the live lifecycle

Planned 2026-09-01. Driver: **there is no test that runs the live journey.**

`internal/e2e/` holds twelve files and none of them mentions `deploy`,
`live observe`, `live upgrade` or `livestore`. Every guarantee in S151–S156a is
covered by unit tests plus a real-cloud run that a person drove by hand, one
command at a time. Nothing re-runs that sequence.

## Why this is worth a slice of its own

The live commands are coupled through a **record**, not through function calls.
`deploy` writes it, `live observe` appends to it, `live upgrade` rewrites three
of its fields, `live teardown` releases it, and `live reap` acts on whatever it
finds. Unit tests cover each command against a record it constructs itself.

That is precisely the shape that hides defects, and this session produced the
evidence:

- `live observe` failed on a record `deploy` had legitimately written without an
  address — found by a real deploy, not by a test (S154 pass 44).
- `live upgrade` wrote back a record it had read before a minutes-long apply,
  discarding observations `live observe` appended in between — found by review,
  not by a test (S155b pass 57).
- The record's project id and the marker's could disagree, and three separate
  passes were needed to settle which one an apply may trust (S155b passes 53–55).

Every one is an interaction between two commands. **No unit test could have
caught any of them**, because each command's tests build their own record.

## Why now, and not as part of S164

The UI arc already contains this work, at the very end: S164 is *"Playwright e2e
over the whole journey, then one real run."* That ordering made sense when the
UI was the only consumer. It does not now.

The CLI lifecycle is complete and merged. Leaving its only end-to-end coverage
inside the last slice of an arc that has not started means every UI slice is
built on four commands that have never been run together by anything but a
person. Pulling it forward costs one small slice and gives every later slice a
regression test underneath.

## Scope

One test, driving the real commands in sequence against **mockway**, plus a
single real-cloud pass.

| step | asserts |
|---|---|
| `deploy` | a record exists, with project id, address, port, health path and expiry |
| `live ls` | the deployment appears, `HEALTH` reads `unobserved` |
| `live observe` | records an observation and confirms the declared version |
| `live upgrade` | preflight confirms the old version, apply runs, verify confirms the new one; `.infrafactory-previous/` holds what was replaced |
| `live observe` again | the second observation appends rather than replacing |
| `live teardown` | destroy, project delete, sweep, record released |
| after | `live ls` shows the record released; `live reap` finds nothing to do |

Two properties the sequence exists to pin, which no single-command test can:

- **The record round-trips through every command.** Each writes fields the next
  reads, and this is the only test that would notice if one stopped agreeing
  about a field's meaning.
- **Observations survive an upgrade.** The pass-57 defect, as a test rather than
  a review finding.

### Against the mock, and once for real

Mockway is the default so the test can run in CI on every PR. The real-cloud
pass is manual and recorded, exactly as the S154/S155 canaries were — the mock
proves the commands agree with each other, and only real Scaleway proves they
agree with the world.

### Explicitly out of scope

`live reap`'s TTL expiry path beyond the trivial "nothing to do" assertion:
making it fire needs either a clock seam or a real wait, and both are bigger
than this slice. Noted rather than smuggled in.

## What this does NOT prove

The same limit the S168 canary stated: a mock run proves the commands agree with
each other. It does not prove any of them is right about Scaleway. That is what
the real pass is for, and why it stays in the slice rather than being replaced
by the mock.
