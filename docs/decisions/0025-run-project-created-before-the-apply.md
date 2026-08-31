# ADR-0025: The run's project is created before the apply, not by it

## Status
Accepted

## Context

ADR-0010 gives every Layer 3 run its own disposable project, and the generated
HCL creates it: a `scaleway_account_project` resource that every other resource
binds `project_id` to. `layer3_hcl_shape.go` enforces that binding, and the
orphan sweep destroys exactly the project the state names.

That works for every resource which *has* a `project_id` attribute. One does not.
`scaleway_instance_private_nic` (provider 2.81.0, verified against the schema)
exposes only `id, ip_ids, ipam_ip_ids, mac_address, private_network_id,
server_id, tags, zone`. It has no `project_id` and cannot be given one, which is
why the shape gate lists it under `layer3ChildScopedTypes` and validates it
through its parents instead.

A resource with no `project_id` is created in the **provider's default project**.
That default is `SCW_DEFAULT_PROJECT_ID`, which `sandboxCommandEnv` sets to
`scaleway.fallback_project_id` — the shared containment project. The server,
meanwhile, is in the run's own project. Real Scaleway refuses the combination:

    Cannot create a private_network_interface in a project
    attached to a server from another project

So a private NIC cannot be applied by any Layer 3 run. That matters because
`policies/scaleway/vpc_required.rego` **denies any `scaleway_instance_server`
without one**, and it is evaluated for every Scaleway plan — the per-cloud filter
drops only *other* clouds. **Layer 1 requires a resource Layer 3 cannot create.**
No Scaleway compute scenario satisfies both gates.

Two diagnoses were made before this one and both were wrong, each from reading
configuration rather than running anything: that the blocker was `IPAMFullAccess`
(the API actually asked for `write compute_private_networks`), and that
`vpc_required.rego` was AWS-only. Both are retracted in ADR-0024.

## Decision

**infrafactory creates the run's project through the Account API before invoking
tofu, and passes it to the provider as `SCW_DEFAULT_PROJECT_ID`.** The generated
HCL no longer declares `scaleway_account_project`.

Verified end to end on real Scaleway (2026-08-30) before this ADR was written: a
stack of private network + instance IP + instance + **private NIC**, with no
`scaleway_account_project` and the project supplied through the environment,
applied cleanly — 4 resources added, the NIC reported `available` and attached by
the API — then destroyed cleanly. The contradiction is not inherent; it was
caused by *when* the project came into existence.

### This tightens blast radius rather than loosening it

Under the old model, a resource that omits `project_id` lands in the **shared**
`fallback_project_id`, mixed with every other run's strays — which is why the
shape gate has to demand an explicit binding on every resource, and why the gate's
own error text warns that a resource "would be created in the fallback project
rather than this run's disposable one". Under the new model the provider default
*is* the run's own project, so an omitted `project_id` lands somewhere disposable
and swept. The hazard the gate was written to catch stops existing.

### The guard that must not be weakened

`AssertProjectDeletable(stateProjectID, targetProjectID, organizationID)` is what
stands between teardown and real infrastructure. It refuses when the target is
the organization default, and when the target does not match the project recorded
in `terraform-live.tfstate` — the second check being what stops a stale or
tampered record aiming a destroy at something the run did not create.

With no `scaleway_account_project` in the stack, the state no longer names the
project, so that second source of truth disappears. **It is replaced, not
dropped**, by two independent checks:

1. **A run-owned marker**, written beside the state at project-creation time by
   the same process that created the project. This carries the trust the state
   file used to carry: local, written by infrafactory, never by PR-supplied HCL.
2. **An API-side provenance check.** The project must still exist and carry the
   naming/description marker infrafactory stamps on projects it creates. A
   hand-edited marker file therefore cannot aim teardown at `openclaw` or any
   other pre-existing project, because those do not carry the stamp.

The organization-default refusal is unchanged. Deleting either replacement check
re-opens the hole; both are required, and they fail closed independently.

## Consequences

