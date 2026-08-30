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
not treated as free.

The default TTL is 4h. That number is now backed by Scaleway's published list
prices rather than by intuition (read 2026-08-30, before tax, PAR-1):

| component | €/hour | source |
|---|---|---|
| `DEV1-S` instance | 0.00898 | [virtual-instances](https://www.scaleway.com/en/pricing/virtual-instances/) |
| `LB-S` load balancer | 0.023 | [network](https://www.scaleway.com/en/pricing/network/) |
| IPv4, instance | 0.005 | [network](https://www.scaleway.com/en/pricing/network/) |
| IPv4, load balancer | 0.005 | [network](https://www.scaleway.com/en/pricing/network/) |

So the `lb-serving-paris` shape costs **€0.042/hour**: about **€0.17 per 4h TTL**,
**€1.01 per day**, **€30 per 30 days**. If Scaleway bundles the first IPv4 per
asset rather than charging for it — the price list says "Additional IP address",
which is ambiguous on this point and has not been confirmed against an invoice —
the figures fall to €0.032/hour, €0.13 per TTL and €23 per 30 days.

The consequence is that 4h is conservative to the point of being cheap, and the
binding constraint on TTL is exposure and forgetting, not money. A deployment
left running for a month costs roughly what one takeaway does; a deployment
left running and *forgotten* is the actual risk, which is what the reaper is
for. S157 replaces this table with measured spend rather than list price.

## Amendment, 2026-08-30 (S152): the service spec, and why a moving tag is refused

`scenario.schema.json` gains a top-level `service:` block — `image`, `tag`,
`port`, `health_path`, `ttl` — with `image`, `tag`, `port` and `ttl` required.
Its presence makes a scenario a live-service scenario; its absence leaves every
scenario that predates S152 untouched.

Two rules follow from the decisions above rather than being new policy.

**TTL lives in the schema, not in a flag.** Rule 2 says an expiry is mandatory
with no unbounded form. Putting it on the service spec means a scenario cannot
*declare* a service without saying how long a deployment of it may live, and the
JSON schema enforces that for callers which validate without decoding — the UI's
real-time validation path included. `MaxServiceTTL` is 168h. That is a typo
control, not a cost control: at roughly EUR 0.042/hour a week is small money, but
`400h` written where `4h` was meant should fail at validation rather than on an
invoice.

**A moving tag is refused.** `latest`, `stable`, `edge`, `main`, `master`,
`nightly` and similar are rejected by the loader. Version identity is the only
property this arc needs from an image: without it an upgrade cannot be
distinguished from a no-op, and a soak failure cannot be attributed to a version
— which would make the entire S154–S156 signal meaningless. This is honest about
its own limits: it catches the common case and is not a proof of immutability. A
numeric tag such as `1` also moves, and only a digest (`@sha256:...`) is
genuinely fixed. Digest pinning is worth doing and is deliberately not done here.

**Consequence for `docs/layer3-coverage.md`.** That document counts
`**runnable**` rows as scenarios that *have run*. `web-live-paris` is ungated but
has never been run, so recording it as runnable would have made the document
claim a real-cloud run that never happened — the S147 failure class, caught by
`TestLayer3CoverageDocTotalsMatchItsTable` rather than by review. The table gains
a fourth bucket, `runnable, unrun`, enforced by that same test. A row is promoted
to `**runnable**` on the day it goes green and not before.

## Amendment, 2026-08-30 (S153): deploy, teardown, and the reaper

The slice where something actually stays up. Per rule 2 the reaper ships with it,
not after it.

**`deploy` does not generate.** `run` proves a change is safe — generate,
validate, mock apply, real apply, destroy, sweep. `deploy` takes what `run`
already validated and keeps it. Splitting the verbs means deploy cannot become a
route around the layers, and it keeps "prove this is safe" separate from "put it
up", which is the distinction the whole project is about. The allowlist is
nonetheless re-checked at deploy time: *already validated* is a claim about a
previous command, and anything that may touch the cloud is checked
deny-by-default at the point it does.

**Every deployment gets its own workdir.** `<live root>/workdirs/<id>`, never the
shared `output/<scenario>`. Two deployments of one scenario would otherwise share
a single `terraform-live.tfstate`, and the second apply would overwrite the
first's — orphaning real resources with nothing left that knows how to destroy
them. This is the same class of defect as D6: a leak that no cost check would
show, because the resources are real but unowned.

**Registration happens on the failure path too.** An apply that dies partway
leaves real resources and a real project. The record is the only thing that will
bring the reaper back to them, so it is written from whatever the state shows
rather than only when the apply returns success. An apply that created nothing
writes no state and is recorded as a skip — there is nothing to reap.

**The registry is not authority for what gets destroyed.** It says *which*
deployment; the state file in that deployment's workdir says which project, and
`AssertProjectDeletable` is given both so a disagreement is fatal. A stale,
hand-edited or tampered record therefore cannot aim teardown at a project it did
not create, including the organization default.

**A deployment whose state has vanished is not released.** Its resources may
still be running, and marking it released would retire the only record that says
so — turning a visible problem into an invisible one. It stays reapable and is
reported as unreclaimable, with the project id, so a human can finish the job.

Still outstanding from rule 3's promise: reconcile-against-API. The reaper today
trusts that the store knows every live deployment. A wiped working directory
loses records while the cloud keeps the resources, and nothing yet detects that.
It is the largest remaining hole in this ADR and belongs in S157 with cost
accounting.

## Amendment, 2026-08-30 (S153a): what the audit found, and what rule 3 actually required

PR #170 merged without a review pass. A post-merge audit returned 15 findings,
several of them leaks in code whose job is to destroy real infrastructure. The
corrections belong here because two of them were **this ADR's own rules, stated
and not implemented**.

**Rule 3 was prose, not behaviour.** "Unreadable is expired" promised that a
record which will not decode is *reported as expired so the reaper takes it
down*. `Reapable` filtered over decodable records only, so such a record never
entered the set, and `MarkReleased` decoded before writing, so it could not be
cleared either — it failed every pass forever with no way out. `List` now returns
undecodable records as deployments marked `Undecodable`, and `MarkReleased`
replaces one it cannot read with a minimal released record. The original
regression test covered only decodable-but-invalid records, which is why the gap
survived.

**Fail-closed was not applied to `--dry-run`.** `live reap --dry-run` returned
early, discarding failures already recorded for unreadable records: it exited 0
while something that might be running was unaccounted for, and skipped the output
contract entirely. It now finishes through the same path as a real run.

**The blast-radius argument had a hole in the id.** Deployment ids were
second-resolution, so two deploys of one scenario within a second shared a record
path *and* a workdir — the second apply adopted the first's state and left a
project running with nothing that knew how to destroy it. Ids now carry entropy,
and `copyDeploySource` refuses a workdir that already holds live state. The
per-deployment workdir was the right decision; the id defeated it.

**Registration on the failure path did not cover signals.** The claim that a
record is written "whether or not the apply succeeded" held for a returned error
and not for SIGINT, which killed the process outright with the project already
created. `deploy` now runs its apply under a signal guard — and, unlike
`run`/`test`, it **records rather than destroys** on interrupt: deploy's purpose
is that things stay up, so an interrupted deploy is left to its TTL rather than
torn down under the operator.

**Two further corrections of fact.** A relative store root made a scheduled
reaper in another working directory report "nothing has expired" and exit 0;
roots are now absolute. Writes were `os.WriteFile` (truncate-then-write), which
manufactures exactly the truncated record rule 3 concerns itself with; they are
now temp-file-plus-rename. Ids are validated against path traversal, because they
are derived from scenario names the schema does not constrain and are also taken
straight from the command line.

**And the private-networking blocker was misdiagnosed — twice.**

The repo recorded it as `IPAMFullAccess`. A canary asked the real API, which
wanted `write compute_private_networks` — `PrivateNetworksFullAccess`, granted
2026-08-30. Private *networks* now create. Private *NICs* still cannot, and no
permission grant will fix it: `scaleway_instance_private_nic` takes no
`project_id`, so the provider creates it in the default project — which is
`scaleway.fallback_project_id`, the ADR-0010 containment project — while the
server lives in the run's own project, and the API refuses the mismatch.

The second misdiagnosis was mine, and it went the other way. I concluded
`vpc_required.rego` was "wired for AWS only" because it is absent from
`constraint_policies`, narrowed `pitfalls/scaleway.yaml` on that basis, and wrote
the claim into four files. It is false: `constraint_policies` is a different
mechanism, and `filterPolicyPathsByCloud` drops only *other* clouds, so every
`policies/scaleway/*.rego` runs against a Scaleway plan. A generation run the same
day failed on exactly that rule. All of it is retracted, and the pitfall is
restored.

**What is actually true is worse than either diagnosis: the two gates
contradict each other.** Layer 1 requires a private NIC on every
`scaleway_instance_server`; Layer 3 cannot apply one while the run creates its own
project. **No Scaleway compute scenario satisfies both today** — not
`web-live-paris`, not any other. This is a design question, not a permission or a
pitfall, and it is the real blocker on the live-services arc reaching real
infrastructure with generated HCL.

Options, none yet taken: give the NIC an explicit `project_id` if a newer provider
supports one; relax `vpc_required` for scenarios that declare no private
networking; or accept that scenarios creating their own project cannot use private
networking and say so in the policy. **The method lesson is the cheaper one: both
misdiagnoses came from reading configuration and inferring, and both were settled
in minutes by running the thing.**

The standing correction: the review loop runs on every PR **before** merge. Every
one of these 15 findings coexisted with a fully green test suite.
