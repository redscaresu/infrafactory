# ADR-0027: deploying from the UI

Status: accepted (2026-09-02, S160c)

## Context

The UI arc's plan says the deploy safety model is *"decided before the capability
exists, not after."* Three of its four proposals have since been built for other
reasons, so this ADR is mostly a record of what is already true plus the
decisions that are specific to **creating** infrastructure.

What exists already:

- **An origin guard** (ADR-0026, S160a). A request carrying a non-loopback
  `Origin` is refused above the routing, so a page the server did not serve
  cannot reach any endpoint.
- **Start-time capability gates.** `--allow-layer3` decides whether UI-started
  runs touch real infrastructure (S160b); `--allow-teardown` decides whether the
  UI can destroy deployments (S159b). In both cases the capability is *absent*
  rather than present-and-refusing, so a request cannot confer it.
- **A named confirmation.** Teardown states scenario, project and address before
  it will proceed (S161b).

What is not decided, and is decided here: what it takes to let a web page create
infrastructure that **outlives the request, costs money by the hour, and is
reachable from the internet.**

## Decision

### 1. `infrafactory ui --allow-deploy`, and it is not implied by anything

Deploy gets its own start-time flag. It is **not** implied by `--allow-layer3`
and not implied by `--allow-teardown`, and both of those would be tempting
shorthands.

They are different capabilities. `--allow-layer3` permits an *ephemeral* apply
that the run destroys before it finishes: the blast radius is minutes and the
account ends as it started. `--allow-deploy` permits infrastructure that is
**kept** — that is the entire point of ADR-0024 — so it bills until something
reaps it and serves the internet until then. An operator who agreed to the first
has not agreed to the second.

`--allow-teardown` is the opposite direction of harm and implies nothing about
this one either. Someone who enabled the UI to *clean up* has not asked for it to
create.

Without the flag the endpoint does not exist and answers **404**, for the reason
S159b gives: announcing "not implemented" advertises a capability the operator
declined.

### 2. The confirmation states cost, lifetime and blast radius, before the click

A deploy confirmation must show, from the scenario about to be deployed:

- **what it will create** — the resource shapes, not a count;
- **what it costs per hour**, and the total at the chosen TTL;
- **when it expires**, as a wall-clock time rather than a duration, because "4h"
  is a number people agree to without arithmetic;
- **that it is reachable from the internet**, when the shape includes a public
  address.

The cost figure is a *list price estimate* and must say so. ADR-0024's table is
list prices read from Scaleway's pricing page, not measured spend, and a
confidently wrong number is worse than an admitted estimate.

### 3. A TTL is mandatory in the UI, exactly as it is in the CLI

ADR-0024 has no unbounded form and the UI does not introduce one. The control
offers bounded choices and has no "no expiry" option to click.

### 4. Deploy is a distinct verb from run, in the UI as in the CLI

S153 split `run` and `deploy` so that keeping infrastructure could not be reached
by accident from the verb that proves a change is safe. The UI must not re-merge
them into one "go" button. `run` proves; `deploy` keeps; the distinction is the
thesis the whole project rests on.

### 5. An apply is not cancellable by the caller

Same rule as teardown (ADR-0026, S159b) and `ensureRunProject`: once an operation
begins changing real infrastructure, the caller going away must not stop it. A
deploy cancelled mid-apply leaves resources with no complete record of what was
created — the leak D6 taught this project to fear, arriving by a different route.

### 6. What deploy may not do

- It may not deploy a scenario with no `service:` block. `runDeployCommand`
  already refuses; without one there is nothing whose version could be rolled
  forward and "deploy" would just mean "apply and forget to destroy".
- It may not choose the project. Run-owned projects are created by the harness
  (ADR-0025) and a request that could name one is a request that could name
  somebody else's.
- It may not raise the TTL of an existing deployment. Extending a lifetime is a
  separate decision from granting one, and it is not in this arc.

## Consequences

**Three flags now gate three capabilities**, and that is deliberate rather than
untidy. Each names a distinct kind of harm: spending money on an ephemeral run,
destroying what exists, and creating what persists. Collapsing them into one
`--unsafe` would make the cheapest of the three carry the weight of the most
expensive, and operators would enable it for the cheap reason.

**The default UI can do none of them.** `infrafactory ui` with no flags reads the
estate, runs mock validation, and edits scenarios. Everything that touches real
infrastructure or real money is off until somebody says otherwise in a shell.

