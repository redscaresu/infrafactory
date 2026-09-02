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
