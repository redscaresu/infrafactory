# Review pass 147 — S163e, thirteenth `/code-review` round

**12 findings, 8 accepted, 4 declined.** Two accepts are the previous two rounds'
fixes, each correct about the thing it fixed and wrong about its neighbour.

## A report is not an outcome, and neither is the other

Round twelve made the forget rule read `reports` so a successful retry could not
erase what a failed attempt leaked. It returned early when it found one — keeping
the **whole entry**, including the last outcome. So a failed-then-retried deploy
pinned "Deployed. It is listed on the Deployments page until its TTL expires." on
every later visit for the rest of the session, for infrastructure whose TTL may
long since have gone. That is the stale-success banner three rounds of hooks exist
to prevent, kept alive by the fix for losing reports.

`retireDeploy` separates them: the banner goes, the reports stay. The unit test
that pinned the accumulation asserted the reports and never looked at the outcome
sitting beside them.

## An alarm nobody can silence

Nothing could ever clear a report. `forgetDeploy` was unreachable for any entry
holding one, and there was no dismiss control — so an operator who read the project
id, removed the project by hand, and came back found the same red banner on every
page for the rest of the session. Two failed attempts pinned two.

An alarm that cannot be acknowledged trains readers past exactly the message this
arc says must never be lost. There is a button now, and it names which report: two
attempts can fail identically.

## A typo pinned a leak report for a scenario that never existed

Round eleven made the post-`Deploy` `os.ErrNotExist` branch deliberately NOT a
refusal, reasoning that an implementation could surface it after the apply. True —
and the real implementation resolves the name **before** it claims the lock, so
every mistyped scenario answered a plain 404, the client read "we do not know what
happened", and a red "it may have created resources that are still running" was
pinned for the session.

The deployer says which it means now: `ErrNoSuchScenario`, answered with
`writeRefusal`. A bare `os.ErrNotExist` keeps the cautious treatment, because a
state file vanishing mid-apply produces one too.

## The rest

- The `failed` arm of the in-flight banner still said the applying scenario "does
  not appear below" — rows are kept across a failed refresh on purpose, so the
  table can be showing an earlier deployment of that name. The loaded arm was
  rewritten for this two rounds ago; its twin was not.
- `loadDetail`'s new catch set `detail = null`, so a transient failure on the
  post-save reload unmounted the whole page — title, buttons, "Saved" status and
  the textarea holding the reader's YAML — after a PUT that had succeeded. It only
  clears `detail` when there was nothing there.
- The layout printed the scenario name twice, because the message had been
  prefixed before being stored and the banner adds a heading of its own. Attribution
  is a render step now: the layout has a heading and needs none, the page's slot has
  no heading and adds one.
- That render step anchors on `${scenario} ` **or** `${scenario}:`; a space-only
  anchor double-prefixes a message formatted `"web-app-paris: tofu apply failed"`,
  which is how the deploy pipeline's own errors read.
- `request()` — the helper every endpoint except deploy uses — still called
  `res.json()` unguarded, so a truncated body surfaced "Unexpected end of JSON
  input" as an estate banner or a scenario load error. The fix already existed three
  lines above.

## Declined

- **`deploymentsHandler` discards the in-flight list when `List()` fails.** The
  client throws away non-2xx bodies for this endpoint by design; returning the list
  there would be unread. The estate page already labels a kept list as stale, and on
  a first-load failure "we could not find out" is the honest answer.
- **`deployHandler`'s 405 branch is unreachable.** Its only caller invokes it under
  `if r.Method == http.MethodPost`. Defensive, uncalled, and no longer documented as
  a contract anywhere.
- **A nil deployments lister makes every confirmation warn "could not be fully
  read" while `/api/deployments` answers 501.** Not a configuration the CLI
  produces: `ui_command.go` sets `Deployments` unconditionally, and only
  `DeploymentActor` and `Deployer` are gated by flags.
- **A full estate walk per preview click.** Declined in round ten and recorded in
  STATUS and ADR-0027; noted again here so it stays a tracked cost.
