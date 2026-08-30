# ADR-0024: Live deployments are bounded by a mandatory TTL, and unreadable means expired

## Status
Accepted

## Context

Every `infrafactory run` today ends `destroy → sweep`. That promise — *nothing
survives, the account ends clean* — is what ADR-0010's disposable project,
`AssertProjectDeletable`, the orphan sweep and the `no_orphans` criterion all
rest on. It is also why the learning loop has exactly one input shape: a
`failure.Detail` raised inside a run's stages. Infrafactory can learn *the cloud
refused this at apply time* and nothing else.

The live-services arc (`docs/plans/live-services-arc-plan.md`) needs a service
that outlives its run, so that failures which only appear afterwards — degraded
40 minutes in, a health check that flaps, an upgrade that drops connections —
become producible and therefore learnable.

That directly contradicts the invariant above, and the contradiction is the
whole risk. Three things had to be decided before any code could persist.

**Where live resources live.** The alternative to a separate project was
teaching the orphan sweep to discriminate: look up each resource in a registry,
skip the ones declared live. Rejected. It makes sweep correctness depend on
registry accuracy, so one stale entry becomes a silent leak — the exact failure
class D6 produced (a `project_default` security group Terraform never destroys,
invisible because nothing billable survived and cost checks kept reporting
clean) and that S144/S146 kept re-surfacing.

**What bounds the cost.** Ephemeral runs bill for roughly two minutes. A live
service bills continuously, and the failure mode is not a crash but an invoice.

**What happens to a record the tooling cannot understand.** A truncated write, a
hand-edit, a schema change — the state where infrafactory holds a file it cannot
parse that may describe running infrastructure.

## Decision

**1. Live deployments get their own Scaleway project and keep it.** A live
deployment creates its own `scaleway_account_project` (already allowlisted) and
does not destroy it. Ephemeral per-run projects continue to be destroyed and
swept exactly as before. The sweep's meaning does not change by one word; the
*scope* of the "nothing survives" claim narrows to ephemeral runs, and that
narrowing is stated wherever the claim is made.

**2. An expiry is mandatory, with no unbounded form.** `livestore.Deployment`
requires `ExpiresAt`, and `Put` refuses a record without one. There is no
sentinel meaning "forever" and no config key that grants one. The reaper that
enforces expiry ships in the same slice as the ability to persist (S153) — never
a later one.

**3. Unreadable is expired.** A record that will not decode, or that decodes
without an expiry or a project id, is reported as **expired and reapable**, not
skipped and not assumed healthy. `live ls` exits non-zero when any record is
unreadable, and names it.

**4. A deployment that cannot be located cannot be written.** `ProjectID` is
required at validation time, because it is the only handle the reaper has.

**5. Releasing is always recordable.** `MarkReleased` deliberately bypasses
validation. A damaged record is still reapable and its resources are still real;
if recording the teardown required a valid record, the reaper would destroy the
resources, fail to write the outcome, and destroy the same already-destroyed
deployment on every subsequent pass.

## Consequences

**Benefits.** No existing safety claim weakens — the ephemeral invariant is
untouched, and the live estate is bounded by construction rather than by
operator discipline. Every unsafe state fails toward teardown: no expiry, no
project id, corrupt file, unknown state all converge on "reap it". The
learning loop gains a new input class without a second learning system, because
live failures emit the same `failure.Detail` shape the existing extractors
already consume (ADR-0019 vocabulary; live-sourced entries tag `source: live`).

**Tradeoffs.** A leak now resembles a feature: "still running" is a legitimate
state, so the registry becomes the sole authority on what may persist, and its
accuracy matters more than any single file in the project. The store is on local
disk under `.infrafactory/live`, so a wiped working directory loses the record
while the cloud resources remain — which is why the reaper must reconcile
against the API rather than trust the store alone. And an unreadable record
costs a non-zero exit on a listing command, which is mildly annoying and
deliberately so.

**Follow-up.** The reaper and its reconcile-against-API path are S153. Cost
accounting per live deployment and exposure hardening for a 24/7 public instance
are S157 — until then a live service is reachable on `:80` through the load
balancer, which is a different security proposition from a 144-second one and is
not treated as free. The 4h default TTL is provisional: revising it requires
reading real Scaleway pricing, and per S147 no euro figure enters this repo
until someone has.