**Benefits.** Private NICs become applicable, so `vpc_required` and Layer 3 stop
contradicting each other and every Scaleway compute scenario becomes runnable
end-to-end for the first time. Containment improves: strays land in a disposable
project instead of a shared one. The shape gate's per-resource `project_id`
requirement becomes a belt-and-braces check rather than the only thing standing
between a run and the shared project. And the API-side provenance check is the
first piece of the reconcile-against-API work ADR-0024 has owed since S153.

**Tradeoffs.** The project is no longer Terraform-managed, so `tofu destroy` no
longer removes it — teardown must delete it through the API after the destroy,
which is the sequence `destroySandbox` already performs for the auto-created
security group. A crash between project creation and apply leaves an empty
project; empty projects are free, but they must be swept or they accumulate. And
ADR-0010's wording ("the run creates its own project") narrows to "infrafactory
creates the run's project"; the disposability guarantee is unchanged.

**Migration.** Both models must work during the transition, because
`examples/layer3-gate/*` fixtures and the recorded demo generation declare
`scaleway_account_project` today, and the PR gate applies fixture HCL directly.
The pre-created path lands behind a config flag, both paths are supported until
the fixtures and prompts are updated, and the old path is removed only once a
real-cloud canary has passed on the new one.

**Not decided here.** Whether `vpc_required` should apply to scenarios that
declare no private networking at all. This ADR makes the policy *satisfiable*; it
does not argue the policy is right.

## Amendment, 2026-08-31 (S165): the lifecycle, and where it stops

The flag is honoured wherever the Layer 3 apply goes through `executeTest` —
`test` and `run` both — creating the project before the apply and deleting it
afterwards. Three things the implementation settled that the ADR had left open.

**Deletion is conditional on the account, not on the destroy.** A run project is
removed only when nothing of the run can still exist: the account was proven
clean, or no state was ever written so there was nothing to create. Otherwise the
project is **kept and reported as a skipped delete**, because its id is the handle
to whatever survived — removing it would discard the only pointer to the leak.
A codex pass caught the inverse of this: the delete was originally gated behind
the destroy branch, so an apply failing at preflight, init or plan created a
project and never removed it.

**Creation failure is fatal to the run.** It does not fall back to the shared
project. The caller asked for a run-owned project precisely so this run's strays
would not sit next to every other run's, and silently applying into the shared
one would give exactly what was being avoided.

**`deploy` refuses the flag for now.** It keeps its project by design, so deletion
belongs to `live teardown`; honouring the flag there before that exists would
create projects nothing deletes. The refusal is explicit rather than silent.

Known gap, deliberately left: a run whose destroy falls to `run`'s
auto-destroy-on-failure path keeps its project. That path runs outside
`executeTest` and has no access to the id. It is reported, not silent, and closing
it belongs with the `deploy`/`teardown` work.

## Amendment, 2026-08-31 (S165, pass 23): apply and destroy must agree

A run's apply and its destroy must use **the same** provider default project.
They did not: the apply was wired to the run-owned project while the destroy
rebuilt its environment from the shared fallback.

That is worse than it sounds. Destroy refreshes and removes resources carrying no
`project_id` of their own — the entire motivating case for this ADR — so it would
have looked for them in the wrong project and either failed the teardown or left
them running. A change made to stop projectless resources being stranded would
have stranded them.

The rule, stated so it cannot be re-derived incorrectly: **every Layer 3
environment for a given run is built from the same project.** It is enforced by a
source audit rather than a unit test, because the defect lives in which helper a
call site picks — and no test of either helper would have caught it.

## Amendment, 2026-08-31 (S165, pass 24): S166 must land before S167

The slice ordering in the plan was a preference. It is a safety requirement.

`CaptureSweepTarget` derives the run's blast radius from the
`scaleway_account_project` resource in state, and its output flows into
`destroySandbox`, which calls `AssertProjectDeletable`. So the moment S167 removes
that resource from the HCL, the sweep target has no source — a successful
apply/destroy would report an orphan-sweep failure, and the auto-created purge
would be skipped for want of a project id. **Removing the resource before
replacing the guard that reads it breaks the guard.**