**This ADR is a gate, not an implementation.** S162 builds the button against it;
if S162 needs to weaken anything here, that is a change to this document and its
own argument, not a detail of the pull request that happens to need it.

## Amendment, 2026-09-02 (S162b): the endpoint, and what it may be told

`POST /api/deployments` creates a deployment. `DeploymentDeployer` is a separate
interface from `DeploymentActor`, so §1's argument — that creating and destroying
are different kinds of harm — lives in the type system rather than only in this
document. A server holding one cannot be talked into the other.

**The request carries a scenario name and an optional TTL, and nothing else.** The
absences are the decision: no project, because a request that could name one could
name somebody else's; no skip-validation; and no value meaning "forever".

**A name, never a path.** `deploy` takes a filesystem path, and accepting one over
HTTP would let a request name any YAML on the machine, including one outside the
scenarios tree that the layers have never seen. Resolution walks the tree matching
the declared `scenario:` field.

That resolver reads **only the name**. Loading each candidate through
`scenario.Load` would validate it against `DefaultSchemaPath`, which resolves
relative to the working directory — so in a server process started anywhere else,
every file would fail validation, be skipped, and the API would report "no
scenario named X" for a scenario sitting right there. Validation is deferred, not
skipped: the command loads the path through the runtime's own loader.

**The seam drives the command rather than reimplementing it.** Between the request
and the apply sit a deny-by-default Layer 3 HCL preflight, a credentials check, a
per-deployment workdir, a run-owned project created inside an interrupt guard, and
a registration step that writes the record *even when the apply fails*. A second
implementation of that sequence is a second thing that can be wrong, on the path
that spends money.

### A result that cannot be read is a failure

The command emits `{"schema": ..., "result": {...}}`. Unmarshalling that into the
inner `OutputResult` **succeeds with every field zero**, because unknown keys are
ignored — so a successful deploy read as unclean, with no steps and no failures,
and the endpoint would have answered 409 after creating infrastructure.

A parse that cannot fail is worse than one that does: there is nothing to notice.
The envelope is now required, and output that is not it is reported as a failure
saying *whether infrastructure was created is unknown* — because the apply may
well have created some, and an empty result would say the opposite.

## Amendment, 2026-09-02 (S163): a long action reports as it goes

A deploy runs for minutes. Minutes of silence reads as broken, and a reader who
cannot tell a long apply from a hung one will do one of two harmful things: kill
it, or start another.

**The harness reports, because it is the only thing that can.** The first attempt
tee'd the deploy command's stderr, which is written twice before any cloud work
and once after the apply returns — and `CommandRunner.Run` returns a fully
buffered result, so `tofu`'s own output does not exist until each process exits.
There is no stream to tee. `SandboxDeployHarnessRunner.Run` therefore takes an
`io.Writer` and announces each stage as it begins, including **retries**, since a
silent retry is indistinguishable from a stage that is merely slow.

Stage granularity rather than raw provider output: it answers the reader's actual
question — *where is it up to?* — in a predictable handful of lines.

The progress writer is a parameter rather than a field on the harness, because the
harness is constructed once and shared, and two deploys running at once must not
write into each other's stream.

The command's progress is streamed to the websocket, one **line** per event.
Lines rather than writes, because a command using several `Fprintf` calls
otherwise produces fragments, and the last line — often the one that matters — is
flushed on close even without a trailing newline.

**Every event names its subject.** A deploy outlives the page it was started from,
so its events arrive on whatever page is open. This is the same rule S162c arrived
at for outcome messages, applied to progress before it could cost anything: a
client can filter, label or ignore, but it is never handed an unattributed
statement about something the reader may not be looking at.

Watching is never required. The apply is detached from the request, so closing the
tab stops the stream and not the deploy.

**The subject is the scenario, not the deployment id**, and that is a limit rather
than a choice: the id is minted inside the command, after the request is accepted.

Two concurrent deploys of one scenario are now prevented **within a process** by
S163c's lock, so a reader will not see two streams interleaved that way. The
hazard the keying creates is not retired, only narrowed: **sequential** deploys
produce several live deployments of one scenario — which is why `already_live`
returns a list — and a reader watching a scenario-keyed stream with two ids live
still cannot tell which one they are watching. That matters because the id is the
argument to `live teardown`, and taking the wrong one has consequences.

## Amendment, 2026-09-03 (S163c): the guard against a second deploy is server-side

Two deploys of one scenario produce **two run-owned projects and two sets of
billable resources for one thing**, and until now the only thing preventing it was
client-side state.

