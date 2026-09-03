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
statuses `deployHandler` can produce *before* `Deploy` is called: 400, 404, 405,
and 423. `500` is deliberately excluded — `writeActionResult` returns it for a
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
