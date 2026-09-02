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