A page is exactly the wrong place for that guard. A refresh wipes it, a second tab
never had it, and `curl` never consulted it. Three review findings across two
rounds were variants of the same hole, and each fix moved the client state around
without addressing that it was client state.

`LiveDeployer` holds the lock. A second `Deploy` of a scenario already in flight
returns `ErrDeployInProgress`, which the endpoint answers **423 Locked** — naming
the scenario, because a bare refusal leaves a reader wondering which of their tabs
is responsible.

**423 and not 409**, because 409 on this endpoint already means something else: a
deploy that *ran* and could not prove itself clean, carrying an `ActionResult`. A
refusal sharing that status was parsed as a result, found no `clean` field, and
told the reader *"resources may still be running"* after a request that never
touched the cloud.

**Per scenario, not global.** Two different scenarios deploying at once is
ordinary, and blocking it would make the UI worse for no safety gain.

**The lock is an in-memory map in ONE process, and that is the larger limit.**
The CLI `deploy` command goes straight to `runDeployCommand` and never touches
`LiveDeployer`, so `infrafactory deploy` run alongside a UI deploy of the same
scenario produces two run-owned projects — as would a second server instance. What
the lock closes is duplicate deploys *from one UI*, which is where the accidental
ones come from.

**It also prevents CONCURRENT duplicates only.** Deploying a
scenario, waiting for it to finish, and deploying it again still produces a second
run-owned project — the lock is released when the first completes, and nothing
consults the live estate. The ADR previously stated the harm without that
qualification, which overstated what this closes. What it does close is the
accidental duplicate: the reload, the second tab, the double click. Warning about
an *existing* live deployment is a different guard, and it belongs in the
confirmation rather than in a lock.

**Released on every exit, including failure.** A scenario stuck marked-as-deploying
could never be deployed again without restarting the server — a worse failure than
the one being prevented.

The claim happens after name resolution, and that ordering is **tidiness rather
than safety**. An earlier version of this ADR said a typo could otherwise "lock a
name nothing will ever deploy"; mutation testing disproved it, because the
deferred release fires on the resolution failure too. Recorded because a false
safety claim is what the next reader reasons from — the release is the guarantee,
the ordering is not.

### The listing says what is deploying, and that is advisory only

`GET /api/deployments` reports `deploying: []`. An applying deploy has no record
yet, so it cannot appear in `deployments`, and a listing of records alone would
describe an estate as empty while it was busy creating one. The refusal is the
guard; this is what stops the reader being invited to trip it. A client that
ignores the field still cannot start two.

(Written as "so a reloaded page can restore what it was showing". That consumer
was deleted in the S163e amendment below and the sentence outlived it —
corrected 2026-09-03.)

The field is always a list, never `null`: an unconfigured deployer and an idle one
are both "nothing is deploying" to a reader.

### A finished deploy's banner is forgotten when its page is left

It used to reappear on every later visit for the rest of the session — a success
message for infrastructure whose TTL may long since have expired.

## Amendment, 2026-09-03 (S163e): the page knows only what it did

The scenario page tracked deploys started anywhere — another tab, the CLI, itself
before a reload — by adopting them from the server, recovering from missed
terminal events, and resynchronising on reconnect.

**That machinery produced 36 review findings across three rounds**, and twice a fix
introduced a defect of the class it was fixing. It is removed.

The argument for removing it rather than continuing to repair it is the one that
prompted the review: *there is no difference between `terraform apply` to a mock
and to a real cloud.* That is true, and it locates the problem. What is genuinely
hard about real infrastructure is what an **incomplete** apply leaves behind, and
which record still points at it — which is what ADR-0024's TTL, ADR-0025's
run-owned project, `live reconcile` and the write-the-record-on-failure rule all
exist for. Those are small and have been stable.

None of the 36 findings were about that. They were a browser state machine
mirroring server state, and every one of them was the mirror disagreeing with the
thing it mirrored.

**So the page knows only what it did.** It shows the deploy it started, its log,
and how it ended — which survives navigation between scenarios, the case where a
reader is watching something they began moments ago. After a reload it does not
know, and it says so, pointing at the estate.

### The estate page had a gap, and it is closed

Calling it the source of truth required it to be one. A deploy that is *applying*
has no record yet — `registerDeployment` runs after the apply returns — so it
could not appear in the table. `GET /api/deployments` already reported `deploying`,
and the page now shows it, saying explicitly that those have no record yet.

That field was computed and unread after the cut, which is the defect this review
round kept finding. Rendering it where it belongs was the alternative to deleting
it, and the gap it fills is real.


