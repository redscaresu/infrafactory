# Take the project out of the HCL (S165–S168)

Planned 2026-08-30. Driver: `policies/scaleway/vpc_required.rego` requires a
private NIC on every `scaleway_instance_server`, and Layer 3 cannot create one —
so **no Scaleway compute scenario satisfies both gates**. ADR-0025 records why
and what to do about it.

The change in one line: **stop creating the run's project in the generated HCL;
create it before the apply and let Terraform use it.**

## Why this is the fix, not a workaround

`scaleway_instance_private_nic` has no `project_id` attribute (verified against
provider 2.81.0), so it is created in the provider's default project. That
default is `scaleway.fallback_project_id` — the shared containment project —
while the server is in the run's own, and the API refuses the mismatch.

Proven on real Scaleway before this plan was written: with the project
pre-created and passed as `SCW_DEFAULT_PROJECT_ID`, and **no
`scaleway_account_project` in the HCL**, a private network + instance + NIC stack
applied cleanly (NIC `available`, confirmed against the API) and destroyed
cleanly. The blocker was never the allowlist, the permission or the pitfall — the
NIC is allowlisted and gate-exempt already. It was *when the project came into
existence*.

## Shape of the change

    before                                  after
    ------                                  -----
    HCL declares scaleway_account_project   infrafactory creates the project
    every resource binds project_id to it     via the Account API, pre-apply
    NIC has no project_id -> fallback       SCW_DEFAULT_PROJECT_ID = that project
    -> API refuses the NIC                  NIC inherits it -> applies
    tofu destroy removes the project        teardown deletes it via the API

## Slices

| id | slice | why |
|---|---|---|
| S165 | Pre-apply project creation + `SCW_DEFAULT_PROJECT_ID` wiring, behind a config flag; both paths work | the mechanism, provably reversible |
| S166 | Replace `AssertProjectDeletable`'s second source of truth | the guard that stands between teardown and real infrastructure — see below |
| S167 | Remove `scaleway_account_project` from generation: prompts, pitfalls, shape gate, fixtures | the HCL change itself |
| S168 | Real-cloud canary on the new path, then delete the old one + evidence | proven before the fallback is removed |

**S165 → S166 → S167 → S168 is strictly ordered.** S166 must land before S167:
once the project leaves the HCL the state no longer names it, and the guard has
to already have its replacement.

## S166 is the slice to be careful with

`AssertProjectDeletable(stateProjectID, targetProjectID, organizationID)` refuses
when the target is the organization default, and when it does not match the
project recorded in `terraform-live.tfstate`. The second check is what stops a
stale or tampered record aiming a destroy at something the run never created.
With the project out of the HCL, **the state stops naming it and that check loses
its input.**

It is replaced by two independent checks, both required, each failing closed:

1. **A run-owned marker** written beside the state when the project is created,
   by the same process that created it — carrying exactly the trust the state
   file used to carry: local, written by infrafactory, never by PR-supplied HCL.
2. **An API-side provenance check**: the project must carry the naming and
   description marker infrafactory stamps on projects it creates. A hand-edited
   marker file therefore cannot point teardown at `openclaw`, because `openclaw`
   does not carry the stamp.

The organization-default refusal is unchanged. This is also the first real piece
of the reconcile-against-API work ADR-0024 has owed since S153.

## Migration, and why both paths coexist for a while

`examples/layer3-gate/*` and `docs/demo/recorded-generation/*` declare
`scaleway_account_project` today, and the PR gate applies that fixture HCL
directly — so flipping the model in one commit would break the gate, which is the
artifact the talk rests on. The pre-created path therefore lands behind a config
flag (S165), fixtures and prompts move over in S167, and the old path is deleted
only in S168, after a real-cloud canary has passed on the new one.

## Risks

| risk | mitigation |
|---|---|
| **The teardown guard is weakened in the gap between S165 and S167.** | strict ordering: S166 lands its replacement before S167 removes the input |
| **Empty projects accumulate** from crashes between creation and apply. | sweep them by provenance marker; they are free but must not pile up |
| **`tofu destroy` no longer removes the project**, so a teardown that stops early leaves it. | delete it via the API after destroy — the same sequence `destroySandbox` already runs for the auto-created security group |
| **The gate fixtures drift** from generated HCL during the transition. | both paths supported until S168; the gate keeps applying fixtures unchanged |

## What success looks like

`infrafactory run web-live-paris` generating HCL that contains a private NIC,
clearing `vpc_required`, applying to real Scaleway, and being destroyed — the
first Scaleway compute scenario to satisfy Layer 1 and Layer 3 at the same time.

## Out of scope

Whether `vpc_required` *should* apply to scenarios declaring no private
networking. This arc makes the policy satisfiable; it does not argue the policy is
right. That question is worth asking separately, and answering it would not remove
the need for this change — a scenario that genuinely wants private networking
still cannot have it today.
