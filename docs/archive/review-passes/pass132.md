# Review pass 132 — S163 rework, fresh-context correctness lens

Five findings on the reworked slice. Four fixed, one recorded as a scoping gap.

## 1. A stage that FAILED was reported as `done`

`report.done()` ran before the error was inspected, so the last line a watcher saw
on a failed deploy was `init: done in 2s` followed by silence. **A failure
rendered as completion, and a stream that stops without saying why** — both of the
specific things this project holds itself against, in the code written to prevent
exactly that.

Worse on the retry path: attempt 1's failure was named correctly, and attempt 2's
was announced as `apply: done`.

`TestEveryStageReportsItself` **actively pinned the wrong behaviour** by asserting
`stage + ": done"` for every stage. It still does, for the passing case, which is
why `TestAFailedStageIsNotReportedAsDone` had to exist alongside it rather than
replace it.

## 2 and 3. In-flight state died with the component

Two findings, one cause. `resetDeployState` cleared the stream on navigation and
`onDestroy` cleared it on unmount, so:

- navigating away and back mid-deploy left a real, billable apply rendered as an
  **unlabelled disabled button with no log and no warning** — the amber "cannot
  see it" message was itself unreachable, because the panel it lives in was gated
  on state that had just been cleared;
- navigating to another *section* unmounted the component, so returning gave a
  fresh one that believed nothing was running and would start a **second deploy of
  the same scenario**. The server has no in-flight lock, so nothing else stops it.

My own comment asserted that `deploying` "is the only thing that stops a second
deploy" while sitting in a component that does not outlive the section.

The fix is a module-level store keyed by scenario, owning the socket. State that
must outlive the page cannot live in the page. Two e2e tests pin both journeys,
including a full unmount.

## 4. Deploy progress leaked into the Live Run page

The hub broadcasts to every client and `/live` appends every message and refetches
run state per event, so a deploy in one tab interleaved raw JSON into an unrelated
run's log. Filtered.

## 5. Recorded, not fixed: `run`/`test` still applies silently

`test_command.go` passes `nil`, because `executeTestWithScenario` has no writer in
scope — its progress goes through `runtime.Logger`, a structured API that needs an
adapter. So the Layer 3 apply is still silent on the Live Run page, which is the
**more commonly watched screen**.

That is the same defect this slice exists to fix, on a different path, and calling
it done would be the sort of claim the last two passes were about. Named here
rather than implied away.

## On the method

Codex was unavailable; three fresh-context agents reviewed instead. They found the
central defect in the first version — that the streaming did not stream — and then
found five more in the fix, including one I introduced while fixing another.

Worth recording honestly: the fix for finding 2 in the *previous* round created
finding 2 in this one. Removing state on navigation was right for the confirmation
dialog and wrong for the deploy stream, and I applied it to both because they had
been folded into one function two passes earlier.
