# STATUS

Last updated: 2026-08-31

## 2026-09-03 — S163d: nine findings from `/code-review`

The review harness, now run on every PR, on the merged in-flight-lock slice.

**The worst was two 409s with incompatible bodies.** `writeActionResult` answers
409 for a deploy that *ran* and could not prove itself clean, carrying an
`ActionResult`; the "already deploying" refusal answered 409 carrying an error.
The client special-cases 409 as a result — so a refusal was parsed as one, found no
`clean` field, and rendered **"resources may still be running"** for a request that
never touched the cloud. The refusal is **423** now.

**The claim I had just corrected elsewhere was still wrong here.**
`deploy-store.js` said *"the server has no in-flight lock, so this is the only
thing standing between a reader and that second deploy"* — false as of the very PR
it sits in. Pass 134 was about that exact defect class, and I fixed the comment in
`live_service.go` and the ADR and left the one naming the missing lock *by name*.

**The lock's scope was overstated.** It prevents *concurrent* duplicates only;
deploy → wait → deploy again still makes a second project. A lock cannot tell "I
forgot" from "I meant it", so the preview reports `already_live` and the
confirmation says what exists. The ADR now says what it does and does not close.

**An adopted deploy could never finish** — no terminal websocket event, and only
the tab that issued the POST calls `finishDeploy` — so after a reload the button
stayed disabled for the rest of the session. It polls now, and an entry this tab
*owns* is never cleared by a listing that has not caught up.