## Amendment, 2026-09-03 (S163e-fixes): only the server can say nothing started

A fifth review round, and its findings are what a cut leaves behind.

### A rejected request is not evidence that nothing happened

`POST /api/deployments` is deliberately detached from the request that starts it
(`destructiveContext`), for the reason the whole ADR exists: an apply that keeps
running after the client goes away ends with a record describing what was made,
and one that is cancelled halfway does not.

The consequence had not been carried through to the client. The page treated
**every** failed request as "nothing was started" — deleting the deploy's entry
and its progress log. A dropped connection mid-apply therefore erased the log of
a live apply and told the reader it did not exist, while a project was being
created and billed.

**The server is the only thing that can say nothing started, so it is now the
only thing that does.** `DeployError.startedNothing` is true for exactly the
statuses that can be produced *before* `Deploy` is called: 400, 404, 405 and 423
from `deployHandler`, plus 403 from `guardCrossOriginRequests`, which wraps the
whole mux and answers before any handler is entered. `500` is deliberately excluded — `writeActionResult` returns it for a
deploy that ran and errored, which may have created resources.

Everything else keeps the entry, keeps the log, and says the deploy may still be
running.

### Warnings accumulate; they are never chosen between

"One is applying right now", "some are already live" and "the estate could not be
fully read" are three different facts, and all three can hold at once. Returning
the first discarded the most actionable of them — that a second deploy creates a
second project and a second bill — precisely when the reader was most likely to be
duplicating something.

### What the estate page can and cannot promise

It cannot be *wrong* about a record that exists; that is the argument for
deleting the browser-side mirror, and it holds. It is not omniscient: the
in-progress list is one process's in-memory lock, so a CLI deploy, or an apply in
flight when the server restarted, is invisible until it finishes and writes its
record. The scenario page's scope note now says "everything recorded" rather than
"everything that is running".

### Known cost, accepted

The preview endpoint reads the whole estate on every Deploy click. It is one read
per deliberate human click against an estate bounded by what somebody is willing
to pay for, and a cached index would be a second source of truth that can
disagree with the store — the exact class this arc spent three rounds removing.
Worth a lighter endpoint when an estate is big enough to notice; not worth an
index.


## Amendment, 2026-09-03 (round six): what the corrections got wrong

### The preview reads what is applying before it reads the estate

The two reads cover each other in one direction only. A deploy has no record until
registration, which runs *after* the apply returns — so reading the estate first
leaves a window in which a deploy that finishes in between is in **neither**
answer, and the confirmation renders with no warning and an explicit "checked, and
nothing exists" claim at the exact moment the scenario goes live.

The other order has no such window: anything that leaves the in-flight list after
the first read has necessarily registered before the second. The order is a
guarantee, and there is a test that fails when it is swapped.

### The in-flight list is as stale as the rows beside it

The previous amendment had the estate page keep `deploying` across a failed
refresh. Correct, but it was then stated in the present tense, while the rows read
in the same request said "read before the error" about themselves. Surviving an
error does not make a fact current. It is now qualified, from a single shared
builder that the summary line and the banner both use — they sit two lines apart
and had been wording the same count independently.

### Only a deploy still being watched absorbs its stream

Progress events are keyed by scenario, and a finished entry stays on screen until
the reader leaves. Without a running check, a second deploy of the same scenario
from another tab or the CLI appended into it — a live log of an apply this tab did
not start, underneath a completed-outcome banner. That is the adoption this ADR
removed, reachable through the one door left open.

### Every ending is an outcome

A refusal used to be held outside the store, and that single difference in scoping
produced three defects in three rounds: rendering on whichever scenario the reader
had navigated to, outliving the attempt that caused it, and — once guarded by a
navigation token — deleting the entry while reporting nothing at all, so the button
silently reverted to "Deploy…" as though the click had never landed.

Outcomes are keyed by scenario, which scopes them by construction. There is no
token and no clearing rule. This was only available once the running-only filter
above removed the reason refusals were forgotten, which is the general shape: when
the third instance of a class arrives, delete the state that permits it rather than
guarding the path again.

### The body decides whether a response is a result

The client special-cased a bare 409 as an `ActionResult` because
`writeActionResult` produces one. So does anything else that answers 409 — a proxy,
an intermediary, the next refusal somebody adds — and such a body has no `clean`
field, so it rendered "resources may still be running" for a request that never
reached the deployer. Moving the "already deploying" refusal to 423 fixed that for
one producer; checking for `clean` fixes it for all of them.