Hence: **S166 before S167**, not merely S166 then S167.

One consequence of S165 landing first, stated so nobody mistakes the flag for
finished: enabling `create_run_project` *before* S167 gives a run **two**
projects — the pre-created one and the one its HCL still declares. Nothing leaks;
the pre-created one is empty and deleted by the cleanup, and the declared one is
destroyed and swept as always. But it is waste, and the flag is mechanically
complete rather than coherent until S167.

## Amendment, 2026-08-31 (S165, pass 25): released in one place

The run project's cleanup was written three times and skipped by some exit path
twice: first on the happy path only, then inside the destroy branch, which
`--no-destroy` and disabled destruction both walk past.

It now sits outside every branch, immediately before the result is assembled.
**A resource created in one place is released in one place** — every attempt to
attach the release to a particular outcome produced a path that escaped it.

## Amendment, 2026-08-31 (S165, pass 26): two rules the cleanup needed

**"No failures" is not "nothing is left."** A `--no-destroy` run succeeds with an
empty failure list while its resources are deliberately still up, so a clean
result alone must never authorise deleting the run project. Deletion requires
that nothing was created, or that destruction actually ran and the account came
back clean.

**Cleanup runs on a fresh context.** Passing the run's own context to the delete
meant a cancelled run — Ctrl-C, a timeout — skipped cleanup entirely, leaving the
project behind on exactly the runs that most need it. The same reasoning the
interrupt guard already applies to its destroy: the point of cleanup is to do
work after cancellation.

## Amendment, 2026-08-31 (S165, pass 27): validate before you create

The run project is created only after the sealed environment validates. Creating
it first meant a configuration certain to be rejected — a missing
`SCW_ACCESS_KEY` — still produced a real project, with cleanup left to best
effort.

The rule generalises past this ADR, and is most of what this slice's review
passes actually found: **an operation with a side effect goes after the checks
that can refuse it**, not before them with cleanup as compensation.

## Amendment, 2026-08-31 (S165, pass 28): the teardown's verdict, not the command's

Whether to delete the run project is a question about **the account**, and only
the destroy-and-sweep outcome answers it. Keying the decision off the command's
accumulated failure list meant a mock criteria failure — which says nothing about
Scaleway — stranded an empty project forever after a demonstrably clean teardown.

Across nine review passes on this slice, the condition on *whether* to delete was
correct from the first version. Where and when it ran was wrong five times. That
is the shape of defect this arc keeps producing, and it is worth expecting rather
than rediscovering.

## Amendment, 2026-08-31: no transition — supersedes the migration decision and the pass-24 ordering

This supersedes two earlier parts of this record: the **Migration** paragraph in
the original Consequences ("both models must work during the transition"), and
the **pass 24** amendment titled "S166 must land before S167". Both are wrong,
and left in place above only because an ADR records what was decided when.

The question that changed it: *what is the benefit of a transition for a
non-production tool like this?* There is none. No fleet, no external consumers,
nobody mid-migration. The only thing a transition protected was the PR gate's
fixtures — and the gate runs on every PR, so breaking them is caught there.
Rollback in a single-user repo is `git revert`.

It also cost something. Two models means two code paths, and two code paths is
where this arc's defects came from: five of S165's nine review findings were a
cleanup path that did not run, and the dual model produced the "two projects per
run" wart this ADR had to document. **`scaleway.create_run_project` was
scaffolding mistaken for a feature.**

So there is no ordering between S166 and S167, because they are **one change**:
the guard's input changes at exactly the moment the HCL changes. The cutover
lands the new guard, the shape gate without its `scaleway_account_project`
requirement, the prompts, the pitfalls, the fixtures and the recorded generation
together — reviewed hard and canaried before merge, with the flag deleted.

Design and the four decisions behind it:
`docs/plans/s166-teardown-guard-design.md`.
