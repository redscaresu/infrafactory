# Review pass 160 — S156e, second round

**12 findings, 10 accepted, 2 declined.** The theme: the *corrections* from the
first round reintroduced the same false-signal shape one level up.

## Accepted

**The false green, again (findings 1, 5, 9).** `Released` — added last round so an
unreapable project would be reported rather than counted as agreement — was
*appended to the sentence that says they agree*. The line read "…1 released
record(s) whose project still exists (nothing will reap them); the cloud and the
store agree", with `StageStatusPass` and an empty failures array. One line raised
the alarm and withdrew it, and a machine reading the JSON saw success.

Split into `Clean()` (nothing disagrees) and a new `Reapable()` (nothing needs a
human). The exit code stays 0 — `live forget` is deliberate, and failing every
later reconcile for it is the permanent-red defect this arc just removed — but the
summary now ends "nothing is unaccounted for, but the above will not be reaped
without a human", and **names the ids**. Counting them told an operator to go and
cross-read `live ls` against the project listing by hand.

**Records counted nowhere (findings 2, 3).** `ProjectID == ""` was skipped before
every bucket, on the stated grounds that ADR-0024 reports the damage elsewhere.
True and irrelevant: `Examined()` sums buckets, so a store holding one such record
reported holding none — the *same* "0 record(s)" signal, indistinguishable from an
empty or unreadable store, that last round's `Examined()` was written to remove.

It is reached by a real path, not a hypothetical one: `MarkReleased`'s fallback
writes exactly this shape when a record's bytes will not decode, so `live forget`
on a damaged record produces it, and whatever that record named keeps billing. New
`Damaged` bucket, counted and named. The test drives the real path — garbage bytes,
then `MarkReleased` — rather than hand-building the record.

**The docs described the fix that was replaced (findings 6, 7, 8).** The write-up
still said a released record whose project survives "must stay Accounted", which
the shipped code explicitly no longer does, and quoted "examined 3 project(s) and 0
live record(s); the cloud and the store agree" as the post-fix *success* output —
that is the pre-fix wording and the pre-fix bug, quoted as evidence for the fix.
STATUS.md contradicted itself twelve lines apart. All three corrected, and the
quoted line is now labelled as the defect rather than the proof.

**Cosmetic but real (findings 11, 12).** A paragraph inserted mid-sentence in
`layer3-coverage.md` orphaned " As" onto its own line, re-attaching a qualifier to
the wrong paragraph. A dangling `// Worth` fragment in the `Accounted` comment.

## Declined

**4 — the `runs.png` baseline cannot be reproduced from a clean checkout.**

> **This decline was WRONG, and was reversed in S164.** The finding was
> correct. See the correction at the end of this file.

Declined at the time on the grounds below, which were refuted by running it. `git log` does confirm the structural half: `runs.png` was
captured in the first commit and the other four were re-captured in the second. But
the conclusion does not follow, and the visual suite passes 8/8 on a clean worktree
of the PR head, with `make test` including `ui-test-e2e`.

The premise is that an uncommitted `scenarios/training/*.yaml` made the sidebar one
row taller. That region is **explicitly masked** — `visual.spec.ts:17-20`, "adding a
scenarios/training/*.yaml file changes sidebar height on every page, which would
otherwise force a re-baseline". And the 28px measured between `runs.png` and the
others is explained without any defect: `runs.png` captures `fullPage: false`
(viewport only, deliberately, so the growing runs table cannot shift it) while the
others capture full-page, so their pixel extents are not comparable.

**10 — `Examined()` conflates live records with released tombstones.** The
alternative offered is a per-bucket breakdown in the summary line. `Examined()`
answers one question — *was the store read at all* — and that is the question the
"0 record(s)" defect was about; the discriminating detail is already in the
`Released` and `Damaged` clauses, which now name ids. A four-way breakdown on every
run would bury those. Declined as a rewording without a defect behind it.


## Correction (S164): finding 4 was right and the decline was wrong

The decline rested on "the visual suite passes 8/8 on a clean worktree of
the PR head". It does — and that worktree had no `.infrafactory/runs`.

That is the state which decides this image. The masks cover `main table`,
and **in the empty state there is no table to mask**, so a populated run
store and an empty one produce genuinely different pages. The `runs.png`
captured in the first S156e commit encoded an empty store, so:

- a checkout *with* runs fails against it — 56% of pixels differ, and
  `make test` runs `ui-test-e2e`;
- a worktree *without* runs passes against it, which is exactly the
  environment the decline was verified in.

Confirmed after the fact by restoring the pre-S156e baseline (`07eeaca`)
and running it against a populated checkout: it passes, where the S156e
one does not.

The sidebar/`fullPage` reasoning in the decline was accurate and
irrelevant — it explained away a 28px measurement while the real
difference was the table. **A correct sub-argument is not a verdict.**

Fixed at the root rather than by swapping the file: the runs visual test
now stubs `/api/runs`, so the baseline no longer depends on undeclared
local state. Verified in both environments — a checkout with 46 runs and
a fresh worktree with none — which is the check the original decline
should have run and did not.