`startedNothing` remains a client-side allowlist of pre-apply statuses, now
including the origin guard's 403. It is an allowlist deliberately: erring in this
direction sends a reader to check the Deployments page for infrastructure that was
never created, and erring the other way tells them nothing happened while a project
is being created and billed. A discriminator in the response body would close the
class; it is not built.

### A deploy does not speak in teardown's vocabulary

`teardownOutcome` was reused for deploy results, so a failed deploy could render
"Teardown returned nothing." next to a Deploy button. Only the failure branch was
affected, because the template overrides the success text — which is exactly why it
went unnoticed, and exactly where a reader can least afford a confusing message.


## Amendment, 2026-09-04 (round seven): the flush, and what the server cannot see

### The last line is broadcast before the response

The page stops accepting progress for a deploy the moment its request resolves — an
entry that is no longer running must not absorb somebody else's stream. That makes
the ordering between the sink's final flush and the HTTP response load-bearing: a
line emitted after the response is a line the browser discards.

`ProgressSink.Close` is therefore called explicitly before `writeActionResult`, and
still deferred for panics. It is idempotent, so the two do not fight.

This makes the tail arrive first; it does not GUARANTEE it (corrected 2026-09-04).
The broadcast goes out on the websocket and the response on the HTTP connection,
and two connections have no ordering between them — an in-process test can only pin
the order the server writes in. It is kept as the better of two orderings rather
than presented as a solution. The residual is narrow: `deploy` newline-terminates
every line it writes, so there is no tail to lose for today's producer. One that
emitted a partial final line would need it carried in the response body rather than
raced against it.

### A refusal discards the log it collected

The store entry is created before the POST so that progress can stream during the
apply. For a deploy that is about to be REFUSED, that means it was running — and
collecting — during the round trip, and the refusal's own reason is that another
apply of the same scenario holds the lock. Those lines belong to that apply, and
are dropped.

### A finished deploy's banner belongs to the visit it finished in

Dropping it on leaving is not enough: a deploy still running when the reader left
finishes afterwards, and nothing was left to drop it. Arriving at a scenario drops
a finished deploy too.

**A SUCCESS only** (corrected 2026-09-04). A stale success is a false claim about
infrastructure whose TTL may have expired. A failure is not a claim at all, it is
an unread report — "it may have created resources that are still running", with
the project id somebody has to remove by hand — and a deploy that fails before
`registerDeployment` has no live record either, so that banner is the only place
it is ever said. Dropping it because the reader looked away is how the leak goes
unnoticed.

The same rule applies on LEAVING, and that half took a second correction. "The
reader saw it, so it can go" sounds reasonable until you read what the message
tells them to do: *check the Deployments page before starting another*. The
Deployments link is directly beneath the button, so following the instruction was
what deleted the project id the instruction was about. One predicate,
`forgetIfSucceeded`, at both ends — a failure survives until the tab does.

### What `already_deploying: false` does and does not mean

It means this server is not applying that scenario. It does not mean nobody is.
The in-flight list is one process's memory, so an `infrafactory deploy` in a
terminal, or an apply in flight when the server restarted, is absent from it — and
no field this server sets can say otherwise, which is why there is no
`already_deploying_unknown` to mirror `already_live_unknown`.

The guard for that case is not in the preview. It is the run-owned project
(ADR-0025), which contains a duplicate rather than merging it into an existing one,
and the estate page, where both appear once recorded.


## Amendment, 2026-09-04 (round ten): what a banner is FOR decides when it goes

Three rounds circled one rule and got it wrong twice, because the predicate was
about the wrong property.

A finished deploy's banner is dropped when it has nothing left to report — not
when it succeeded. `outcome.mayHaveCreated` is that question, decided where the
outcome is built:

  - **success** — recorded on the estate page, so the banner is a claim about a TTL
    that may already have expired. Droppable.
  - **refusal** — `startedNothing` is the server's own word that nothing was
    created. Nothing to report. Droppable, and keeping it made a transient "already
    deploying" reappear on every visit for the rest of the session.
  - **unproven, or a request that failed after the apply began** — may have left
    resources with no record anywhere, and carries the project id somebody has to
    remove by hand. Kept until the tab closes.

A report that is kept also has to be REACHABLE. The outcome banner lives inside the
page's `{#if detail}` guard, so a scenario read that happens to fail hides it; the
load-error branch renders every kept report instead, named individually, because
without `detail` the page cannot say which scenario it is.

### An absent field is not an empty one, on the client either

