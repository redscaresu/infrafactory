# Codex review pass 130 — S163

One finding, accepted. **Eighth instance of S162c's class, in a new slice.**

## [P2] The deploy stream survived navigation

`onDestroy` does not fire when SvelteKit reuses the `[...path]` component across
scenario routes — which is every client-side move between scenarios. So the
websocket, `deployingScenario` and `deployProgress` all survived, and a previous
scenario's progress kept rendering under the new one.

**My own STATUS entry claimed the opposite**, in the same commit: *"the page
discards lines whose subject is not the deploy it is showing"*. It does that check
against `deployingScenario`, which still held the old scenario. The filter was
correct and the state it filtered against was stale.

## Why the fix is one function

The navigation reset was a **hand-written list** of things to clear, and S163 added
stream state without adding it to the list. That is not a new failure mode; it is
the enumerate-the-readers problem in miniature, on a list that lives twenty lines
from the state it is supposed to cover.

`resetDeployState()` is now the single place, called from both `afterNavigate` and
`onDestroy`. New deploy state has one obvious home.

## And the state that had no subject

The panel was gated on `deploying` — a bare boolean about no particular thing — so
it stayed open on a page with nothing to do with the deploy still running.

That is the same defect one level down from the finding: **state without a subject
cannot be filtered by subject.** The panel now asks `deployingScenario`, which
knows what it is about.
