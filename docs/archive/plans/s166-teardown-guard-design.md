# S166 design: replacing the teardown guard's second source of truth

Written 2026-08-31 for review **before** implementation. S165 is merged and
canaried. S166 replaces the guard standing between an automated destroy and real
infrastructure — and, per the cutover decision below, it now lands **together
with** S167 as one atomic change rather than before it.

Nothing here is built yet. The four judgement calls this document was written to
surface were answered on 2026-08-31 and are recorded below as decisions.

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
name it. Removing `scaleway_account_project` from the HCL takes the witness away
and leaves the check with no input — which is why the guard and the HCL change
are the same change, not two ordered ones.

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

## No migration: a cutover (decided 2026-08-31)

The design originally proposed running both models side by side and accepting
whichever check applied. **That was wrong, and the question that killed it was
"what is the benefit of a transition for a non-production tool like this?"**

There is none. No deployed fleet, no external consumers, nobody mid-migration.
The only thing a transition protected was the PR gate's fixtures — and the gate
runs on every PR, so breaking it is *caught*, not discovered later. Rollback for
a single-user repo is `git revert`, not a migration plan.

It also cost something real: two models means two code paths, and two code paths
is where this arc's bugs came from — five of S165's nine review findings were a
cleanup path that did not run, and the dual model is what produced the
"two projects per run" wart. **`create_run_project` was scaffolding mistaken for
a feature.**

So: **one model, one guard, one path.** The flag is deleted.

### Consequence: S166 and S167 are one atomic change

The guard's input changes at exactly the moment the HCL changes, so they cannot
be separated. One slice, containing: the new guard, the shape gate no longer
requiring a `scaleway_account_project` binding, prompts and pitfalls updated, and
the gate fixtures plus recorded generation regenerated.

That is a large security-critical change to review at once. **"Split for review
size" is a different argument from "split for migration"** — this needs a hard
review and a real-cloud canary before merge, but it does not need a flag kept
alive to get them.

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

## Decisions (2026-08-31)

1. **Marker + provenance is accepted as equivalent.** The marker gives identity
   at tfstate's trust level; provenance adds something tfstate never had — a
   check that cannot be forged locally. Neither alone: marker alone is a pure
   downgrade, and provenance alone authorises deleting *any* stamped project, so
   parallel runs could delete each other's.
2. **Cut over; no transition.** See above. The flag is deleted and S166+S167
   become one slice.
3. **The marker lives in the workdir**, beside the state, as
   `.infrafactory-run-project`. `reap` has no deployment record — it exists for
   runs that died before anything was recorded — so a record-only marker would
   leave the guard blind on the command most likely to be aimed at orphaned
   infrastructure. It also sits where the witness it replaces sat.
4. **API unreachable ⇒ refuse.** Consistent with the orphan sweep's rule that
   "we could not check" must never look like "nothing leaked". A blip costs a
   retry; proceeding wrongly costs a project nobody meant to destroy. Resources
   keep billing either way, so the asymmetry favours refusing.

## Explicitly out of scope

Removing `scaleway_account_project` from generated HCL and the fixtures is **in**
scope, not out — it is the other half of the same change (see the cutover
decision). Implementing the guard without it produces a guard with no input.

Signing or tamper-proofing the marker: it is parity with the state file, and a local tool that a user can already
edit gains little from cryptography.