The server makes an empty `already_live` a checked claim, returning `(out, true)`
on every path that did not look. Reading a missing field as `[]` discarded that at
the client boundary — an older server, or a body trimmed by an intermediary, would
render no warning at all, indistinguishable from "we looked and there is nothing".


## Amendment, 2026-09-04 (round eleven): the server says whether anything started

`started_nothing: true` in the response body, written by `writeRefusal` on the
paths that reject a request before it can touch the cloud. The client reads that
field; it no longer keeps a list of status codes.

The list was not merely fragile, it was already wrong. `deployHandler` answers
**404 in two places**: for a scenario that does not exist, before the apply, and
for an `os.ErrNotExist` returned by `Deploy`, after it. `DeploymentDeployer` is an
interface, so any implementation whose post-apply error wraps `os.ErrNotExist`
produces the second — and a client reading the status called it "nothing started",
discarding the progress log of a live apply and telling the reader nothing
happened. The origin guard's 403 sat outside `deployHandler` entirely.

Absence of the field means unknown, which is the safe direction: it sends a reader
to check the Deployments page for infrastructure that may not exist, rather than
telling them nothing happened while a project is being billed.

### A report survives a retry

The obvious next action after a failed deploy is to try again, and that overwrote
the entry holding "project 7c98d82e is live and could not be deleted". If the retry
succeeded, the banner became "Deployed." and the next navigation dropped it — a
leaked project with no live record, named nowhere. Reports accumulate on the entry
instead, because two failed attempts leak two projects, and they are rendered in
the LAYOUT so following the message's own advice does not hide it.


## Amendment, 2026-09-04 (round twelve): a report is not an outcome

They had been treated as the same thing, and three rounds of defects came from it.

An **outcome** is how the last attempt ended. A **report** is a statement that
infrastructure may exist with no record of it anywhere. An entry can hold one
outcome and several reports: two failed attempts leak two projects, and a
successful third does not un-leak them.

So the rule for forgetting an entry reads `reports`, not the last outcome — judging
by the outcome and then deleting the whole entry destroyed the reports the previous
amendment existed to accumulate. And reports render in the LAYOUT, not on the
scenario page: they have to outlive the page they came from, including when the
reader follows the message's own advice to the Deployments page.

### Both listings read what is applying before they read the estate

`GET /api/deployments/preview` and `GET /api/deployments` have the same obligation
for the same reason: a deploy has no record until registration, which runs after
the apply returns, so reading the estate first leaves a window in which a deploy
that finishes in between is in neither answer. For the listing that window produced
`deployments: []` with `deploying: []`, from which the page concludes "Nothing is
deployed." at the exact moment a scenario went live.


## Amendment, 2026-09-04 (round thirteen): a report has to be silenceable

The previous amendment separated a report from an outcome. It did not say what
happens to the outcome once a report exists, and the answer turned out to be
"nothing" — the entry survived because it held a report, and the stale success
banner survived with it.

`retireDeploy` drops the banner and keeps the reports. They are different claims
with different lifetimes, and only now do they have different mechanisms.

**And a report can be dismissed.** Nothing else can retire it: the deploy failed
before registration, so there is no live record for `live reap` or the estate
listing to clear. The operator reads the project id, removes the project by hand,
and says so. Without that, the banner stays on every page for the session — and an
alarm nobody can silence is one everybody learns to ignore, which loses the message
just as thoroughly as deleting it did.

### Only the deployer knows whether a name resolved

`os.ErrNotExist` answers 404 but says nothing about WHEN. `LiveDeployer` resolves
the scenario before it claims the lock and before anything touches the cloud, so it
wraps `api.ErrNoSuchScenario` too, and the handler answers that with `writeRefusal`.
A bare `os.ErrNotExist` — a state file or a workdir vanishing mid-apply — keeps the
cautious treatment.

Without the distinction, a mistyped scenario name pinned "it may have created
resources that are still running" for the rest of the session, for a scenario that
never existed.


## Amendment, 2026-09-04 (round fourteen): only the deployer can promise nothing ran

`ErrNothingStarted` is the general form of the promise; `ErrNoSuchScenario` is a
refinement that answers 404 where its parent answers 500. Every exit from
`LiveDeployer.Deploy` that happens before the apply wraps one of them — resolving
the name, rebuilding the runtime, parsing the flags, walking the scenario root.

