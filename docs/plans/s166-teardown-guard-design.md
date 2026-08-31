# S166 design: replacing the teardown guard's second source of truth

Written 2026-08-31 for review **before** implementation. S165 is merged and
canaried; S166 is the slice that must land before S167, and it is the one that
touches the guard standing between an automated destroy and real infrastructure.

Nothing here is built yet. The judgement calls are collected at the end.

## What the guard does today

```go
AssertProjectDeletable(stateProjectID, targetProjectID, organizationID) error
```

It refuses when the target is empty, when the target **is** the organization's
default project, when `terraform-live.tfstate` records no project at all, and
when the target does not match the one the state records.

The last two are the substance. `terraform-live.tfstate` is an *independent
witness*: Terraform writes it during the apply, and it names the project as a
managed resource — "this run created this". Nothing a caller passes can
manufacture that. `reap` and `live teardown` both lean on it, and `reap` passes
the state-derived id as *both* arguments precisely so the check cannot be
bypassed by an argument.

## What ADR-0025 removes

Under the new model the project is created through the Account API before tofu
runs, so it is **not a Terraform resource** and `terraform-live.tfstate` will not
name it. Once S167 removes `scaleway_account_project` from the HCL, the witness
is gone and the check has no input. That is why the ordering is a safety
requirement: **removing the resource before replacing the guard that reads it
breaks the guard.**

## The proposed replacement: two checks, both required

### 1. A run-owned marker (identity)

Written beside the state, by infrafactory, at the moment the project is created:

    <workdir>/.infrafactory-run-project

Holding the project id and the name it was created with. It answers *"did this
run create this specific project?"*

Trust level is **the same as the state file's** — local, written by the tool
during the run, not supplied by a caller and never by PR-supplied HCL. It is not
stronger; it is parity, deliberately. A human with an editor can forge it exactly
as they could forge tfstate today.

### 2. API-side provenance (class)

The project must still exist and carry the stamp S165 already applies: an
`if-run-` name prefix **and** the exact `RunProjectDescription`. It answers *"is
this an infrafactory disposable project at all?"*

This one **cannot be forged locally**. To defeat it you would have to create a
real project shaped like an infrafactory run project — at which point you have
created the kind of thing that is safe to delete.

### Why neither is sufficient alone

- **Marker alone** is a pure downgrade: it replaces one local file with another,
  and adds nothing the state file did not already give.
- **Provenance alone is worse than it looks.** It authorises deleting *any*
  `if-run-*` project, not this run's. Two runs in parallel, and one teardown could
  delete the other's project — a regression the current check does not have,
  because it pins to one id.

Together each covers the other's weakness: forge the marker and provenance still
refuses anything unstamped; create a lookalike project and the marker still
refuses anything this run did not record.

The organization-default refusal is **unchanged**, and stays first.

## Failure modes

Every uncertain outcome refuses. "We could not check" must never equal "safe to
delete" — the lesson the orphan sweep already encodes.

| situation | verdict | why |
|---|---|---|
| marker missing | **refuse** | cannot show this run created it |
| marker unreadable/corrupt | **refuse** | same, and consistent with `livestore`'s unreadable-is-expired |
| marker names the organization default | **refuse** | unchanged existing check, applied first |
| API says the project is gone (404) | **allow** | already the desired end state; mirrors `Delete`'s 404-is-success |
| API unreachable / errors | **refuse** | an unverifiable delete is the one this guard exists to stop |
| project exists, **not** stamped | **refuse, loudly** | something is badly wrong; this is not our project |
| marker and target disagree | **refuse** | the argument does not get to win over the record |

## Migration: a disjunction, not a replacement

Both models coexist until S167 lands and the fixtures move. During that window a
project may have been created either way, so the guard answers *"may I delete
X?"* with yes when **either**:

- the state names X as `scaleway_account_project` (old model, today's check), or
- the marker names X **and** X carries the provenance stamp (new model)

This is a disjunction, which is weaker than requiring both. It is not weaker than
today, because each branch is independently sufficient for the model that
produced it — but it is the part of this design I would most want a second
opinion on. The alternative is a hard cutover, which cannot be staged and would
break the PR gate.

## Blast radius if this is wrong

An over-permissive guard lets an automated destroy delete a project this run did
not create. The organization default is refused by a separate check that is not
being touched, so the worst realistic case is deleting **another infrastructure
project that happens to carry infrafactory's stamp** — i.e. another run's, or a
stray from a previous run. Bad, recoverable, and loud.

An over-strict guard refuses legitimate teardowns, leaving real resources
running. Given today's evidence — nine review passes on S165, five of them about
cleanup not running — **over-strict is the safer failure and the one to prefer
when uncertain.**

## Judgement calls I want reviewed

1. **Is marker + provenance genuinely equivalent to the state-derived check?**
   My claim is yes, and slightly better because provenance is not locally
   forgeable. Disagreeing here changes the whole design.
2. **Is the migration disjunction acceptable?** It is the weakest point. The
   alternative is a flag day.
3. **Should the marker live in the workdir or in the live-deployment record?**
   Workdir is proposed, because `reap` operates on a workdir and has no record.
   `live teardown` has both.
4. **Does "API unreachable ⇒ refuse" make teardown too fragile?** It means a
   network blip leaves resources running until someone retries. That is the
   deliberate choice, but it has a cost and it is worth stating out loud.

## Explicitly out of scope

Removing `scaleway_account_project` from generated HCL and the fixtures — that is
S167, and it must not land until this does. Signing or tamper-proofing the
marker: it is parity with the state file, and a local tool that a user can already
edit gains little from cryptography.