Also: the stale-banner fix did not cover leaving the *section* (`afterNavigate`
does not fire on destroy — the case the store's own doc names); the `adopted` flag
was written and never read, so a reload mid-apply said "Starting…"; and
`adoptInFlight` had no test at all, its "leave owned entries alone" branch
deletable with everything still green.

**Recorded, not fixed:** every scenario-page mount fetches the whole estate to read
one string array.

## 2026-09-03 — S163c: the deploy guard moves to the server

Two deploys of one scenario mean **two run-owned projects and two sets of billable
resources for one thing**. Until now the only thing preventing that was
client-side state — and three review findings across two rounds were variants of
the same hole, each fix moving the client state around rather than noticing it was
client state. A refresh wipes it, a second tab never had it, `curl` never
consulted it.

`LiveDeployer` holds a per-scenario lock. A second deploy of one already in flight
returns `ErrDeployInProgress`, answered **409** and naming the scenario, because a
bare "conflict" leaves a reader wondering which of their tabs is responsible.

Per scenario rather than global — two different scenarios at once is ordinary.
Claimed *after* name resolution so a typo cannot lock a name nothing will deploy,
and released on every exit including failure: a scenario stuck marked-as-deploying
could never be deployed again without restarting the server, which is worse than
what the lock prevents.

`GET /api/deployments` now reports `deploying: []` so a reloaded page can restore
what it was showing. **Advisory only** — the refusal is the guard, and a client
that ignores the field still cannot start two. Always a list, never `null`: an
unconfigured deployer and an idle one are both "nothing is deploying" to a reader.

**Mutation testing disproved a claim rather than finding a bug.** Moving the
claim before name resolution passes every test, because the deferred release fires
on the resolution failure too — so the comment asserting "a typo cannot lock a name
nothing will ever deploy" was false. The lock is self-correcting either way; the
*release* is load-bearing and the ordering is tidiness. Both the comment and the
test's docstring now say so.

Also fixed: the test **deadlocked** under the no-lock mutation rather than failing,
because its fake runtime factory blocked unconditionally. A test that hangs on
regression is barely better than one that passes — CI reports a timeout, and nobody
reads a timeout as "the lock is gone". Only the first call blocks now, and the
mutation fails in half a second.

Also closed: a finished deploy's success banner used to reappear on every later
visit to that scenario for the rest of the session — a claim about infrastructure
whose TTL may long since have expired.

## 2026-09-02 — S163: streaming a deploy, and the first version that did not

A deploy runs for minutes, and **minutes of silence reads as broken**: a reader
cannot tell a long apply from a hung one, and the difference matters when the
thing running is creating billable infrastructure.

**The first version of this slice did not deliver that, and its tests passed
anyway.** It tee'd the deploy command's stderr — written twice *before* any cloud
work and once *after* the apply returns. Nothing in between touches that stream:
`CommandRunner.Run` returns a fully buffered `CommandResult`, so `tofu`'s output
does not exist until each process exits. Shipped behaviour would have been two
lines, silence for the whole apply, one line — **worse than before**, because a
terminal-styled log pane that has stopped moving is a stronger "hung" signal than
the button label beside it.

Codex was out of quota, so three fresh-context agents reviewed instead. Two found
it independently, and one *demonstrated* it by replacing the live tee with
buffer-then-dump and watching the suite stay green.

**The harness is the only thing that knows a stage has begun**, so it reports.
`SandboxDeployHarnessRunner.Run` takes an `io.Writer` — a parameter, not a field,
because the harness is shared and two deploys must not write into each other's
stream. Stage granularity rather than raw tofu output: it is what the plan asked
for, and it does not drown a terminal. **Retries are visible now**, because a
silent retry is indistinguishable from a slow stage.

`TestProgressIsVisibleWhileTheApplyIsStillRunning` asserts what a watcher sees
**from inside the apply**, which buffer-then-dump cannot satisfy — verified by
mutation.

Six further confirmed findings fixed, the worst being a regression the slice
itself introduced: clearing `deploying` on navigation re-enabled the button, so
navigating away mid-deploy and back allowed a **second real deploy** of the same
scenario — two projects, double billing. Also: an earlier deploy's completion
clobbering a later one's state; the `Deployed as dep-…` line losing a race with
the HTTP response; a dropped socket rendering as "Starting…"; the progress filter
having no test at all; and a `Close()` comment claiming a bug it does not fix.

The event subject is the **scenario**, not the deployment id — that id is minted
inside the command. Two concurrent deploys of one scenario share a stream. Stated
in the code rather than implied away.

**A second fresh-context review found five more, four of them in the fix.** A
stage that FAILED was reported as `done` — a failure rendered as completion,
followed by silence, in the code written to prevent exactly that; and
`TestEveryStageReportsItself` actively pinned the wrong behaviour.

The other three shared a cause: **in-flight state died with the component.**
Navigating away and back mid-deploy left a real, billable apply rendered as an
unlabelled disabled button with no log — the "cannot see it" warning was itself
unreachable, gated on state that had just been cleared. Leaving the *section*
unmounted the component entirely, so returning gave a fresh one that believed
nothing was running and would start a **second deploy of the same scenario**.

That last one was created by the previous round's fix: removing state on
navigation is right for the confirmation dialog and wrong for the deploy stream,
and I applied it to both because two passes earlier they had been folded into one
function. **State that must outlive the page cannot live in the page** — it is a
module-level store keyed by scenario now, owning the socket, with two e2e tests
pinning both journeys including a full unmount.

**A third review, adversarial on the tests, found nine more — every one verified
by mutation.** The shape of most of them: *the units were tested and the wiring
between them was not.* Dropping the tee, and passing `nil` to the harness at the
call site, each broke the feature completely while the whole Go suite stayed
green — because `deployStderr` was tested in isolation and every test calling
`Deploy` used a runtime that failed before the writer was touched.

One test closes all three of those: `LiveDeployer.Deploy` → `runDeployCommand` →
the real harness → sink → hub → what a websocket client receives, with only the
subprocess faked. **My first attempt at it skipped both broken points**, going
harness→sink directly — it looked like coverage and would have closed nothing.

Also: `retrying` was pinned as a substring, so the stream could promise a retry on
the final attempt *and on the interrupt path* — where the operator has asked us to
stop touching the API. It says `giving up` now. And the UI had **no test that
rendered a progress line at all**: replacing the whole `{#each}` with a constant
string passed the full suite. `page.routeWebSocket` lets the tests be the socket
server, so real frames reach the real DOM.

**Two recorded rather than fixed.** `forgetDeploy` has no production caller, so a
finished deploy's banner reappears on every later visit for the rest of the
session. And a **page reload defeats the in-flight guard** — the store is module
state with no server-side lock, so refreshing during a deploy re-enables the
button. That one wants a server-side in-flight lock, which is a design decision.

**Named rather than claimed done:** `run`/`test` still applies silently. Its
progress goes through `runtime.Logger` rather than a writer, so the Layer 3 apply
on the Live Run page — the more commonly watched screen — has the same defect this
slice fixes for `deploy`.

## 2026-09-02 — S162c: the deploy button, and a correction to S161

The scenario page can deploy. Deliberately a **separate button from Run**, and
deliberately not matched to it in colour or weight: S153 split the verbs so that
keeping infrastructure could not be reached by accident from the one that proves a
change is safe, and ADR-0027 §4 says the UI must not re-merge them.

The confirmation is S162a's preview rendered as **a list rather than a paragraph**,
because each line is a separate thing somebody might object to and a paragraph is a
thing people skim: what will be created, the cost with "list price" in it, expiry
as wall-clock, the image, and — separated out and styled differently — the
warnings that should actually stop somebody.

`modelled === false` leads the warnings, because it is the one that invalidates
everything above it: an unmodelled scenario's empty component list and €0.00 mean
*unknown*, not *nothing*.

### A correction: S161's visual-baseline claim was overstated

S161 said a visual baseline pinned the three health states being distinguishable,
and that "a regression making them look alike would pass every text assertion".
The first half is not true. `maxDiffPixelRatio: 0.02` means a change confined to
three small badges on a full-page screenshot passes comfortably — and **adding a
whole Deploy button to the scenario page did not fail its baseline either**, which
is what surfaced it.

The property is now asserted directly, which is both stricter and immune to
font-rendering flake: the three health badges must have pairwise-different
computed background colours, and an unconfirmed version must differ from a
confirmed one. A reader tells those apart by colour before they read a word.

## 2026-09-02 — S162b: the deploy endpoint

`POST /api/deployments` creates a live deployment, behind
`infrafactory ui --allow-deploy` — implied by neither `--allow-layer3` nor
`--allow-teardown`, because ADR-0027 says an ephemeral apply the run destroys,
destroying what exists, and creating what persists are three different kinds of
harm. `DeploymentDeployer` is a separate interface from `DeploymentActor` so the
separation lives in the type system rather than in a comment.

**The seam drives the real command rather than reimplementing it**, the way
`uiRunStarter` already drives `runRunCommand`. Between the request and the apply
sit a deny-by-default Layer 3 HCL preflight, a credentials check, a
per-deployment workdir so two deployments cannot share one state file, a
run-owned project created inside an interrupt guard, and a registration step that
writes the record **even when the apply fails** — because a half-finished apply
leaves real resources and the record is the only thing that brings the reaper back
to them. Every one is a guard, and every one is the kind of thing a second
implementation gets subtly wrong.

**A name, never a path.** `deploy` takes a filesystem path; accepting one over
HTTP would let a request name any YAML on the machine, including one the layers
have never seen. Resolution walks the scenarios tree matching the declared
`scenario:` field.

That resolver reads **only the name** — using `scenario.Load` would validate
against `DefaultSchemaPath`, which resolves relative to the working directory, so
in a server process every file would fail validation and be skipped and the API
would report "no scenario named X" for a scenario sitting right there. Silently.
Validation is deferred, not skipped: the command loads the path through the
runtime's own loader.

**The request has two fields, and the absences are the design.** No project — one
that could name a project could name somebody else's. No skip-validation. No
value meaning "forever". A deploy whose result cannot be read is reported as a
FAILURE, since the apply may well have created infrastructure and an empty result
would say the opposite.

## 2026-09-02 — S162a: what a person must be told before they spend money

ADR-0027 §2 made real: `GET /api/deployments/preview` answers what a deploy would
create, cost, expose and when it would expire. A GET, because it does nothing —
and available whether or not `--allow-deploy` was given, since knowing what a
deployment would cost is information rather than a capability, and a page that can
explain a greyed-out button beats one that silently omits it.

**The hard part was the cost, and the honest answer was to admit what it does not
know.** ADR-0024's figures were read off Scaleway's pricing pages by hand for *one
shape*. Any scenario declaring a database, a Kubernetes cluster or object storage
has components this project has never priced.

A total that quietly omitted them would be **wrong in the reassuring direction**,
and a confidently wrong number shown at the moment somebody decides to spend is
worse than no number. So `CostEstimate` carries `Priced` per component and
`Complete` overall: unpriced components are counted, named, and the summary says
*"AT LEAST … because N component(s) have no list price here"*. `Priced` is a
separate field from `EurPerHour == 0` because **free and unknown are different
facts**, and summing them as if both were zero is how a total understates itself.

An overridden compute offer is named and marked unpriced rather than billed as a
DEV1-S because that is the only number to hand. A non-Scaleway scenario is not
priced from Scaleway's list at all — it still says what will be created.

**Expiry is wall-clock**, not a duration: "4h" is a number people agree to without
doing the arithmetic; "expires Wed 3 Sep 03:47 UTC" is one they check against
whether they will still be awake. A TTL that will not parse makes the scenario
undeployable rather than defaulting, since guessing a lifetime is how a deployment
outlives everyone's memory of it.

A test pins the estimate against ADR-0024's own €0.042/hour, so the document and
the code cannot drift about what the demo costs.

## 2026-09-02 — S160c: the deploy safety model, decided before the button

ADR-0027. Mostly a record of what is already true — the origin guard, start-time
capability gates, a named confirmation — plus the decisions specific to
**creating** infrastructure rather than running against it or destroying it.

**`--allow-deploy` is its own flag, implied by neither of the others**, and both
would have been tempting shorthands. `--allow-layer3` permits an *ephemeral*
apply the run destroys before it finishes: minutes of blast radius, and the
account ends as it started. `--allow-deploy` permits infrastructure that is
**kept**, bills by the hour and serves the internet until something reaps it. An
operator who agreed to the first has not agreed to the second. `--allow-teardown`
is the opposite direction of harm and implies nothing about this at all.

Three flags gating three distinct kinds of harm is deliberate rather than untidy:
collapsing them into one `--unsafe` would make the cheapest carry the weight of
the most expensive, and people would enable it for the cheap reason.

The confirmation must state cost at **list price and say that it is an estimate**
— ADR-0024's figures are read off a pricing page, not measured, and a confidently
wrong number is worse than an admitted one. Expiry is shown as wall-clock, not a
duration, because "4h" is a number people agree to without arithmetic.

The default `infrafactory ui` can do none of the three.

## 2026-09-02 — S161b: teardown from the estate page

The estate page can now destroy what it lists, using S159b's endpoint. Three
things make that a deliberate act rather than a click:

**The control does not appear unless the server allows it.** The listing reports
`teardown_allowed`, so the page does not offer a button it knows will 404 — and
says how to enable it rather than silently omitting the capability. The *safety*
is still that the endpoint does not exist; a client setting the field locally gets
a button that 404s.

**The confirmation names what is about to go** — scenario, project, address. "Are
you sure?" is a speed bump people learn to click through; stating which project is
about to be deleted makes a misclick on the wrong row visible while it is still
reversible.

**A teardown that cannot prove the account clean is never shown as success.** The
409 carries the per-stage failures, and the shared `request` helper would have
thrown them away — leaving a generic error where *"the state file has vanished and
the resources may still be running"* belongs. `teardownOutcome` reads `clean`
rather than counting failures, because they are different claims and ADR-0024
turns on the difference.

The table reloads whatever the outcome: a failed teardown changes the record too,
since an unreclaimable deployment stays reapable rather than released, so the page
must not keep showing what was true before the attempt.

## 2026-09-02 — S159b: teardown and reap over the API

The mutating half of the deployments endpoint, and the seam turned out to be much
thinner than the plan expected: `tearDownDeployment` already took a context, a
runtime and a store, so `LiveActions` translates its staged output and does
nothing else. **None of the guards are restated at the seam** — teardown still
refuses unless a run-owned marker and the API's own provenance both say the
project is infrafactory's, because a second implementation of a guard is a second
thing that can be wrong.

**Deploy is deliberately not here.** Teardown and reap reduce cost; deploy
creates it, and the deploy button still needs its safety model.

**The capability does not exist without `--allow-teardown`.** Nil actor, 404
routes — not "exists and refuses", so a request cannot talk this server into
destroying infrastructure even if the origin guard has a bug. Same rule as
`--allow-layer3`, applied to the other direction of harm: teardown is not safe
merely because it removes rather than creates. It is irreversible.

404 rather than 501, because announcing "not implemented" advertises a capability
the operator declined. And if `--allow-teardown` IS given and the runtime cannot
be built, the server refuses to start rather than coming up silently without it.

A teardown that cannot prove the account clean answers **409**, not 200.

Both review findings were rules **already written in this codebase, next to the
code that follows them** — `withRuntimeNoGenerator` on requiring the LLM for
housekeeping, and `ensureRunProject` on not letting a caller cancel a change to
real infrastructure. That is four findings in two days of the same shape.

## 2026-09-02 — S161: the estate page

`/deployments` renders what is running, built on S159a's read endpoint. It reads;
it does not act — teardown and reap still need the seam S159b will build.

**The page has one job and it is not layout.** It is that *silence must not look
like health*, and a blank table cell is the most natural way in the world to
render "we do not know". So every state says a word: `never observed`,
`version unchecked`, `version NOT confirmed`, `never` for a last-observed time.

**The row that matters most is green.** A service answering perfectly while
running something other than the record claims is the most dangerous state this
system can reach, precisely because every other signal calls it healthy. If this
page rendered it as a quiet green row, nothing anywhere would flag it —
`needsAttention` marks it, and both a text test and a **visual** baseline pin it,
because a regression that made the three states look alike would pass every text
assertion.

Two failures reported rather than hidden: records the store could not decode
(they may describe running, billing infrastructure — `live ls` exits non-zero,
a page has to show it), and a failed read, which keeps the previous rows and says
so. An empty table would read as "nothing is running", and a failed refresh is
not evidence the estate is empty.

The summary states what was examined, not only what is wrong — the same reason
`live reconcile` does.

## 2026-09-01 — S159a: the live estate, read-only, over the API

S159 as planned bundles a read path, five mutating endpoints and a seam
extraction. Split, applying S156d's lesson: the read path carries no guards and
the mutating ones carry all of them, so shipping the read first lets the estate
page be built against something real without any of those guards being
reimplemented in a hurry.

`GET /api/deployments` serves the estate. It refuses every mutating method, and
says so when no live store is configured rather than answering with an empty list
that reads as "nothing is running" — the same rule `live reconcile` applies.

**The health summary moved onto the record.** `live ls` and this listing must
agree about what silence means, and two implementations of "the last observation,
unless there are none" is how one of them eventually renders `unobserved` as
healthy — silently, and only in the UI, where it is least likely to be noticed.
`healthLabel` now defers to `Deployment.Health()`.

**`unchecked` is spelled out, and finding that was the point of the slice.**
`VersionUnchecked` is the empty string on the stored record, which is right there
— `omitempty` keeps the file terse. In a *view* it is a falsehood: a UI rendering
`""` shows a blank cell beside a `confirmed` one and invites the reader to read
nothing-was-checked as nothing-is-wrong. The UI arc plan warns about exactly this
and the zero value would have delivered it. `HealthSummary.Version` is a string
that always says the word.

**The same defect turned up three times, and the third fix is the one that
matters.** `omitempty` does nothing on a `time.Time` — it is a struct, so the tag
has nothing to act on — and `VersionUnchecked` is the empty string. So the
version label would have rendered blank, the observation timestamp as
`0001-01-01`, and the embedded record's upgrade timestamps the same. Pass 99's
own note says that *checking the field I was thinking about and not its
neighbours* caused it; pass 100 was me doing that again. It is closed as a class
now: a test marshals a deployment with every optional field unset and asserts
`0001-01-01` appears nowhere, so the next optional time inherits a failing test
rather than the defect.

Records the store could not decode travel in the payload. `live ls` exits
non-zero for those; a GET cannot, so the count has to reach the page.

## 2026-09-01 — S156d: prescriptive rules from upgrade diffs

S156c writes descriptive rules only — what was observed, nothing about what to do.
ADR-0019 calls that the weakest useful class, and it is all a single live failure
can support: there is no "next iteration that fixed it" to diff against.

An upgrade supplies one, which is why S155b stashes `.infrafactory-previous/`.

**The planned risk was real, and the resolution was to change what does the
discriminating.** An upgrade diffs two configurations that *both applied
successfully*, so the apply cannot tell a fix from a routine version bump. The
observations can: `livestore.Repairs` promotes an upgrade only when the service
was observed failing **before** and healthy **after** — precisely "this failed,
then this passed", measured against the running service rather than terraform,
which reported success both times.

Healthy-but-on-the-wrong-version does not count as "after": a service answering
fine while running something other than what was deployed is close to evidence
the upgrade never took. One healthy probe does not count either.

**Attribution turned out to be the hard part, and the first answer was wrong.**
A run-loop failure names a resource address; a live probe says `health path
http://… returned HTTP 503` and names nothing. `AddressResource` looked like the
substitute and is not — it is where the probe *pointed* (a load balancer IP), not
where the fault was (a backend block), so narrowing the diff to its type skips
exactly the upgrades worth learning from. Review caught it, and caught that the
test only passed because it seeded a synthetic detail containing a Terraform
address. Attribution now comes from the diff: use the change only when **exactly
one** resource differs.

**Eight findings across five review passes, and every one the same shape**: a rule
the surrounding code already enforced that the new path did not inherit — the
ambiguity gate (deletions uncounted), per-cloud handling, failure-must-persist,
attribution-from-detail, "an absent input is not an empty input", and the
apply-window boundary. S156a's lesson was that a change lands in every place that
reads it; this is its mirror, and the sharper one, because none of those rules are
written where a new path can see them.

The worst was subtle: `upgraded_at` is written *after* the apply returns, so a
concurrent `live observe` — which S158's journey test performs — records the
upgrade's **own downtime** as the failure being repaired. Records now carry
`upgrade_started_at`, probes inside the window are discarded rather than assigned
to either side, and a record without the boundary is declined rather than guessed
at.

Remedies stay `source: live`, not `fix`. `fix` is permanent; `live` is retirable.
A rule extracted from a running deployment is prescriptive in shape and live in
provenance, and what is true of a running service stops being true — tagging it
`fix` would make it immortal and break S156c's rule that nothing is written which
cannot later be retired. ADR-0019 amended: the source tag is about provenance and
retention, the rule text is about shape, and they are separate axes.

## 2026-09-01 — S157a: reconcile against the API

ADR-0024 called this "the largest remaining hole" in it, and the hole was worse
than "not built yet". `livestore.go` stated **as fact** that the reaper
"reconciles against the API rather than trusting this file alone." Nothing did:
`live reap` calls `store.Reapable()` and never contacts Scaleway. A comment
asserting a guard that does not exist is worse than no comment, because it makes
the hole invisible to the next person reading for it.

The failure is quiet and expensive. `.infrafactory/live` sits **inside** the
working directory — wipe it, switch branches, or run from a fresh clone, and the
records are gone while the load balancer, the instance and the public IPv4s keep
running with a TTL nobody will ever enforce. Every signal reports clean, because
every signal reads the store. That is the shape of D6.

`live reconcile` reports both directions and **never destroys anything**: an
unrecorded project is by definition something the records cannot explain, and
destroying what you cannot explain is how a reconciler becomes the incident.

It is precise rather than heuristic because of ADR-0025's stamp, using the *same*
`IsInfrafactoryRunProject` predicate that guards teardown — one definition of
ownership rather than one here and another in the guard. Unstamped projects are
never considered in either direction.

**Three refusals, all the same failure mode.** Missing credentials, an
unreachable API, and a >50-page estate each render as "nothing unaccounted for"
if handled the easy way — the false green S139 exists to prevent. All three fail
instead.

**Verified against real Scaleway, not a fake.** An empty stamped project was
created, left unregistered, confirmed in `List`, confirmed flagged by `Reconcile`
against an empty store, and deleted. Cost: nothing, since projects are free.
The baseline is worth recording — **3 projects in the organization, 0 stamped**,
so the account holds no leaked run projects and the D6 fix is holding.

## 2026-09-01 — S160b: real cloud is decided at start time, not on the wire

S160a stopped a page the server did not serve from reaching the API. This closes
the separate idea that a **request** can escalate the server into spending money.
Different properties: the second survives a bug in the first.

`layer3_enabled` is gone from `StartRunRequest`. Real-cloud apply for UI-started
runs is settled by `infrafactory ui --allow-layer3`, read once at startup — in
the shell that already holds the credentials, by the person who typed it.

**The config file does not get a vote either, and that half had a live failure
mode.** The per-run config is re-read from disk on every run, which is what lets
an operator edit `infrafactory.yaml` without restarting. So a file saying
`sandbox_deploy.enabled: true` would have walked real-cloud apply back in on a
server started *without* the flag — silently, on every run after it. The seam
that re-applies the server's decision is now `uiRunStarter.configLoader`,
extracted so a test can hold it rather than being a closure buried in the middle
of starting a run.

The scenario page's Layer 3 checkbox becomes a report of what the server decided,
plus how to change it. `config_default_enabled` was renamed `server_allows_layer3`
— the old name described a *default* a client could override, and no client can.

`run.json` keeps its own `layer3_enabled`: that is a record of what a run **did**,
not a request for what it should do. Same word, two different things, and only the
request side is removed.

ADR-0026 amended.

## 2026-09-01 — S160a: the UI API answers only loopback origins

The UI arc plan schedules its safety model as *"decided before the capability
exists, not after."* **The capability already existed.** It is spelled
`layer3_enabled`: `POST /api/runs/<scenario>/start` accepts that field and
`ui_command.go` uses it to set `SandboxDeploy.Enabled`, so the request starts a
real Scaleway apply — with credentials from the process environment, which holds
live keys whenever the server was started from a shell that sourced `layer3.env`.

The websocket handler checked `Origin`. The POST handlers checked nothing, and
decode the body without inspecting `Content-Type`, so a cross-origin `fetch` with
`text/plain` is a CORS *simple* request that never preflights. The attacker
cannot read the response and does not need to: the side effect is the goal.

**Loopback is a boundary against processes, not against pages.**

The guard wraps the mux rather than the endpoints that mutate — a list of today's
mutating handlers is a snapshot, and the next one is written by someone who never
saw it. Every method, not only the unsafe ones, so the guard does not rest on "no
GET handler ever mutates", which nothing enforces.

**The rule is loopback, not same-origin, and the weaker rule announced itself.**
Comparing `Origin` to the request's own `Host` looks stricter and is defeated by
DNS rebinding: a domain that flips to `127.0.0.1` sends an Origin and a Host that
*agree*, so equality passes them. It also broke three existing websocket tests,
because the Vite dev server serves the UI on `:5173` and calls the API on `:4173`
— cross-origin and entirely loopback. One rule handles rebinding, the drive-by,
and development.

Recorded in ADR-0026. This is one layer and not the safety model: S160b removes
the wire field so a *request* cannot escalate the server into spending money,
which is a different property that survives a bug in this one.

## Current phase

- 🔄 **S156c: the loop closes — reproduced observations become pitfalls
  (2026-09-01)** — `live learn` writes promoted candidates into the corpus as
  `source: live`, and the generator sees them on the next run.

  **The hard part was not writing; it was attribution.** The corpus is keyed by
  resource and the generator steers on it — but a live observation names none:
  *"the thing at this address returned 503"* is about an endpoint.
  `ExtractDescriptivePitfall` already refuses to invent a resource
  (*"skip rather than fabricate"*), and inventing one here would have been the
  same mistake with a new name.

  The honest answer is **the resource the address was resolved from**.
  `LiveEndpointResource` now reports it alongside the address, and `deploy`
  records it — at the only moment it is a fact rather than an inference.

  Four refusals, each preferring silence to a guess:

  - **No resource, nothing written** — and the operator is told, so the corpus
    does not look complete when it is not.
  - **Deployments that disagree on the resource write nothing.** If one served
    from an `scaleway_lb_ip` and another from an `scaleway_instance_ip`, the
    failure they share is a fact about neither.
  - **Deployments that disagree on the cloud write nothing**, because the corpus
    is per-cloud and either choice files half the evidence where it does not apply.
  - **No remedy is invented.** A descriptive rule states what was seen and stops;
    the prescriptive form comes from an upgrade diff in S156d.

  **Pass 76 caught the slice's main behaviour being defeated by reuse.**
  `AppendPitfall` dedupes on *word overlap* — right for provider diagnostics that
  vary in phrasing, wrong for live rules generated from a template, where two
  genuinely different failures on one resource share nearly every word. The second
  lesson was dropped as a duplicate, silently. Live entries append on **exact
  identity** now, which is sound precisely because the text is derived
  deterministically from the candidate.

  Third time in this arc that reusing an existing mechanism was the mistake rather
  than the shortcut. **A mechanism's semantics travel with it.**

  **Pass 77 caught pass 76's consequence.** Exact-text identity is unstable,
  because a live rule *states its evidence* and that evidence grows — so the
  corpus gained an entry every time the counters ticked, while appearing to
  refresh. `ObservedKey` now holds the normalized detail the gate grouped by:
  identity separated from presentation, so a refresh rewrites the text and the
  corpus carries the **strongest** evidence rather than the first written.

  Then the same bug one layer out, found by **enumerating the readers before the
  next review** rather than after: `pitfall-merge` keyed live entries on rule text
  too, so a sweep would have duplicated every refreshed lesson.

  **Pass 85 then caught the other half of the same identity question.** The key
  was the normalized detail, but the gate groups on *(status, drift, detail)* —
  `unhealthy` apart from `unreachable`, a health failure apart from a version
  mismatch. Persisting anything narrower let one reproduced failure overwrite
  another. `Candidate.Key()` now lives beside the gate and is what gets
  persisted: derived rather than reassembled, so the two identities cannot drift.

  **Pass 86 found the grouping wrong one dimension further out.** The gate ran
  over every deployment and filtered cross-cloud candidates afterwards, which
  both discarded sufficient single-cloud evidence and counted breadth *across*
  clouds — one observation apiece promoting a coincidence of wording. The corpus
  is per-cloud, so the partition belongs before the gate; the old test had
  encoded the bug by relying on cross-cloud breadth to promote. Same pass: the
  first live write could not create its own corpus, because `MkdirAll` was an
  obligation on callers rather than on the writer that needs the directory.

  **Converged on pass 87.** Three of the four passes were one question at
  widening scope — *what is the identity of a live lesson?* — and each answer was
  correct and incomplete in a way visible from the one before. What ended it was
  not a better answer but a structural move: stop restating the identity at the
  persistence layer and derive it from the gate that defines it.

  **Every entry is stamped**, and that is a hard coupling rather than a nicety:
  S156a never retires an entry without `last_seen`, so an unstamped live entry
  would be **immortal** — the inflow quietly undoing the slice built to bound it.
  Learning the same lesson twice refreshes rather than duplicates.

- 🎯 **S156b: the promotion gate — when is an observation a lesson? (2026-09-01)**
  — `live candidates` reports observations that have **reproduced**, and says
  what promoted each one and why.

  Reproduced means either **N consecutive probes on one deployment** (persistent,
  not a blip) or **≥2 distinct deployments** (a property of the shape, not of one
  machine). A single 502 never becomes a pitfall.

  It deliberately produces **candidates, not pitfalls**. Turning one into a rule
  is S156c, and keeping them apart means the gate can be judged on whether it
  promotes the right things without also arguing about rule text — the harder and
  far more subjective half.

  Four refusals, each a way the gate could be wrong rather than merely incomplete:

  - **A recovery breaks the run.** A healthy probe between two 503s means the
    service recovered — precisely the blip this exists to reject. Four failures
    with a recovery in the middle promote nothing.
  - **A different failure breaks it too**, because only one thing can be true of
    a service at a given probe.
  - **`unhealthy` and `unreachable` never merge.** One of each is not two of
    either.
  - **A zero rule promotes nothing.** A misconfigured gate that opens is worse
    than one that closes.

  **Attribution travels with the candidate rather than filtering it.** Something
  was broken either way, but a lesson blamed on a version nobody verified is a
  falsehood (S155a) — so an unattributable candidate is reported as
  *"version UNCONFIRMED, so nothing may be blamed on a tag"* and the extractor
  decides.

  Released deployments still count: their observations happened while they were
  live, and dropping them would discard exactly the reproduced evidence the gate
  looks for.

  **Pass 73 caught the gate excluding the signal the arc exists for.** It skipped
  anything `Healthy()` — and a service answering perfectly while running the wrong
  version records `Status: healthy`, so **version drift could never be promoted**.
  Checking it also turned up something the finding did not say: the mismatch was
  never on the record at all, only in the failure summary, so the reason vanished
  when the command exited. Both fixed — the record carries it, and `adverse()`
  covers it. The report names it `version drift (service healthy)`, because
  "healthy" alone reads as the opposite of a finding.

- 🔁 **S158: the live lifecycle has a journey test (2026-09-01)** — deploy → ls →
  observe → upgrade (with an observation landing *during* the apply) → observe →
  teardown → ls → observe, through the **real commands and the real
  `livestore`**, with only the cloud faked.

  **The plan's premise was wrong and is corrected in place.** It said the test
  would drive the commands against mockway. It cannot:
  `assertRealScalewayEndpoint` refuses any Layer 3 apply not pointed at
  `api.scaleway.com` — checking the passed env, the inherited `SCW_API_URL` *and*
  the scw config file. `deploy`, `upgrade` and `teardown` all inherit that
  refusal. Pointing the test at a mock would mean weakening ADR-0023 containment,
  so the cloud is faked instead and everything touching the record is real.

  That turns out to be the better test anyway: all three defects this slice
  exists to catch are **record-coupling**, and none needs a cloud.

  **Verified by reverting the fixes.** Undo pass 57 and the journey fails with
  *"the observation recorded during the apply must survive the upgrade's write"*;
  undo pass 44 and it fails with *"a live deployment nobody can monitor is a
  finding, not a skip"*. A journey test nobody has seen fail is a journey test
  nobody should trust.

  One real transient noted rather than papered over: an `observe` landing after
  the service has moved but before the upgrade writes the new tag sees record and
  world genuinely disagree, and reports `unconfirmed`. Correct, and exactly the
  single-blip noise **S156b's reproduction gate** exists to filter.

- 🔗 **Doc links are now ratcheted (2026-09-01)** — `STATUS.md` lives at the repo
  **root** while what it points at lives under `docs/`, so `](plans/x.md)` looks
  right and resolves to nothing. **All four** of its relative links were broken;
  a reviewer caught one and I had written three of the others the same session.
  `TestDocLinksResolve` now walks `docs/` plus the root documents, verified
  against synthetic drift, and refuses to pass if it finds fewer than two files —
  because pass 34 showed an audit that narrows its own scope reads as coverage
  while the defect lands elsewhere.

- 📋 **S158 planned: the live lifecycle has no end-to-end test (2026-09-01)** —
  `internal/e2e/` holds twelve files and none mentions `deploy`, `live observe`,
  `live upgrade` or `livestore`. Every S151–S156a guarantee is unit tests plus a
  real-cloud run driven by hand, one command at a time. **Nothing re-runs the
  sequence.**

  The live commands are coupled through a **record**, not through function calls,
  and each command's unit tests build their own. That is exactly the shape that
  hides defects, and this session produced three: `observe` failing on a record
  `deploy` legitimately wrote without an address; `upgrade` discarding
  observations appended during its apply; the record and marker disagreeing about
  which project an apply may touch. **No unit test could have caught any of
  them.**

  Plan: [docs/plans/live-lifecycle-e2e-plan.md](docs/plans/live-lifecycle-e2e-plan.md).
  One test driving the real commands in sequence against mockway, plus one real
  pass — the mock proves the commands agree with each other, only Scaleway proves
  they agree with the world.

  **The UI arc was refreshed to match.** S164 held the only journey coverage, at
  the end of an arc that has not started, so it moves out to S158 and keeps its
  Playwright half. And S161's estate page predates observation: it must render
  `unobserved` and `unchecked` **distinctly**, because a blank cell rebuilds the
  falsehood the three-state design exists to prevent — that nobody having looked
  is the same as nothing being wrong.

- 🗂️ **S156 broken into five slices, and its unscheduled prerequisite scheduled
  (2026-09-01)** — `docs/plans/live-learning-loop-plan.md`. The plan already
  warned that **S156 must not merge without a retirement path for `source: live`
  pitfalls**, and nothing was scheduled to build one. It is now **S156a**, and it
  is the only one of the five with no dependency — so it was the slice to build
  while S155b waited on its canary. **Both have since shipped** (below).

  Doing the outflow before the inflow is deliberate: every other slice *adds* to
  the corpus. A pitfall that steers generation away from something no longer
  broken makes every future generation worse, **silently**, which is the failure
  mode this project has learned to fear most.

  **Why five and not one**: S155b took seven codex passes and thirteen findings,
  ten of which were one incomplete answer spread across six interacting invariants
  in a single command. S154, S170 and S155a — each smaller — converged in three.
  **Slice size was the lever, not review effort**, and S156 is bigger than S155b
  was.

- 🧹 **S156a: the corpus gets an outflow (2026-09-01)** — `pitfalls retire`
  removes `source: live` entries that have not been seen within a retention
  window, and **names every one it removed**.

  Built before anything produces a live entry, deliberately. Every other S156
  slice *adds* to the corpus; this is the only thing that can take entries out,
  and building the inflow first is how a corpus becomes something nobody dares
  prune. The plan flagged it as a hard prerequisite and nothing was scheduled for
  it.

  Why live entries decay and the others do not: learning used to be **bounded** —
  a run emits at most `repair_iterations_max` failures, and a scenario that stops
  failing stops emitting. Live observation removed that bound. And a stale pitfall
  is not inert: it steers generation away from something no longer broken, making
  every future generation worse **silently**.

  Three refusals, each guarding against deleting learning on weak evidence:

  - **No timestamp is never retired.** Absence means nobody recorded when the rule
    was last true — not evidence that it stopped being true. An unparseable value
    counts as absent too, because zero would make every malformed entry infinitely
    stale.
  - **Retirement is exclusive at the boundary**, so a 14-day threshold does not
    delete something last seen 14 days ago to the second.
  - **A non-positive window is refused on both entry points** — a mistyped flag
    would otherwise empty the corpus in one command, and a dry-run that accepts
    what the real run rejects teaches the wrong thing about what is safe to type.

  **Pass 59**: `live` was added as a persisted value while two ratchets still
  fenced the corpus to `descriptive`/`fix`/`avoid`, so the first live entry would
  have failed CI with the blame pointing at the entry rather than the ratchet.
  Both updated. Sweeping for the same class then found what the finding did not:
  `pitfall-merge` preserves `--keep avoid` across a sweep, so **a sweep would have
  silently deleted every live entry** — the exact thing this slice exists to
  prevent. Now `avoid,live`.

  **Pass 60** then caught the incomplete half of that: preserving live entries
  through a sweep preserved the *entries* but not their *freshness*, so a rule
  re-observed during a sweep kept the older timestamp — retention silently
  reverting to *first observed* and retiring a rule that was still true.
  `pitfall-merge` carries forward a newer `last_seen` now and reports
  `refreshed=N`. **Preserving a record is not the same as preserving what the
  record says.**

  **Pass 61**: the command was wired through the full runtime, which builds the
  Claude transport — so corpus maintenance would fail because the *agent* was
  unconfigured, on exactly the machine doing housekeeping.
  `withRuntimeNoGenerator` keeps the logging and substitutes a generator that
  **refuses**, since this command must never generate.

  **Pass 62**: the `<cloud>` argument went into `filepath.Join` unvalidated, so
  `retire ../../x` would have **rewritten** a file outside the corpus.
  `livestore.validateID` already guards deployment ids against exactly that;
  `assertCloudName` now does the same on all three entry points.

  Across S156a's four passes, **three findings were mechanisms that already
  existed in this repository** — the source ratchets, the sweep's preservation
  rule, the path guard. Slice size is not the whole story: the other half is that
  new code touching an existing mechanism should start by finding every place that
  mechanism is already enforced. Adding `LiveSource` was three lines; the work was
  the four places that had to learn about it.

  **Pass 63**: the refresh crossed source boundaries. `entryKey` ignores `source`,
  so a live timestamp could land on a `descriptive` entry — attaching a lifetime
  to a source that has none, while the live record vanished as a duplicate. Now
  like-with-like.

  Passes 59, 60 and 63 all landed on the same **fifteen lines** of merge code,
  each fix correct and each too narrow. What S156a actually teaches is not about
  slice size, since this slice is small: **a change to a data model lands in every
  place that reads it**, and the reliable way to find those is to enumerate the
  readers before the first edit rather than after each review.

  **Pass 64**: the sweep parses `kept_new` into `AVOID_EMISSIONS`, a ratchet on
  whether the avoid extractor still works — so preserving live entries could have
  made a sweep with **zero** avoid pitfalls report a healthy number. `pitfall-merge`
  prints per-source counts now and the sweep ratchets on `kept_avoid`.

  Five passes, five findings, all downstream of adding **one constant**. The
  places that had to learn about `live`: two corpus ratchets, the sweep's
  preservation list, the merge's freshness handling, the merge's source
  semantics, the metric reading the merge, and the new command's path guard.

  **Pass 66 reversed pass 63.** Skipping a source-mismatched duplicate preserved
  the old dedup and silently dropped the *live* entry whenever an older
  `descriptive` rule shared its `(resource, rule)` — losing the `last_seen` that
  retirement runs on. `mergeKey` includes the source for `live` only: dropping a
  duplicate `avoid` loses a rule the corpus already states in other words;
  dropping a duplicate `live` loses information nothing can rebuild.

  **Pass 65 was declined** — the first decline in the slice. The sweep omits
  genesys from its pitfall handling, and so do all three ratchets, consistently
  and since long before this arc; `LiveSource` neither created that gap nor widens
  it. Bringing genesys in means deciding whether it joins the ratchets and what
  `AVOID_EMISSIONS` means for a cloud with no avoid entries — its own scope, and
  folding it in is how a small slice becomes S155b. Recorded in the plan as a
  prerequisite for **S156c**, the first slice that will actually produce live
  entries.

  `--dry-run` and the real thing share one rule by construction: a dry-run that
  can disagree with the real thing is worse than no dry-run. And `TouchLivePitfall`
  refreshes rather than appends, which is what makes retention mean *last
  observed* instead of *first observed*.

- ✅ **S155b canary: the thesis, demonstrated on real infrastructure (2026-09-01)** —
  deployed v1, confirmed the service was serving `nginx:1.27`, upgraded to 1.28,
  and **`sandbox_deploy/apply` passed while `upgrade_verify` failed.** Confirmed
  by hand: `curl` returned `nginx/1.27.0` while the workdir held the 1.28 config.

  The mechanism is mundane, which is the point — changing `user_data` on a running
  Scaleway instance does not re-run cloud-init. Terraform updated the resource,
  reported success, and the machine kept serving what it already served.

  **That upgrade was green everywhere else**: the apply passed, the sweep would
  have reported the account clean because it was, a cost check would have found
  nothing because there was nothing. Only the check that asked *the service*
  disagreed. Same shape as D6, same lesson.

  It also produced, by accident, exactly what S156 wants: a **failed upgrade with
  both configurations preserved** — the before/after pair `ExtractFixPitfall`
  needs, from a real failure rather than a constructed one. Full record:
  [docs/status/s155b-upgrade-canary.md](docs/status/s155b-upgrade-canary.md).

- 🚀 **S155b: `live upgrade` — an apply is not an upgrade (2026-08-31)** —
  rolls a live deployment onto new configuration **in place**: same project, same
  workdir, so the load balancer and its address survive.

  **infrafactory does not invent the new HCL.** It owns the part that is hard to
  get right — applying it into the project the deployment already owns, under the
  same deny-by-default checks a first deploy runs, and proving afterwards that the
  version actually changed. That composes with however the HCL was produced.

  **Deliberately not parameterised through `TF_VAR`.** `SandboxStripEnv` removes
  `TF_VAR_*` because the cost bounds read a variable's *default* to decide blast
  radius, so an injected variable would make those checks vouch for a number that
  never reaches the API. Handing over whole HCL keeps every existing check
  applying to the configuration that is actually applied.

  Three properties worth naming:

  - **A successful apply is not an upgrade.** Terraform reaching its desired state
    does not mean the service is running the new version — the instance may not
    have restarted, the image may not have been pulled, the user data may never
    have run. `upgrade_verify` fails on exactly that, and it is the whole point.
  - **It refuses to start from a version the service contradicts**, because that
    would record a v1→v2 transition that never happened. Unchecked is allowed and
    said out loud; contradicted is not.
  Pass 50 corrected two of them. The tag advanced even when the apply never ran —
  right for a failure *during* apply, wrong at init or plan where nothing reached
  the cloud, and there it made the record claim a version that was never deployed.
  And the address could go stale: replacement HCL can recreate the load balancer,
  so verification would probe infrastructure the deployment no longer owns. It is
  re-read after the apply now, reported when it moves, and when it cannot be
  re-read the old one is kept **and said out loud**.

  Pass 51 found two more, both data-safety: `--from` naming the deployment's own
  workdir **emptied it** (the superseded files are removed before the new ones are
  read), and a rejected configuration was left in place after an init or plan
  failure, so every later operation would plan against something never applied.
  Both now hang off the same `applyRan` predicate as the tag — *did anything reach
  the cloud?* — which is the real fix: one question answered consistently rather
  than three patches.

  Pass 52 closed it: `upgrade` never checked the `sandbox_deploy.enabled` opt-in,
  so Layer 3 could be off and an upgrade would still apply to the real project —
  a gate on one entry point and not the other guards nothing. And an environment
  failure could still leave unapplied configuration behind, which was **fixed by
  reordering rather than another rollback**: everything that can fail without
  touching the workdir now happens before the part that does. Ordering beats
  compensating — one rollback path instead of one per early return.

  Pass 53: the project id came from the **record** rather than the marker — the
  half a stale file can change — on a call that applies real infrastructure.
  `live teardown` learned that in pass 37 and I did not carry it into new code;
  a disagreement is refused now. **Applying deserves at least the care destroying
  gets.** Also, with no `--tag` the verification compared the *old* tag and
  trivially passed, reporting a transition that was never attempted — a green
  built from checking that nothing changed.

  Pass 54 finished it: the marker was *preferred*, not required, so an unreadable
  one fell back to the editable record — a guard that degrades to the thing it was
  guarding against. Required now, and the contrast with `live teardown` (which
  does fall back, deliberately) is written down: refusing there strands real
  pre-cutover resources and destroy is bounded by its own state; neither holds
  when applying.

  Pass 55 finished the second thread: the marker and the record are **both local
  files**, so trusting either only proves two local files agree. ADR-0025 never
  said "use the marker" — it said *two checks that must both pass*, the marker for
  identity and **API provenance** for class, because the second cannot be forged
  locally. Editing two files was enough to point a real apply at a production
  project. The API is asked now, and deliberately not through the deletion guard,
  which treats "gone" as success.

  Pass 56: a prefix match confirmed tag `1.2` against a service reporting
  `nginx/1.27.4` — a **false confirmation**, the exact drift S155a exists to
  catch. `mentionsVersion` requires version boundaries now. And the upgrade could
  overwrite a concurrent teardown: the same race fixed in `observe`, worse here
  because an upgrade holds its record across a real apply — minutes, not the
  microseconds a probe takes.

  Pass 57: the pass-56 re-read was used for the released check and then
  **discarded**, writing back the stale copy — so observations `live observe`
  appended during a minutes-long apply were silently dropped. Those are the input
  S156's promotion gate counts, so it weakens the learning signal rather than
  breaking anything visibly. Now an allow-list merge of the three fields an
  upgrade owns onto the fresh record.

  **The shape of this slice**: eight passes, fourteen findings, and ten of them are
  one incomplete answer rather than eleven mistakes — "did anything reach the
  cloud?" answered four times, "which project do we trust?" three, and three
  separate fixes that **already existed elsewhere in the codebase** and were not
  carried to a new path. The lesson is not "review harder": new code touching an
  existing mechanism should start by reading how that mechanism is already used,
  rather than reimplementing the parts of it that seem needed.

  - **The previous HCL is kept** in `.infrafactory-previous/`. That pair either
    side of one change is the diff `ExtractFixPitfall` needs and cannot
    reconstruct — it is what lets S156 produce prescriptive rules rather than the
    weakest class of lesson.

- 🔍 **S155a: the record states intent; only the service states fact (2026-08-31)** —
  `deploy` records the **declared** image and tag, and the canary showed how far
  that drifts: the record said `nginx:1.27` while the instance served
  `python3 -m http.server`. An upgrade to a version nobody confirmed is running
  proves nothing, so this is S155's prerequisite rather than a nicety.

  A scenario may now declare `service.version_path`. `live observe` probes it
  separately from health and records one of **three** states — deliberately not
  two. `unchecked` (nothing was asked) and `unconfirmed` (we asked and the
  service did not confirm) are different facts, and treating the first as
  confirmation is the falsehood this exists to stop. A probe that fails is
  `unchecked`, never `unconfirmed`: claiming a contradiction on evidence nobody
  gathered is the same error in the other direction.

  The check is **deliberately weak and deliberately stated**: the response must
  *mention* the tag. That verifies a cooperating service and cannot verify an
  uncooperative one, which is why declaring the path is opt-in. A mismatch fails
  even when the service is perfectly healthy — that is the more dangerous case,
  because it looks fine.

  **Pass 47 caught the slice violating its own doctrine**: a truncated or
  unreadable body was compared anyway, so a partial response could be called a
  mismatch — claiming a contradiction on evidence nobody fully gathered, which is
  the same error as treating unchecked as confirmed. The asymmetry is explicit
  now: *finding* the tag proves it is there whatever was cut off; *not* finding it
  in a partial body proves nothing. Every `unchecked` also carries its reason, so
  a declared-but-unreachable path no longer prints "no version_path declared".

  Pass 48 then caught a regression pass 47 had introduced: reading the body head
  meant editing the `defer` beside it, and the response drain lost its bound. It
  would have shown up as slowness rather than a hang — `live observe` probes
  every deployment in turn, so one streaming body delays all the ones behind it.
  **An incidental edit inside a fix is still an edit**, and under a one-clean-pass
  rule those are the ones that survive, because attention is on the finding.

  **Verified against real Scaleway** by deploying a service that contradicts its
  own record: `the record claims nginx:1.27 but / does not mention "1.27"`. Along
  the way it also confirmed pass 44's fix on real infrastructure — a deployment
  with no load balancer produced no address, and `observe` refused it rather than
  reporting clean.

- 🔬 **The NIC blocker retested — it moved, and narrowed (2026-08-31)** — the
  claim that `scaleway_instance_private_nic` is refused by the API with a 403 was
  written before `PrivateNetworksFullAccess` and before the cutover, and **nothing
  had ever applied one**: the arc's only instance-bearing scenario,
  `lb-serving-paris`, declares no NIC. So it was folklore. Measured instead:

  - **Real Scaleway applies it.** `state: available`, attached, in the run's own
    project. The contradiction ADR-0025 was written to end is genuinely gone.
  - **The mock does not.** `501 Not Implemented: POST
    /instance/v2alpha1/zones/{zone}/private-network-interfaces` — and Layer 3 runs
    only `if deployErr == nil`, so a failed mock apply stops it. Every compute
    scenario is now blocked at **Layer 2**, not Layer 1 or 3.

  Two readings corrected along the way, both from inferring rather than running:
  "mockway has no private networks" (it has full CRUD on `/vpc/v1` and `/vpc/v2`;
  my HCL was missing an explicit `scaleway_vpc`), and the 403 claim itself.

  Next slice is small and named: `POST`/`GET`/`DELETE` for
  `private-network-interfaces` in mockway. Detail:
  [docs/layer3-coverage.md](docs/layer3-coverage.md).

- 🌱 **S154 SHIPPED — `live observe`, the first post-apply signal (2026-08-31)** —
  probes every live deployment's health path once and records what it saw on that
  deployment's record. **No learning yet, deliberately**: an observation is not a
  lesson until it is reproduced, and that gate is S156's.

  Three decisions worth keeping. **`unhealthy` and `unreachable` are separate
  statuses** — "it told us it is broken" and "we got no answer" are different
  facts, and a loop that collapses them learns the wrong lesson. **A probe that
  could not run records nothing**, because a failure to observe is not an
  observation of failure. And **port and health path are snapshotted at deploy
  time**: the scenario file changes, while the record describes one deployment
  that already happened.

  Observations are a capped ring (`MaxObservations = 50`) — a permanently broken
  deployment emits one per probe forever, and the plan's own risk section is about
  that firehose. `live ls` grew a `HEALTH` column, because an observation nobody
  can see is not a signal.

  **Converged in three codex passes (44–46)**, the first slice under the
  one-clean-pass rule. Both findings were in code written for this PR, and pass
  45's was in the fix shipped alongside pass 44's — caught only because the rule
  change came with an obligation to re-read your own fix against its defect class.
  Both were also **YAML-resident**: the gate workflow has no type system and no
  test, and is the only place that both reads these invariants and can leak real
  infrastructure.

  **Pass 44 corrected a false green.** A live deployment with no address was
  *skipped*, exiting zero — and the case comes from today's deploy path, not from
  legacy records: `registerDeployment` captures the address best-effort, so an
  apply that produced no load balancer address leaves a live deployment nobody
  can monitor. It fails now, naming which half is missing and the way out. A
  released deployment still skips, because there is genuinely nothing to observe.

  It also **re-reads the record before writing**. `observe` is the command most
  likely to be on a cron, so it is the one most likely to be mid-probe when an
  operator runs `live teardown` — and a read-modify-write over a probe that took
  seconds would put `state: live` back over a record teardown had just released.
  This narrows the window rather than closing it; the store has no
  compare-and-swap, and claiming more than narrowing would be wrong.

  One probe per invocation, no retries: retrying would smear over exactly the
  flapping this exists to notice. Scheduling is a cron, like `live reap` — a
  daemon would be another thing to supervise for no signal cron does not give.

  **Verified end to end against real Scaleway**, which also closed the gap the
  cutover canary named: `deploy` and `live teardown` had never run for real.
  deploy → HTTP 200 → observe healthy → observe again → teardown → account clean.
  Only the healthy path ran for real; `unhealthy`/`unreachable` are unit-covered.
  [docs/status/s168-cutover-canary.md](docs/status/s168-cutover-canary.md).

- ⚠️ **The `layer3-gate` cannot validate this change before it merges (2026-08-31)** —
  it builds its binary from **base**, deliberately (S144-T5a: otherwise a same-repo
  PR could rewrite the checks judging it), so it ran *main's* shape check — which
  requires a `scaleway_account_project` — against fixtures the cutover strips it
  from. Fails at `allowlist` in 0s, before any API call; nothing leaked.
  **Any change that inverts a check in the trusted binary is unverifiable by its
  own gate until merged.** Re-run the gate on the next PR after merge.

  The run did catch one real thing: **the gate's reap step keyed off the state
  file**, and reported "nothing records them, so reap cannot run" when it was
  missing. Post-cutover a run can own a project and never write state, and the
  marker says a project was created — so the step consults it when there is no
  state, and only there: nothing removes the marker after a successful delete, so
  checking it first made every green run pay for a redundant reap (caught by pass
  45, confirmed on disk). The
  same defect the codex loop found seven times in Go, surviving fourteen passes
  because it lives in YAML.

- ✅ **S168 canary: the cutover verified against real Scaleway (2026-08-31)** —
  three applies from `s166-cutover`, ~€0.005. `block-paris` and `lb-serving-paris`
  both pass with **no `scaleway_account_project` and no `project_id` anywhere in
  the HCL**, and `lb-serving-paris` serves a real HTTP 200 through a real LB.

  **D6 moved exactly as pass 32 predicted, and was caught doing it.** `destroy`
  now *passes* — the project is not its resource — so the `resource is still in
  use` 412 lands on the Account API delete, where `releaseRunProject`'s purge
  unblocks it: *"deleted 2ab4a19e… after purging 1 resource(s) the API created
  but Terraform did not own"*. Reproduced on both compute runs.

  One run failed on `real_probe` with a 503; its apply had reported `succeeded on
  attempt 2 (real API returned a retryable error)`, so the instance was young
  when the probe ran. Re-run passed unchanged — diagnosed by running it again
  rather than from the log, because a transient and a regression read the same on
  one sample.

  Account verified against the API afterwards, not from the sweep's own verdict:
  **zero `if-run-*` projects**, and the only server and volume in the org are
  `openclaw-prod` and its root volume from 2026-02-21. Full record:
  [docs/status/s168-cutover-canary.md](docs/status/s168-cutover-canary.md).

- 🔧 **Pass 41: creation is uncancellable now (2026-08-31)** — pass 40 put
  `ensureRunProject` inside the signal guard and thereby handed it a cancellable
  context, so a Ctrl-C timed *inside* the create request could abort the client
  after the API had made the project. It traded one window for another. Both
  properties are needed and are not in tension: the guard **active** during
  creation, the create itself **uncancellable**. Creation now runs on
  `context.WithoutCancel` with a timeout, inside `ensureRunProject` so `test`
  gets it too — losing the id is worse than the extra second, because the id is
  the handle.

- 🔧 **Pass 40: `deploy` could lose a project to Ctrl-C (2026-08-31)** — it
  created the run's project *outside* `runDeployApply`'s signal guard, and the
  deployment record is written only after the apply, so an interrupt in between
  left a real project with no record and nothing coming for it. Creation and the
  env build moved inside the guard. Separately, deploy discarded
  `ensureRunProject`'s staged failures — the ones carrying the leaked project id
  and how to remove it by hand — in favour of a generic "nothing was applied", on
  the one path where that id is the operator's only handle.

- 📝 **Pass 39: the cutover had not finished telling the generator (2026-08-31)** —
  the Layer 3 prompt forbade `project_id` in one bullet and asked for it in the
  next ("wire resources to the bootstrapped project"), which does not fail loudly
  — it spends repair iterations. Worse, `pitfalls/scaleway.yaml` still told every
  Scaleway generation that a private NIC **cannot be applied**, the exact conflict
  this cutover ends and one the canary had already disproved. Both fixed, plus
  three docs (`NEXT_SESSION.md`, `layer3-coverage.md` ×2, ADR-0024) that carried
  the blocker as live fact and now record it as history.

- 🔧 **S166+S167 cutover in progress (2026-08-31)** — the guard core first,
  wired next. `AssertRunProjectDeletable` replaces the state-derived cross-check
  with **two checks that must both pass**: the marker
  (`.infrafactory-run-project`, written beside the state at creation, same trust
  level as the tfstate it replaces) and API provenance (the `if-run-` + fixed
  description stamp, **not locally forgeable**).

  Neither alone: the marker alone is a pure downgrade, and provenance alone would
  authorise deleting *any* stamped project — parallel runs could delete each
  other's, which the old check cannot do because it pins to one id. The
  organization-default refusal is unchanged and runs first.

  **Wired**: the marker is written when the project is created (a failed write is
  fatal — an unrecordable project is unreclaimable), and all three destroy paths
  — `destroySandbox`'s purge, `reap`, `live teardown` — go through one
  `assertRunProjectDeletable` helper, so none can carry a weaker check. `reap`
  reads the marker instead of the state, since ADR-0025 took the project out of
  Terraform.

  **The cutover is complete and the flag is gone.** One model: the run's project
  is created before every apply and is the provider default. The shape gate
  **inverted** — `project_id` is now *forbidden* on resources (there is no correct
  value; the provider default is the run's own project) and a declared
  `scaleway_account_project` is refused as a second project nothing tracks. The
  generation-path check inverted with it, the prompt now tells the generator not
  to create one, and the gate fixtures and recorded generation were regenerated.
  `deploy` gets its own project too, deleted by `live teardown` once the sweep
  proves the account clean.

  `Describe` distinguishes **gone** from **unreachable**: a 404 is a fact the
  guard acts on, anything else is an error, because "we could not check" must not
  look like "already deleted".

  **Cleanup ordering, from review passes 30 and 31.** The project is deleted
  **before** the orphan sweep on every path — `test`, `reap` and `live teardown`
  — because the sweep's job is to verify the project is gone and tofu can no
  longer delete it. Getting this wrong made every *clean* teardown report a leak,
  and it had to be fixed three times, once per path: the condition was right each
  time and the set of places it ran was not. The sweep now takes its blast radius
  from the marker, so a run that applied nothing is still verified, and an
  unreadable state file is a sweep **failure** rather than a silent "no strays".

  A deployment with a project but no state — the ordinary shape of an apply that
  failed at preflight, init or plan — is now `live teardown`'s business rather
  than the operator's: `reclaimable()` gates on the marker, not the state file,
  and the no-resources path runs the guard, deletes the project, sweeps and
  releases.

  **D6 moved, and pass 32 caught it.** `destroySandbox` purges Scaleway's
  auto-created default security group and retries *because `tofu destroy` fails*
  on `resource is still in use`. After the cutover destroy **succeeds** — the
  project is not its resource — and the 412 lands on the Account API delete
  instead. `releaseRunProject` now purges and retries itself, and reports what it
  removed; without that, every run declaring compute leaks a project again, as
  invisibly as the first time, because nothing billable survives to fail a cost
  check. The deletability guard moved inside `releaseRunProject` for the same
  reason `destroySandbox` holds its own: four paths reach it and it deletes real
  infrastructure with Terraform nowhere in the loop.

  S165's one documented gap — `run`'s auto-destroy-on-failure path keeping its
  project — **is closed**. It existed because the id lived in the state file that
  path could not read; the marker gives it one.

  **Every teardown path now names the project explicitly** (pass 33). `live
  teardown` and `reap` build their environment with
  `sandboxCommandEnvForProject`, so the destroy runs with the same provider
  default the apply did rather than the shared fallback. The interrupt guard
  deletes the project after its cleanup destroy — it is the one exit with no
  stage summary, so a project kept there could not even be reported — and it no
  longer treats a missing state file as "nothing to clean up", because an
  interrupt between creating the project and the first apply leaves exactly that.

  Six placements of "delete the project" have now been wrong across passes 30–33
  and none of the conditions have been. Removing a deletion Terraform used to
  perform invalidated every path that had been relying on it without saying so,
  and they surfaced one review at a time rather than from any single reading.

  **Pass 37: the same asymmetry, on three error paths.** Fixing the provider
  default on the happy path left three places where an empty project id arrived
  by accident — a zero-value marker in the interrupt guard, a failed
  `CaptureSweepTarget` in `run`'s auto-destroy, and the deployment record rather
  than the marker in `live teardown`. None is a literal `""`, so pass 34's audit
  could not see them. The project id now comes from the marker directly and is
  separate from the sweep capture (which also answers "what strays", and took the
  project down with it when the state was unreadable); where the marker cannot be
  read, cleanup **stops and says why** with the recovery command rather than
  destroying against the shared fallback.

  Then the class was closed at the seam: `sandboxCommandEnvForProject` now
  **errors** on an empty project id. The builder that tolerates one is
  `sandboxEnvWithProjectDefault`, reachable only from the credentials preflight,
  which discards the env it builds. Three passes fixed instances of this; the
  seam ends it.

  **Pass 36 declined both findings** — both asked for a state-file fallback when
  a workdir has no marker, which is the dual model the cutover dropped. Checked
  rather than assumed: no such workdir exists (the live store is absent,
  `./output` holds no live state, and the only two on disk are committed test
  fixtures), so the fallback would be a second project-resolution path that no
  test run can walk, guarding a case with no instance. The safety property behind
  the finding is already held by pass 35. Both sites now carry the reasoning
  inline so the next reviewer meets the decision rather than re-deriving it.

  **Pass 35: nothing forgettable on no evidence.** Gating `reclaimable()` on the
  marker fixed the post-cutover shape and broke its mirror — a pre-cutover
  workdir has state and no marker, and unreclaimable is what routes an operator
  to `live forget`, which retires a record while resources keep running. Now
  marker **or** state keeps a record in teardown's hands; `releaseRunProject`
  *skips* (not fails) when no marker names the project, because nothing there
  created it through the Account API; and the sweep, which asks the API, is the
  judge. The redundant guard on `tofu destroy` itself is gone — destroy is
  bounded by its own state, while the purge and the Account delete each carry
  the guard.

  **Pass 34 stopped the recurrence at the type level.** An audit test for the
  apply/destroy project asymmetry already existed — and read `test_command.go`
  alone, which is why the same defect kept landing in the other three files.
  `sandboxCommandEnv` is now `assertSandboxCredentials(runtime) error`: it checks
  Layer 3 can run before the project exists and returns **no environment**, so
  nothing can be built without naming a project. The audit was widened to every
  `internal/cli` source file, catches an explicit `""` (the one thing the
  signature cannot), refuses to pass if it finds fewer than two files to read,
  and is verified against synthetic drift.

- 📐 **S166 design written for review (2026-08-31)** —
  `docs/plans/s166-teardown-guard-design.md`. Not implemented: this is the slice
  that touches `AssertProjectDeletable`, the guard between an automated destroy
  and real infrastructure.

  Proposal: replace the state-derived cross-check with **two** checks, both
  required — a run-owned marker written beside the state (identity: "this run
  created *this* project", trust parity with tfstate) and an API-side provenance
  check against S165's `if-run-` + description stamp (class: "this is an
  infrafactory disposable project", **not locally forgeable**). Neither alone is
  sufficient, and provenance alone would be a regression: it authorises deleting
  *any* stamped project, so two parallel runs could delete each other's.

  **All four judgement calls answered 2026-08-31**, and one of them changed the
  arc: challenged on *why a non-production tool needs a transition at all*, the
  dual-model plan was dropped. S166+S167 are now **one atomic cutover** and
  `scaleway.create_run_project` is deleted as the scaffolding it was — two code
  paths is where this arc's bugs came from. Over-strict is named as the preferred
  failure mode, given that five of S165's nine review findings were cleanup that
  did not run.

- ✅ **S165 done: implemented, reviewed clean, canaried (2026-08-31)** —
  ADR-0025's run-owned project. `scaleway.create_run_project` (default **false**)
  creates the run's project via the Account API before the apply and passes it as
  `SCW_DEFAULT_PROJECT_ID`, wherever the Layer 3 apply goes through
  `executeTest` — `test` and `run`. `deploy` refuses it: it keeps its project by
  design, so deletion belongs to `live teardown`.

  **Canary on real Scaleway**: `block-paris`, **14.2s**, every stage pass —
  `run_project` created `33838f22-…`, `orphan_sweep` destroyed `7394b328-…`,
  `run_project_delete` removed `33838f22-…`. Account back to its 3 baseline
  projects, both ids **404**, verified against the API rather than the tool's own
  report.

  It shows the two-project behaviour and cleans up both, because the HCL still
  declares `scaleway_account_project`. So it proves create/pass/delete works and
  does not break the existing flow — **not** the post-cutover shape, which the
  shape gate still forbids. That wart ends when the cutover deletes the flag.

  Converged after **nine Codex passes (20–28)**, and what they found matters more
  than the feature: almost nothing was a wrong computation. Every real finding was
  an **operation ordered so a failure left something behind** — a project created
  before its config was validated, a cleanup attached to a branch some exit path
  skipped (three times, three places), a delete inheriting a cancelled context,
  and a decision reading the command's failure list instead of the teardown's
  verdict. The condition on *whether* to delete was right from the first version;
  where and when it ran was wrong five times.

  Known gap, reported not silent: a run whose destroy falls to `run`'s
  auto-destroy-on-failure path keeps its project — that path sits outside
  `executeTest` and has no access to the id.

- 🧪 **ADR-0025 + S165–S168 planned (2026-08-30)** — take the run's project out
  of the generated HCL. `scaleway_instance_private_nic` has **no `project_id`
  attribute** (verified against provider 2.81.0), so it is created in the
  provider's default project — the shared containment project — while its server
  is in the run's own, and the API refuses the mismatch. Since `vpc_required.rego`
  denies any instance server without a NIC, **Layer 1 required a resource Layer 3
  could not create**: no Scaleway compute scenario satisfied both gates.

  Fix, proven on real Scaleway *before* the ADR was written: create the project
  through the Account API before the apply, pass it as `SCW_DEFAULT_PROJECT_ID`,
  and drop `scaleway_account_project` from the HCL. A private network + instance +
  **NIC** stack applied cleanly (NIC `available`, confirmed against the API) and
  destroyed cleanly. The blocker was never the allowlist, the permission or the
  pitfall — the NIC is allowlisted and gate-exempt already. It was *when the
  project came into existence*.

  **It tightens blast radius.** An omitted `project_id` currently lands in the
  shared fallback project alongside other runs' strays; afterwards it lands in the
  run's own disposable project and is swept with everything else.

  **The care is in S166.** `AssertProjectDeletable` refuses a target that does not
  match the project named in `terraform-live.tfstate` — the check that stops a
  tampered record aiming teardown at real infrastructure. With the project out of
  the HCL that input disappears, so it is replaced by a run-owned marker **plus**
  an API-side provenance check, both required, in the same change that removes it.

  Incidentally, **D6 reproduced a third time**: the experiment's project refused
  to delete until the auto-created `Default security group` was purged — in a path
  that never touched infrafactory's code.
- ✅ **Review CLEAN at pass 13 (2026-08-30)** — `codex exec review --base main`
  found no correctness, safety or maintainability issues in the diff. S153a/S153b
  converged after four Codex passes (10–13), archived as
  `docs/review-passes/pass10.md` … `pass13.md`.

  Worth recording: pass 10 returned **one** finding where a Claude
  `/code-review` pass had returned fifteen — and that one was the most serious of
  them. The three earlier Claude passes are not counted toward convergence.

- 🔎 **Review pass 12 (2026-08-30)** — one finding: `live forget` still rejected
  records teardown cannot reclaim, where the record decodes but carries no
  `project_id`. Same no-escape loop as pass 11, one class along. `reclaimable`
  now requires a project id.

- 🔎 **Review pass 11 (2026-08-30)** — three findings, all real, all fixed. A
  dropped `store.Put` error meant a failed sweep-marker write left the sticky flag
  false, silently undoing pass 10's fix. `live forget` **rejected exactly the
  record teardown refused** — a dead end created while closing the previous one,
  leaving no CLI escape at all. And dropping `WaitDelay` removed the kill
  fallback, so a `tofu` ignoring SIGINT would hang forever and stop `deploy` ever
  reaching registration; `Cancel` now arms a SIGKILL fallback scoped to
  cancellation, where a normal exit cannot trip it.

- 🔎 **Review pass 10 (2026-08-30)** — `codex exec review --base main`, archived
  in `docs/review-passes/` — one file per pass, `pass10.md` … `pass13.md`. **Codex returned one finding**, and it was
  the same one a Claude `/code-review` pass had rated worst: the empty-state
  release path rebuilt the sweep target with **nil `Strays`**, so a sweep that had
  failed on resources *outside* the run project would be re-run against a project
  that no longer exists, report clean, and release the record — laundering the
  failure the branch existed to prevent. Fixed with a sticky
  `SweepVerificationFailed` flag: **positive verification, never the absence of
  contrary evidence.**

  Four Claude-only findings acted on anyway, each a real safety problem: `live
  forget` would release a *healthy* deployment on one command (permanent
  untracked leak); `MarkReleased` wrote outside the store before rejecting a
  traversing id; the `CLAUDE_CODE_` prefix stripped `CLAUDE_CODE_OAUTH_TOKEN`,
  which would have broken generation in CI — worse than the hang it fixed; and
  `WaitDelay` applied to every command, where its timer also starts on a *normal*
  exit, so a lingering provider plugin would turn a successful apply into
  `ErrWaitDelay` with truncated output.

  Five declined with reasons recorded, including one whose stated threat model is
  false — `scenario.schema.json` does constrain scenario names, so a traversing id
  cannot arrive by that route.

  **Process correction**: the three earlier passes used Claude's `/code-review`
  skill, not the Codex loop `AGENTS.md` requires. A same-family reviewer shares
  the blind spots that produced the defect, which is plausibly why three rounds of
  fixes each reproduced the failure they targeted.

- 🔧 **S153b (2026-08-30)** — the review of S153a found 15 more findings, several
  of them **regressions S153a itself introduced**. Worst: the empty-state shortcut
  released a record with a PASS without re-running the orphan sweep, so a teardown
  whose sweep had *failed* was laundered green on the next pass and the orphans
  became invisible. The sweep is now re-run against the record's project id, and
  the record is released only if it passes.

  Also fixed: `exec.CommandContext` sent **SIGKILL** to `tofu`, so an interrupted
  apply could leave a resource live and absent from state — now SIGINT plus a
  bounded `WaitDelay`, because tofu forks provider plugins that hold the output
  pipes. An undecodable record had **no CLI route out** (`live teardown` failed at
  load, `live reap` failed forever) — new `live forget` releases without
  destroying, says exactly what it gives up, and preserves the unparseable bytes
  beside the record rather than overwriting the project id they may still contain.
  `validateID` now guards **reads** as well as writes. Records written with a
  relative `WorkDir` resolve deterministically. The deploy failure hint is keyed
  off whether registration *succeeded*, so it no longer says "nothing to tear
  down" in the one case where a project is live and unrecordable. A cancelled
  parent is no longer reported as a Ctrl-C. `live ls --output json` marks
  unreadable records instead of emitting phantom deployments.

  **And generation is unblocked.** `claude_adapter.go` filtered only
  `CLAUDECODE=`, while a parent Claude Code session exports nine more
  (`CLAUDE_CODE_MESSAGING_SOCKET`, `CLAUDE_CODE_BRIDGE_SESSION_ID`, …). The nested
  `claude` inherited them, behaved as part of the parent session, and hung on the
  first phase that used a **tool** — `self_review`, the only one that writes files
  — until the 300s timeout. Phases 1 and 2 are pure text and completed, which is
  why it looked like a `self_review` bug rather than an environment one. Now
  prefix-matched so a new variable is excluded by default.

  **Verified, not inferred**: provider 2.81.0's `scaleway_instance_private_nic`
  has no `project_id` attribute, so the containment conflict cannot be fixed in
  HCL. And **#163 (TypeScript 5→7) does not install** — `@sveltejs/kit@2.70.3`
  declares `peerOptional typescript@"^5.3.3 || ^6.0.0"`, so npm refuses without
  `--force`. TypeScript 6 would satisfy that range.

- 🔧 **S153a hardening (2026-08-30)** — a post-merge audit of #170 returned
  **15 findings, several of them real leaks** in code that destroys real
  infrastructure. #170 merged without a review pass; the loop now runs on every
  PR before merge, not after.

  Two were principles written into a doc and not implemented beside it.
  **ADR-0024 rule 3 promises "unreadable means expired"** — but `Reapable`
  filtered only decodable records, so an undecodable one never reached the reaper
  and `MarkReleased` could not clear it either: it failed every pass, forever,
  with no way out. And **`live reap --dry-run` returned `nil`**, discarding
  failures already recorded for unreadable records, so a dry run exited 0 while
  something that might be running was unaccounted for.

  The leaks: **second-resolution deployment ids** meant two deploys of one
  scenario in the same second shared a record path *and* a workdir, so the second
  apply adopted the first's state and left a project running with nothing that
  knew how to destroy it — the per-deployment workdir defeated by the id, and the
  arc's own test slept 1100ms to dodge it. **No SIGINT guard on `deploy`**, so
  Ctrl-C during a ~140s apply killed the process with the project already created
  and the record not yet written — `live ls` showed nothing and it billed
  indefinitely. **A relative store root** made a scheduled reaper in any other
  directory report "nothing has expired" and exit 0. **Non-atomic writes**
  manufactured exactly the truncated record that could not be recovered. And
  **unsanitised ids** reached `filepath.Join`.

  Also fixed: teardown of an already-destroyed deployment reported a permanent
  false leak alarm (destroy empties the state the next pass reads); `--output
  json` was unparseable for `deploy` and `live reap`; the deploy failure hint
  pointed at teardown even when nothing was recorded; `live ls` swallowed stray
  args; and `docs/layer3-coverage.md` contradicted itself on the training-set
  denominator — the same unenforced-prose bug S152 took credit for catching.

  **Two misdiagnoses of the private-networking blocker, both retracted.** The
  repo said `IPAMFullAccess`; the real API wanted `write compute_private_networks`
  (`PrivateNetworksFullAccess`, granted). Private *networks* now create; private
  *NICs* still cannot, and no permission fixes it — the resource takes no
  `project_id`, so it lands in the ADR-0010 containment project while the server
  is in the run's own. Then I claimed `vpc_required.rego` was "wired for AWS only"
  and narrowed the pitfall on that basis. Also false: `filterPolicyPathsByCloud`
  drops only *other* clouds, so all five `policies/scaleway/*.rego` run. A
  generation run failed on exactly that rule. Pitfall restored, claim retracted in
  four files.

  **What is actually true is worse: the two gates contradict each other.** Layer 1
  requires a private NIC on every `scaleway_instance_server`; Layer 3 cannot apply
  one while the run creates its own project. **No Scaleway compute scenario
  satisfies both today.** That — not IPAM, not permissions, not the pitfall — is
  what blocks generated HCL from reaching real infrastructure.

  **Generation was also blocked on this machine, and is now fixed.**
  `claude_adapter.go` filtered only `CLAUDECODE=`, while a parent Claude Code
  session exports nine more `CLAUDE_CODE_*` variables. The nested `claude`
  inherited them, behaved as a child session, and hung on the first phase that
  used a **tool** — `self_review`, the only one that writes files — until the 300s
  timeout. With the family unset, all three phases complete.

- ✅ **Live-services canary green (2026-08-30)** — `deploy` → `live ls` →
  `live teardown` proven against **real Scaleway**, first attempt. Deploy 35.8s,
  teardown 36.1s, **HTTP 200 through a real load balancer**, account back to its
  3 baseline projects with the canary project returning 404 — verified against
  the API, not from the command's own report.

  Seeded HCL, deliberately: the `lb-serving-paris` fixture stood in for generated
  output so any failure would be S151–S153 code rather than generation (the S143
  pattern). **D6 reproduced itself in the new teardown path** — Scaleway
  auto-created a `Default security group`, destroy could not delete the project
  while it existed, and the S146 purge-and-retry cleared it *and said so*. Without
  that reporting the teardown would have read as an ordinary clean run.

  **Gap it exposed**: `deploy` records the *declared* image without verifying what
  is running. The record said `nginx:1.27`; the instance served
  `python3 -m http.server`. Queued for S155, where upgrade makes it load-bearing.

- 🚧 **S153 landed (2026-08-30)** — the first slice where something stays up,
  and the thing that takes it down, in one PR. `infrafactory deploy` applies a
  validated scenario and records it; `live teardown <id>` destroys one;
  `live reap` destroys everything past its TTL.

  **`deploy` deliberately does not generate.** `run` proves a change is safe;
  `deploy` takes what `run` validated and keeps it. The allowlist is still
  re-checked at deploy time, deny-by-default — *already validated* is a claim
  about a previous command.

  **Every deployment gets its own workdir** (`<live root>/workdirs/<id>`), never
  the shared `output/<scenario>`. Two deployments of one scenario would otherwise
  share a single `terraform-live.tfstate` and the second apply would overwrite
  the first's, orphaning real resources with nothing left that knows how to
  destroy them — the D6 class again, invisible to any cost check.

  **The registry is not authority for what gets destroyed.** It says which
  deployment; the state file says which project, and `AssertProjectDeletable`
  gets both, so a disagreement is fatal. A deployment whose state has vanished is
  reported and **not** released: its resources may still be running, and
  releasing it would retire the only record that says so.

  Registration happens on the failure path too — a half-finished apply leaves
  real resources, and the record is the only thing that brings the reaper back.

  **Known hole, deliberately left open**: the reaper trusts the store to know
  every live deployment. A wiped working directory loses records while the cloud
  keeps the resources. Reconcile-against-API is the largest remaining gap in
  ADR-0024 and is queued for S157.

- 🚧 **S152 landed (2026-08-30)** — the app gets a version. `service:` in
  `scenario.schema.json` names the image, tag, port, health path and TTL that a
  live-service scenario runs, and `scenarios/training/web-live-paris.yaml` is the
  first to declare one. Infrastructure-only scenarios are unaffected: no
  `service:` block means nothing changes.

  **The tag must not move.** `latest`, `stable`, `edge`, `main`, `master` and
  friends are refused by the loader, because an upgrade from a tag that moves
  cannot be told from a no-op and a soak failure cannot be attributed to a
  version. This catches the common case and is not a proof of immutability — a
  numeric tag like `1` moves too, and only a digest is genuinely fixed. TTL is
  bounded at 168h as a typo control, not a cost control: `400h` where `4h` was
  meant should fail at validation rather than on an invoice.

  Caught by the audit tests, and worth recording because it is the S147 failure
  class exactly: `docs/layer3-coverage.md` counts `**runnable**` rows as
  scenarios that **have run**, so adding `web-live-paris` as runnable would have
  made the doc claim a real-cloud run that never happened. The table had no way
  to say *ungated but never run*. It now does — `runnable, unrun`, a fourth
  bucket in both the doc and `TestLayer3CoverageDocTotalsMatchItsTable`. Ungated
  is 4 of 18; have-run remains 3 of 18, and the row becomes `**runnable**` the
  day it goes green, not before.

- 🚧 **S151 landed (2026-08-30)** — first slice of the live-services arc
  (`docs/plans/live-services-arc-plan.md`). `internal/livestore` records
  infrastructure that deliberately outlives the run which created it, and
  `infrafactory live ls` reports what is running and how long it has left.

  Nothing persists yet — this slice is bookkeeping, on purpose: there must be a
  record of what is out there before anything is allowed to stay up. The
  decisions are in **ADR-0024**, and all of them fail toward teardown. An expiry
  is mandatory and there is no value meaning "forever". A record that will not
  decode, or that lacks an expiry or a project id, is reported as **expired and
  reapable** rather than skipped — "we could not check" must never look like
  "nothing is running", so `live ls` exits non-zero and names the record.

  Caught while writing it: `MarkReleased` originally routed through `Put`, which
  validates. A damaged record is still reapable and its resources are still
  real, so the reaper would have destroyed them, failed to record that it had,
  and destroyed the same already-destroyed deployment on every later pass.
  Releasing now bypasses validation; creating still does not.

  Live deployments will get their own Scaleway project rather than teaching the
  orphan sweep to discriminate live resources from strays — that would make
  sweep correctness depend on registry accuracy, where one stale entry is a
  silent leak (the D6 class). **The ephemeral invariant is therefore unchanged:
  per-run projects are still destroyed and swept.**

- 🔒 **S150 landed (2026-08-24)** — first slice of the Presentable arc, taken before S144 so the workflow that will hold cloud credentials is born into a hardened pipeline. `govulncheck` in CI, explicit `permissions:` on every workflow, SHA-pinned actions on the two workflows with elevated capability. **The scanner found ~20 known stdlib advisories on its first run**: `go.mod` pinned `go 1.25.0` and several were genuinely reachable (`GO-2026-6218` in `net/url` via `http.Client.Do` from `internal/api/server.go`), so every binary built here was linked against a vulnerable stdlib. Now pinned to `go 1.25.13`. Posture and the goldfinger comparison in `docs/ci-security-posture.md`.

- 🚦 **S148 in progress (2026-08-26)** — `make demo-gate` puts a generated change on a PR, labels it, waits on the deployment approval and follows the run; `make demo-preflight` fails rather than reads, and checks the account is clean before you start. **The demo is agent-authored**: `infrafactory generate` writes the HCL and a script only carries it to a PR, so the agent in the talk's title is on screen rather than taken on faith.

  Finding that changed the design: a real generation of `lb-serving-paris` is **refused by the gate** — the generator emits `scaleway_instance_security_group` and private networking, which need IPAM and are not allowlisted. `block-paris` generates cleanly in 76s and passes every check, so it is the demo scenario. The same run also showed the function ban was too strict to survive contact with the generator (`tags = concat(...)`), which is now a pure-function allowlist.

  Outstanding: S148-T2 (recorded fallback — needs one approved real run to record) and S148-T3 (three rehearsals, operator-only).

- 🔒 **S144 landed (2026-08-25)** — the arc's spine. `.github/workflows/layer3-gate.yml`: apply the `layer3-gate` label to a PR and, **after a human approves the deployment**, the gate applies that PR's infrastructure code to real Scaleway, probes it, destroys it, sweeps the account and comments the stage summary back. That comment is the artifact the talk rests on.

  **27 codex passes, 25 findings.** Almost all of one class: the gate evaluates PR-supplied HCL with real cloud credentials in the environment, and every enumerate-the-dangerous-ones guard produced a bypass. What survived is deny-by-default at each layer — an allowlist of top-level block types, providers restricted to `scaleway/scaleway` **pinned to an exact version** the base-branch binary carries plus a trusted `.terraform.lock.hcl` copied from the base checkout, every resource required to bind `project_id` to the run's own disposable project (or, for child resources, an in-stack parent), and **all function calls refused** — `user_data = file("/proc/self/environ")` reads `SCW_SECRET_KEY` and ships it to a machine the PR boot-scripts. The binary and harness come from the base branch; only HCL comes from the PR, because a maintainer label is not a security boundary.

  The `layer3` environment carries **required reviewers**, deliberately. The approval prompt is the thesis made visible: the *without breaking it* is a human approving a real-cloud apply, and the machine then proving it applied, verified and destroyed.

  Verified end to end against real Scaleway with every restriction active: 144s, provider 2.81.0 from the trusted lock, account clean.

- 🔒 **S145 + S146 landed (2026-08-24)** — the evidence and the coverage. `examples/layer3-plan-lied/` holds two infrastructure changes that clear `validate`, clear `plan`, clear a **mock apply**, and are refused by real Scaleway, both 10/10 reproducible: a block volume asking for `iops = 9000` (the API sells 5000 or 15000) and a DNS zone under a credential not permitted to create one. Which of the two a mock *could* have caught is written down, because presenting the first as though it were the second is the fastest way to lose a technical audience.

  `lb-serving-paris` is the first scenario to satisfy an `http_probe` against real Scaleway — one instance serving behind a real load balancer, green end to end in **144 seconds**. Building it surfaced **D6**: Scaleway auto-creates a `project_default` security group on the first Instance in a project, Terraform never destroys it, and so `tofu destroy` cannot delete the project. ADR-0010's disposable project — the foundation of every blast-radius claim in the arc — had silently stopped holding for any scenario declaring compute, and nothing billable survived, so a cost check would have kept reporting clean. `destroySandbox` now purges what the API auto-created inside the run's own project and retries once: guarded by `AssertProjectDeletable` in the wrapper rather than at the call sites, restricted to groups the API marks `project_default` *and* reports as belonging to the run's project, and **reported in the stage summary** — the first verification run passed with the purge firing invisibly, and that output was the only way to tell the fix from a coincidence. Also mockway#22: the provider polls a v2alpha1 endpoint mockway answered with 501, and Layer 2 gates Layer 3.

- 🔒 **S147 landed (2026-08-24)** — `docs/layer3-evidence.md`: the six findings Layer 3 produced that Layer 2 and unit tests did not, each with its *mechanism*, because "the real cloud is different" is folklore and a mechanism is not. The denominator leads rather than trails: **3 of 17** Scaleway scenarios have ever run against the real API, on **1 of 4** clouds, over roughly 50 applies mostly repeating two fixtures. Three scenarios is not a trend and no rate is claimed from it. Numbers are measured — eleven real gate runs, the six green ones at 60–95s, `lb-serving-paris` at 143/144/144s — and there are no euro figures, because Scaleway pricing is not codeable and an invented number is worse than none.

  Four consecutive review passes then found counting errors in `docs/layer3-coverage.md`, each one a paragraph disagreeing with a table two screens away, and fixing each by hand introduced the next. The totals, the gated remainder and the enumerated allowlist are now derived from the table's own rows and diffed against `infrafactory.yaml` by `TestLayer3CoverageDoc*` — the project's drift-becomes-a-failed-test pattern applied to prose.

- 🔒 **S149 landed (2026-08-24)** — README section for a stranger arriving after the talk: what Layer 3 proves, **what it does not**, and why only Scaleway. The negative list is the load-bearing one — the pattern has never run on the other three clouds, 3 of 17 Scaleway scenarios have ever touched the real API, and a green run means the cloud accepted the change and it cleaned up, not that the change is good. Caught in review: the section described the label-triggered PR gate as though it existed, and it is still on the unmerged S144 branch — a public safety claim readers could not verify. Now qualified, and `TestReadmeLayer3GateClaimMatchesWorkflows` fails in *both* directions, so the claim and `.github/workflows/layer3-gate.yml` rise and fall together.

- 🚧 **Active arc**: `docs/plans/presentable-arc-plan.md` (S144–S150) — making infrafactory demonstrable on stage for the *"Letting Agents Change Production Without Breaking It"* talk, 2–6 weeks out. Scoped by what the talk must be able to **show**. The spine is S144, a label-triggered PR gate that applies to real Scaleway, destroys, sweeps, and comments the stage summary back — which is what turns "before it ever reaches a production repo" from aspiration into an artifact. S145 builds the evidence that `plan` passes where the real cloud refuses; S150 closes the CI-hardening delta found against `redscaresu/goldfinger` (govulncheck, explicit `permissions:` blocks, SHA-pinned actions).

- 🎯 **Baseline: 44/44 deterministic** + **fakegenesys v0.2.1** + **27 paired contracts across the family** (CI-enforced via `handlers/contract_audit_test.go` in all 4 siblings) + **ADR-0021 cloud-prefix lockstep CI-enforced**.
- ✅ **Last arc complete**: `docs/plans/layer3-real-scaleway-plan.md` (S139–S143) — **Layer 3 now applies to and destroys real Scaleway infrastructure.** Code-complete since S30, it had never called `api.scaleway.com`; it does now. Both canary runs are green end-to-end (`preflight → init → plan → apply → destroy → orphan_sweep`), run 1 with seeded HCL to isolate the harness and run 2 with the **LLM generating the HCL** — `target_reached` in one iteration. Sweeps clean and independently confirmed against the account; it ends in the same state it started. Guardrails: sealed sandbox env + fail-closed endpoint assertion, mockway `account/v3` projects with non-empty-delete, real-API orphan sweep with a state-derived stray check, `AssertProjectDeletable`, interrupt-safe destroy + `infrafactory reap`, a disposable fallback project, and a deny-by-default resource-type allowlist — each with synthetic-drift coverage.

The canary found three defects nothing else could. Run 1: the orphan sweep read `terraform-live.tfstate` *after* destroy emptied it, and mockway's seeded default project counted as a Layer 2 orphan — which would have failed `no_orphans` for **every scenario in the suite** (both fixed in S143a). Run 2: real Scaleway returned a block-volume create error *after* the volume existed, leaving it tainted; transient, and a re-apply succeeded. Diagnosing it was hard because Layer 3 reported only `exit status 1` — both sandbox paths discarded the provider's message. Fixed via the shared `stderrFailureDetail` helper; ADR-0023 amended (fail-closed is necessary but not sufficient).

Deltas in `docs/layer3-real-vs-mock-deltas.md`. Close-out in `docs/status/ARCHIVE.md`.

**Layer 3 follow-ups now closed** (post-arc). A failed run no longer just destroys and hopes: it captures the sweep target before destroy and sweeps afterwards, and any failure raised while real resources may still exist carries `infrafactory reap <scenario>` in its detail, where the operator actually reads it. Apply also retries once — bounded, surfaced as `succeeded on attempt 2`, and never on a cancelled context, so the interrupt guard still holds.

Codex review loop now runs on every PR (2 consecutive clean passes to converge; passes recorded in `docs/review-passes/`). Pass 1 on the arc caught one real defect: the `infrafactory reap` recovery hint was not shell-quoted, so a scenario path containing a space produced a command that would not run.

**Dependency pipeline unblocked** (2026-08-23). Doc Hygiene required a `STATUS.md` edit for any `go.mod`/`go.sum` change — which dependabot cannot make — so with required status checks on `main`, **every Go dependency PR was permanently unmergeable**; eight had piled up, the oldest three weeks old. Dependency-manifest-only changes are now exempt, all-or-nothing: a PR that bumps `go.mod` *and* touches `internal/` still needs `STATUS.md`. `scripts/check_doc_hygiene_test.sh` pins both halves and runs in CI.

**Probe canary run (2026-08-23)**: `lb-paris` gained a `connectivity` criterion and ran against real Scaleway. The real-probe path works — it resolved the LB's public IP from `terraform-live.tfstate` and opened a TCP connection to it, the first time that code has run against anything but a mock. It also found a **real leak**: `test` only cleaned up after a *successful* sandbox apply, so a partial apply (killed by an IAM permission error) left a real project and LB IP billing until reaped by hand. Fixed — cleanup is now gated on the sandbox layer having run, not having worked. Deltas in `docs/layer3-real-vs-mock-deltas.md` (D4).

**Superseded next-arc opener**: `lb-paris` as probe canary — but note it currently declares **no probes** (only `region_restriction` + `no_orphans`), so the arc needs probe criteria added to the scenario first; it is not just a run. The five scenarios that do declare probes (`compute-lb-multi-paris`, `mysql-ha-paris`, `redis-xlarge-session-paris`, `web-app-paris`, `gcp-cloud-sql`) are all substantially more expensive. Costs real money either way, so scope it deliberately.

## Recent arcs

| Arc | Outcome | PRs |
|---|---|---|
| S89–S93 (2026-06-03) | 🎯 39/39 first deterministic; fakeaws Secrets Manager soft-delete fix; AWS phase2 audit (10/10 Cat C). Option C scaffold shape adopted. | fakeaws#6, #69, #70 |
| S84–S88 (2026-06-03) | gcp-full-stack convergence (servicenetworking escape pitfall); `scripts/sweep_39.sh` panic gate. | #64, #65, #66 |
| S79–S83 (2026-06-02) | Sibling-mock drainage + N3 carve-out validation; `cmd/s3router/` shim added. | fakeaws#5, #58–#61 |
| S74–S78 (2026-06-02) | AWS + Scaleway phase3 collapse; `make sweep-39`; N3 GCP-escape carve-out. | 5 PRs |
| S54–S73 | GCP phase2 collapse, sustain ratchets, N3/N10/N13 architecture build-out. | ~22 PRs across 4 sub-arcs |

Full per-arc close-outs in `docs/status/ARCHIVE.md`.

## OSS-readiness

All four repos (`infrafactory`, `mockway`, `fakegcp`, `fakeaws`) ship Apache-2.0 + SECURITY.md + CODE_OF_CONDUCT.md + CONTRIBUTING.md + CHANGELOG.md + release workflow. Pre-commit hook (`gitleaks` + `go test`) installable via `make install-hooks`. Full-history `gitleaks detect` zero leaks.

**User-only click-ops pending** (private → public visibility flip + branch protection on each repo).

## Known blockers

None.

## Open tickets

None — `docs/tickets/rename-learning-system.md` closed by S104.

## Update policy

- Trim this file every arc close-out — historical detail belongs in `docs/status/ARCHIVE.md`.
- Goal: ≤ 30 lines. If it grows past 50, time to trim again.
- ADRs and `CONCEPT.md` carry durable architecture decisions; STATUS.md is just the current-shape pointer.