The claim is about infrastructure, not blame. Nothing outside the deployer can make
it: a deploy is detached from the request that starts it, and from outside, a
config error and a half-finished apply look identical. Every unmarked pre-apply
failure pinned a permanent "it may have created resources that are still running"
for a request that never reached Scaleway.

The sentinels stay OUT of the operator-facing message. Wrapping them with `%w` put
the discriminators in the body — `no scenario named "typo": no such scenario:
nothing was started: file does not exist` — which the page then prefixed with the
scenario name a fourth time. A sentinel is for `errors.Is`; a message is for a
person.

### A report is self-contained

It carries a copy of the log's opening lines. When the request never returns an
ActionResult the message is generic, and the head of the log is the only place the
run's project and workdir are named — while the log itself is cleared by the next
retire or retry.

### Every ending is stated somewhere

A report renders in the layout and not in the page's outcome slot, so the page
carries a terminal line pointing at it: a log that stops with nothing said reads as
a deploy still running. Dismissing the last report removes the entry, so nothing is
left holding an ending the page will not render.


## Amendment, 2026-09-04 (round fifteen): cleanup is scoped to what it can see

A deploy can END while the page that would show it is still loading. The hook that
retires finished deploys therefore acts on a SNAPSHOT taken when the navigation
began, not on the state it finds when the detail arrives — otherwise a refusal that
landed in between is deleted before it has ever been rendered, and the button
reverts to "Deploy…" as though the click had not landed.

Dismissing a report likewise removes the entry only when nothing renderable is
left. A non-report outcome — the success of a retry, sitting beside an earlier
attempt's leak report — is on screen, and deleting the entry took it down with the
report the reader had just dealt with.

### `started_nothing` is not a middleware concern

The cross-origin guard wraps every endpoint. Marking its 403 as a refusal stamped a
claim about whether an APPLY created cloud infrastructure onto refused reads and
onto refused teardowns, where it is a claim about the wrong verb. It is a plain
error again; a deploy client reads that as "we do not know", which is the safe
direction, and a page this server did not serve is not one whose reader is watching
a deploy.


## Amendment, 2026-09-04 (round sixteen): `started_nothing` is a deploy claim only

Two routes had it stamped on them by generalisation rather than by meaning: the
cross-origin guard, which refuses every endpoint, and the `/api/deployments`
collection's 405, which every non-POST verb reaches — including a DELETE meant for
a teardown. Both are plain errors. The claim is made only where the deploy path
knows it is true.

A sentinel also stays out of the message. `fmt.Errorf("%w: %w",
api.ErrNothingStarted, err)` reads back as "nothing was started: config is
unreadable", and the handler puts that string into the response body for a page to
render. Wrapper types carry the sentinel for `errors.Is` and leave the text alone.

### One emptiness question, one answer

`knownEmpty` takes `deployingKnown`, and every caller passes it — including
`estateSummary`, which asks the same question for the line above the panel. A
parameter added to the predicate and not to its callers puts the two claims back in
disagreement, which is what having a shared predicate was for.


## Amendment, 2026-09-04 (round seventeen): a failed deploy is not an unrecorded one

`deploy` registers from whatever the state shows, whether or not the apply
succeeded. So the usual failed deploy leaves a live record — with a TTL, on the
estate page, reapable — and treating every unclean result as an unreported leak
raised a permanent, human-only-dismissible alarm for infrastructure the system
already tracks.

`ActionResult.Deployment` and `OutputResult.Deployment` carry the id, set only when
registration SUCCEEDED, which is the condition the CLI's own recovery line uses. A
recorded failure names its record; an unrecorded one keeps the alarm, because it is
the only place that infrastructure is ever mentioned.

### Leaving retires a deploy. Nothing else does.

A banner is shown at most once — on the visit the reader came back for — and
leaving the scenario retires it. There is no arrival hook.

One existed because leaving used to retire only what had already finished, so a
deploy that finished afterwards greeted every later visit. Leaving is unconditional
now, and the arrival hook had become the destructive half: it raced the detail
fetch, deleting a refusal before it rendered, and it could not distinguish a deploy
that finished long ago from one that finished moments before the reader returned to
look at it.

### Identity, not position

A report is dismissed by an id minted when it is recorded. Positions move: two
clicks landing before a re-render deleted two different reports, and the second was
a leak nobody had read.


## Amendment, 2026-09-04 (round eighteen): who may say nothing was started

**Any path that answers before a deploy could begin.** That includes the
cross-origin guard and the collection's method check, neither of which knows which
verb it refused.

