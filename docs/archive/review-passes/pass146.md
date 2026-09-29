# Review pass 146 — S163e, twelfth `/code-review` round

**12 findings, 9 accepted, 3 declined.** Two of the accepts are the previous
round's fixes undone by code the previous round did not touch.

## The report survived the retry and died on the navigation

Round eleven made `beginDeploy` carry reports forward, so clicking Deploy again
could not delete "project 7c98d82e is live and could not be deleted".

`forgetReportlessDeploy` then judged the entry by its **last outcome** and deleted
the **whole entry**. Fail, retry, succeed, navigate — the second attempt's outcome
says there is nothing to report, so the entry goes, and with it the first
attempt's leak. The unit test could not see it: it asserts before any navigation.

The predicate reads `reports` now. Pinned by an e2e that fails when the check is
removed — the unit test does not, because it exercises the store while the defect
was in the component.

## The same ordering inversion, in the other handler

`handlers_deploy_preview.go` reads the in-flight list before the estate, says why
in a paragraph, and pins it with a test. `deploymentsHandler` did the opposite —
so a deploy finishing between the two reads is in neither, the payload says
`deployments: []` and `deploying: []`, and the estate page derives **"Nothing is
deployed."** at the exact moment the scenario went live and billable.

Swapped, and pinned by the same shape of test.

## Two identical failures collided

Reports accumulate because two attempts leak two projects — and two attempts can
fail identically, with the same dropped connection and the same message. The
layout's `{#each}` keyed on `scenario + message`, so an identical pair produced
duplicate keys: Svelte throws in a dev build, and a production build silently
collapses them, showing one leak where there are two. The index is in the key now.

## Attribution, on both branches and anchored

Two defects in one rule:

- `namedUnlessSelfDescribing` guarded only the refusal path; the other branch
  called `named()` unconditionally, so a `Deploy` error whose text embeds the
  scenario rendered it twice.
- The check was a bare `includes`, so a scenario named `json` matches "invalid
  json body" and renders unattributed — the very defect the prefix exists to
  prevent. The one server message that names a scenario formats it as `"%s is
  already deploying…"`, so the test is anchored at the start now.

## The rest

- The layout banner and the page's outcome slot rendered the same report, so the
  reader saw one sentence twice with the scenario name three times. Reports belong
  to the layout, because they have to outlive the page; the page's slot shows the
  endings that are not reports.
- The e2e named "a refusal that arrives after a detour is still reported" fulfilled
  423 without `started_nothing`, so it never reached the refusal branch at all —
  deleting `startedNothing` entirely left it green. One of four fixtures that
  drifted.
- `types.ts` declared `already_deploying` required while its two siblings are
  optional for a reason that applies to it identically.
- The comment above `applyingLabel` was two paragraphs making the same point, the
  first a superseded draft. On a file whose comments are treated as normative, a
  duplicated one is the next stale one.

## Declined

- **`pendingReports` runs on every store mutation, so once per progress line.**
  `Object.entries` over a store holding at most a handful of scenarios, against a
  derived-store construct to avoid it.
- **Two import statements from `deploy-store.js`.** Merged in passing while editing
  that file — not because it was a finding, but because it costs nothing.
- **`estateSummary` holds a numeric and a string form of "is anything applying".**
  Declined last round for the same reason: three uses of one variable, inside ten
  lines of one function, with no behaviour difference.