Two earlier amendments said the opposite, on the grounds that `started_nothing` is
a claim about an apply and is therefore meaningless on a read and about the wrong
verb on a teardown. Meaningless is not false — nothing was started, because no
handler ran — and withholding a true claim is not neutral. A refused deploy POST
read as "we do not know what happened", and the page pinned a permanent "it may
have created resources that are still running" for a request that never reached the
deployer. A vacuous truth on a PUT costs nothing; a missing one manufactures a
false alarm.

### A guard the caller can ignore is not a guard

The store refuses to record a second start over a running deploy. It cannot stop
the POST, so it returns whether the deploy may proceed and the caller must honour
it — otherwise the 423 that comes back is applied to the FIRST deploy, clearing its
log and marking it finished while it keeps creating infrastructure.

### Reports live in their own store

They change twice a session; the deploy store is written once per progress line.
Holding them together subscribed every route to thousands of notifications for data
that had not changed, and coupled two things with different lifetimes: a banner
that goes stale, and a statement that infrastructure may exist unrecorded.


## Amendment, 2026-09-04 (round nineteen): retiring is for leaving

`afterNavigate` fires for a navigation to the page you are already on. Retiring
there discards the banner and the apply log of a deploy the reader never left, so
the hook compares the navigation's own `from` and `to` — not `scenarioPath`, which
a reactive statement has already updated by the time the callback runs.

Every hop is optional-chained. A `from` carrying a null `url` made the expression
throw, and a throw inside `afterNavigate` aborts it: `loadDetail` never ran, and
every scenario page rendered blank.

### A report is dismissed one at a time

The entry survives while any of its scenario's reports stand. Its outcome is the
pointer the page renders at them, and its log is the only account of the apply.

### The cautious value is the zero value

`AlreadyLive` starts as an empty list so it cannot read as null; `AlreadyLiveUnknown`
starts as `true` for the same reason, because `false` is the positive claim
"checked, and nothing exists". A preview built without consulting the live store has
no right to make it.


## Amendment, 2026-09-04 (round twenty): one shape for "nothing started"

`api.NothingStarted(message, cause)` and `api.NoSuchScenario(message)`. The message
is the caller's, the sentinel is matched by `Is`, the cause is exposed by `Unwrap`.
It had grown to two exported sentinels and three bespoke error types across two
packages, each unwrapping differently — for a concept that is one bit.

The type no longer claims `os.ErrNotExist`. It did, through a custom `Is`, and that
promise held for `errors.Is` and not for `os.IsNotExist` — which unwraps three
concrete types and compares by `==`, consulting neither `Is` nor `Unwrap`. No custom
type can satisfy it short of being an `*fs.PathError`, which would put filesystem
framing back into the message this 404 exists to keep out. A promise that holds for
one of two idioms is worse than none.

### What a client may conclude from a failed deploy request

Three states, not two booleans:

  - **refused** — the server said it rejected the request before anything ran.
  - **clean** — a 2xx whose body could not be read. `writeActionResult` answers 2xx
    only for a provably clean result, so nothing was left behind and the log is
    still the reader's.
  - **unknown** — anything else. The apply may be running, and a report is filed.

### An empty `deployment` is not proof of a leak

It means one of three things: nothing was created, something was and could not be
registered, or the result was unreadable so the id never arrived. The page says "no
record of it reached this page", which is true of all three, rather than asserting
the one it cannot distinguish.


## Amendment, 2026-09-04 (round twenty-one): three lifetimes, three homes

The store held a terminal `outcome` — how the last deploy ended — and it outlives
the page. Six review rounds of defects were guards on that one fact: retire hooks,
a shown-scenario tracker, a route-change guard, an arrival hook added and then
deleted, stale-success rules, a report pointer, cross-store dismiss coordination,
and three separate ending functions.

It is deleted. What remains is three things with three lifetimes, in three places:

  - **running** → the `deploys` store. An entry exists only while a deploy is in
    flight, so it can be dropped the moment one ends.
  - **just watched** → the page, transient. One `ending`, rendered only when its
    scenario is the one on screen and cleared on a route change. Those two facts
    are the entire scoping rule: no token, no clearing pass, no staleness question.
  - **must not be lost** → `reports`, in the layout, durable and dismissible.

`endDeploy(scenario, outcome)` is the only ending: it files a report if there is one
to file, drops the entry, and returns the log so the page can keep showing what the
reader was watching.

**The trade.** A deploy that finishes while the reader is on another page is not
announced when they return. Three rounds of defects came from trying to announce it,
and the durable answers were always elsewhere — which is this ADR's own thesis.
