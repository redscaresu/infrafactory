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
project **as a Terraform resource**. (Resolved 2026-08-31 by ADR-0025's cutover,
which stops it being one; this paragraph records the position as it stood.) **No Scaleway compute scenario satisfies both today** — not
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

## Amendment, 2026-08-30 (S153b): the fixes needed fixing

The review of S153a found 15 further issues, several of them regressions S153a
introduced. Three change what this ADR promises.

**"Already destroyed" is not evidence the account is clean.** S153a added a
shortcut: an empty state means destroy already ran, so release the record. That
laundered a *failed orphan sweep* into a green result — destroy succeeds, the
sweep finds orphans and fails before release, and the next pass sees an empty
state and retires the record without ever re-running the sweep. The orphans then
exist with nothing tracking them, which is the leak class this whole ADR exists to
prevent. The sweep is now re-run against the project id the record carries, and
the record is released only if it passes. **Rule: releasing requires positive
verification, never the absence of contrary evidence.**

**Rule 3 needs an escape hatch, not just detection.** Making undecodable records
reapable (S153a) was half the fix: they are reapable *by design* and unreclaimable
*by nature*, so they failed every pass forever while `live teardown` could not
even load them. `live forget` releases a record without destroying or verifying
anything — the operator asserting they have dealt with the resources by hand. It
says exactly what it gives up, and preserves the unparseable bytes beside the
released record rather than overwriting them, because a record truncated mid-write
often still contains the project id someone would need to finish the job.

**Interrupting an apply must not corrupt its state.** The guard added in S153a
cancelled a context wired into `exec.CommandContext`, whose default cancel sends
**SIGKILL** — stopping `tofu apply` between a resource being created and its state
being flushed, which produces exactly the untracked resource the guard existed to
prevent. It now sends SIGINT and bounds the wait, because tofu forks provider
plugins that inherit the output pipes and would otherwise hang the parent
indefinitely.

The pattern across all three: **a safety mechanism added in haste reproduced the
failure it was written to prevent.** Each was caught by review, not by tests — the
suite was green for every one of them.

## Amendment, 2026-08-30 (pass 10): releasing requires positive verification

The empty-state release path added in S153b was still unsafe, and both an
independent Codex review and a Claude review found it. It rebuilt the sweep
target from the record's project id alone, with **no strays**. Strays are
resources found *outside* the run project, computed by `CaptureSweepTarget` from
state that destroy has since emptied — so where the first sweep failed on a
stray, the re-verification found only that the project was gone, reported clean,
and released the record while the stray kept billing untracked.

`Deployment` now carries a sticky `SweepVerificationFailed`. It is set the moment
a sweep fails and checked before the empty-state path, which refuses rather than
re-verifying something it cannot see. **The rule this makes explicit: a record is
released on positive verification, never on the absence of contrary evidence.**
Where verification is impossible, the operator is pointed at `live forget`.

`live forget` gained the guard it should have had on arrival. It took any id and
released it with no state check, so pointing it at a healthy deployment — a
mistyped but existing id — made `Reapable()` false forever and left a project
billing with nothing that would ever destroy it. **An escape hatch for
unreclaimable records became the sharpest way to create one.** It now refuses
anything teardown can still handle.

Method note, recorded because it changed the outcome: the three review passes
before this one used Claude's `/code-review` skill, not the Codex loop AGENTS.md
requires. A same-family reviewer shares the blind spots that produced the defect,
which is the most plausible explanation for three consecutive rounds in which a
safety fix reproduced the failure it targeted.

## Amendment, 2026-08-30 (pass 11): an escape hatch must accept what the guard refuses

Three further corrections, each closing a gap the previous fix opened.

**A guard that cannot record itself is not a guard.** The sticky
`SweepVerificationFailed` marker was written with `_ = store.Put(d)`. A failed
write left the flag false, so the next pass would take the empty-state path and
release anyway — silently undoing the protection. A failed marker write is now a
teardown failure in its own right, and says not to expect a re-run to refuse.

**`teardown` and `forget` must not point at each other.** Teardown refuses a
sticky empty-state record and names `live forget` as the way out, but
`reclaimable` counted any record with a state file as teardown's business — so
`forget` bounced it straight back. The operator had no CLI escape at all. The
rule this makes explicit: **whenever a guard refuses and names a remedy, the
remedy must accept exactly that case**, and it is worth a test that walks the
pair rather than each half alone.

Pass 12 found the same loop one class along: a record that decodes but carries no
`project_id` counted as reclaimable purely because a state file existed, so
`forget` refused it while teardown failed at `AssertProjectDeletable`.
`reclaimable` now asks what teardown actually requires rather than whether a file
happens to be present — which is the general form of the rule above.

**Removing a hazard must not remove the escape.** Dropping `WaitDelay` avoided
turning successful applies into `ErrWaitDelay`, but it also removed any bound on
a `tofu` that ignores SIGINT — which would hang `deploy` before registration,
recreating the unrecorded-live-resource path the signal guard exists to close.
The kill fallback is now armed inside `Cancel`, so it is scoped to cancellation
and a normal exit cannot trip it.

## Amendment, 2026-08-31 (S154): the record carries what the service did

`live observe` probes each live deployment's health path once and appends an
`Observation` to its record. Four choices that this ADR should hold, because
each one is a place a later slice could quietly get it wrong:

- **`unhealthy` and `unreachable` are distinct statuses.** "It answered and said
  it is broken" and "we got no answer" are different facts about the world.
  S156's promotion gate groups observations by normalized detail, so collapsing
  them here would put two different failures in one bucket and teach the wrong
  lesson from both.
- **A probe that could not run records nothing.** A malformed address or a
  missing port is a failure to *observe*, not an observation of failure. It is
  reported as a command failure and no `Observation` is written, so the
  reproduction gate never counts it.
- **Port and health path are snapshotted at deploy time**, alongside the address
  they belong with. The scenario file changes; the record describes one
  deployment that already happened, and an observation attributed to a health
  path that deployment never had is worse than no observation.
- **Observations are a bounded ring** (`MaxObservations`). A permanently broken
  deployment emits one per probe for as long as it lives. Without a cap the
  record becomes an append-only log that grows until the disk does — the
  unbounded-signal risk the learning-loop plan names, arriving one slice earlier
  than expected.

One probe per invocation and no retries, deliberately: the existing
`RealProbeHarness` retries because it runs seconds after an apply, whereas this
runs out-of-band and a retry would smear over exactly the flapping it exists to
notice. Scheduling stays the operator's cron, as `live reap`'s does.

`observe` re-reads a record immediately before writing to it. It is the command
most likely to run on a cron, and therefore the one most likely to be mid-probe
when an operator runs `live teardown`; a read-modify-write spanning a slow probe
would restore `state: live` over a record that had just been released. The
re-read narrows that window to the microseconds around one write. It does not
close it — the store has no compare-and-swap — and this record should not be
read as claiming otherwise.

A live deployment that **cannot** be probed is a failure, not a skip. `observe`
originally skipped a record carrying no address or no port, and exited zero —
excused as an artefact of records written before S154. That was wrong about
where the case comes from: `registerDeployment` captures the address
best-effort, so an apply that succeeded without producing a load balancer
address writes exactly that record today. Reporting it as a skip meant the
command said "all is well" about a deployment it had just admitted it could not
see. Only a **released** deployment skips, because only there is nothing left to
observe.

## Amendment, 2026-08-31 (S155a): the record states intent, not fact

`deploy` records the image and tag a scenario **declares**. That is a claim about
what was asked for, and the 2026-08-31 canary measured the gap: the record said
`nginx:1.27` while the instance served `python3 -m http.server`.

A scenario may declare `service.version_path`, which `live observe` probes
separately from health. Three decisions worth holding:

- **Three states, not two.** `unchecked` means nothing was asked; `unconfirmed`
  means the service answered and did not confirm. Treating the first as
  confirmation is precisely the falsehood this exists to prevent, and a probe
  that fails is `unchecked` — claiming a contradiction on evidence nobody
  gathered is the same error inverted.
- **The check is weak on purpose, and says so.** The response must *mention* the
  tag. That verifies a cooperating service and cannot verify an uncooperative
  one, which is why the path is opt-in rather than assumed.
- **A mismatch fails a healthy deployment.** A service that answers perfectly
  while running something else is the more dangerous case, because nothing else
  in the system will notice.

This is S155's prerequisite: an upgrade to a version nobody confirmed is running
proves nothing.

The evidence rule is asymmetric, and pass 47 found the slice breaking it.
**Finding** the tag in a truncated body proves the tag is there; **not** finding
it proves nothing, because the rest was never read. So a partial or unreadable
response is `unchecked`, and only a complete one can produce `unconfirmed`.
Every `unchecked` carries the reason it was not checked — "no version_path
declared" is a lie for a declared path that could not be reached, and that
distinction is the whole point of having three states.

## Amendment, 2026-08-31 (S155b): an apply is not an upgrade

`live upgrade` rolls a deployment onto new configuration in place — same project,
same workdir, so the address survives.

**infrafactory does not produce the new HCL.** It owns applying it safely: into
the project the deployment already owns, under the same deny-by-default checks a
first deploy runs, and with proof afterwards that the version changed. This
composes with however the configuration was produced, and keeps the generator out
of the live path.

Not parameterised through `TF_VAR`, deliberately. `SandboxStripEnv` removes
`TF_VAR_*` because the cost bounds read a variable's **default** to decide blast
radius; an injected variable would make those checks vouch for a number that never
reaches the API. Handing over whole HCL keeps every check applying to the
configuration actually applied.

Three rules:

- **A successful apply is not an upgrade.** Terraform reaching its desired state
  says nothing about whether the service restarted, pulled the image, or ran its
  user data. Reporting an upgrade on the strength of the apply is the same error
  as trusting the record over the service, one step later.
- **An upgrade refuses to start from a contradicted version.** Rolling forward
  from a version the service denies would record a transition that never happened.
  Unchecked is permitted and stated; contradicted is not.
- **The superseded configuration is kept** in `.infrafactory-previous/`, one
  generation deep. `ExtractFixPitfall` produces prescriptive rules by diffing a
  failing configuration against a passing one, and a live failure has no "next
  iteration that fixed it" — but an upgrade has a before and an after, which is
  the same shape. Discarding it would leave live signals at the weakest class of
  lesson.

Two corrections from pass 50. The record advances onto the new tag only when the
apply **ran**: a failure during apply may have changed a great deal, but a failure
at init or plan changed nothing, and advancing there would make the record claim a
version that was never deployed. And the address is re-read from state after the
apply, because replacement HCL can recreate the load balancer — verifying against
the address captured at first deploy would probe infrastructure the deployment no
longer owns, and leave every later observation pointed there too.

Pass 51 added two more consequences of the same question. `--from` may not name
the deployment's own workdir or anything inside it: the superseded configuration
is removed before the new one is read, so that would leave the workdir empty while
the infrastructure kept running. And when nothing reached the cloud, the rejected
configuration is reverted from `.infrafactory-previous/` — otherwise the workdir
describes something that was never applied.

The tag, the address and the workdir contents all hang off one predicate: **did
anything reach the cloud?** Three separate answers to that question is how the
first version got two of them wrong.

`live upgrade` requires `validation.layers.sandbox_deploy.enabled`, as `deploy`
does. A gate on one entry point into real infrastructure and not the other guards
nothing.

And the ordering is load-bearing: every fallible step that does not touch the
workdir runs **before** the destructive swap. Three review passes each found a
different early return leaving unapplied configuration behind; the fix is not a
fourth rollback path but removing the opportunity for one. **Ordering beats
compensating.**

The project an upgrade applies into comes from the **marker**, not the deployment
record, and a disagreement between them is refused. The record is the half a stale
or edited file can change, and this call applies real infrastructure into whatever
it names — applying deserves at least the care destroying gets (pass 37 for
teardown, pass 53 for upgrade).

Verification distinguishes an upgrade from a no-op. Without a new tag the record
still names the old version, so confirming it shows the service is unchanged
rather than upgraded; reporting otherwise would be a green built from checking
that nothing changed.

The marker is **required** for an upgrade, not merely preferred: falling back to
the record when it cannot be read leaves the editable half deciding where real
infrastructure is applied. `live teardown` does fall back, deliberately — refusing
there strands a pre-cutover record whose resources are real, and destroy is bounded
by the state in its own workdir. Neither argument holds when applying, and an
operator who cannot upgrade can still tear down and deploy again.

The version comparison respects version boundaries. A plain substring match
confirms tag `1.2` against a service reporting `nginx/1.27.4` — a service running
something other than what the record claims, reported as confirmation, which is
the precise drift this check exists to catch. A match may not be flanked by digits
and may not directly follow a dot: `1.27` is confirmed by `1.27.4`, `1.2` is not,
and `11.27` does not confirm `1.27`.

`live upgrade` also re-reads its record before writing, as `observe` does. It
holds that record across a real apply — minutes, not the microseconds a probe
takes — so a teardown finishing in that window would otherwise be overwritten and
the deployment resurrected. A record released mid-apply produces a loud failure
naming the project, because whatever the apply created is not tracked by a
released record. The window is narrowed, not closed: the store has no
compare-and-swap.

Re-reading a record before writing is only half the rule: the write must go onto
the **fresh** copy, not the one loaded earlier. `live upgrade` holds its record
across an apply that takes minutes, and `live observe` on a cron appends
observations in exactly that window. Writing back the stale copy discarded them —
which does not corrupt the record, it quietly weakens the learning signal S156's
promotion gate counts. An upgrade merges only the three fields it owns (`Tag`,
`UpgradedAt`, `Address`); anything else belongs to whoever wrote it last.

## Amendment, 2026-09-01 (S158): the record is the interface, so the record is what gets tested

The live commands do not call each other. They are coupled through the
deployment record: `deploy` writes it, `observe` appends, `upgrade` rewrites three
fields, `teardown` releases it, `reap` acts on what it finds.

Every command's own tests build the record they read, which is why three defects
in this arc were interactions no unit test could see — `observe` failing on an
address-less record `deploy` legitimately wrote, `upgrade` discarding
observations appended during its apply, and the record and marker disagreeing
about which project an apply may touch.

The journey test runs the real commands and the real `livestore`, with only the
cloud faked, and asserts what each step **leaves for the next one**.

It cannot run against a mock, and that is worth recording so nobody tries:
`assertRealScalewayEndpoint` refuses any Layer 3 apply not pointed at
`api.scaleway.com`, checking the passed env, the inherited `SCW_API_URL` and the
`scw` config file. Every command that builds a sandbox env inherits the refusal.
Pointing a test at a mock would mean weakening that control, and the control is
right — a green Layer 3 result must not be able to be evidence of nothing.

Faking the cloud rather than the record is also the stronger choice: every defect
this test exists to catch lives in the record, not in the API.

An observation records **why** it was adverse, including a version mismatch.
`live observe` originally put the version detail only in its failure summary, so a
healthy-but-wrong-version observation reached the record as `healthy` with an
empty detail and the reason vanished when the command exited. It is written onto
the observation now, unless a health failure already claimed the field — a service
that is both down and misreporting its version should say the more urgent thing
first.

This matters beyond readability: the promotion gate groups on that detail, so
without it the most dangerous shape live observation can find — the service and
the record disagreeing while every other signal reports success — could never
become a candidate lesson.

## Amendment, 2026-09-01 (S157a): reconcile-against-API, delivered

Rule 3's outstanding promise is kept. `live reconcile` compares the
organization's projects against the live store and reports both directions:
stamped projects no record explains, and records naming projects the API says do
not exist.

**The gap was worse than "not built yet".** `livestore.go` stated, as fact, that
the reaper "reconciles against the API rather than trusting this file alone."
Nothing did — `live reap` calls `store.Reapable()` and never contacts Scaleway.
A comment asserting a guard that does not exist is worse than no comment: it
makes the hole invisible to the next person who reads for it. That comment now
describes what is true, and says explicitly that the reaper is store-driven.

### It reports; it never destroys

An unrecorded project is *by definition* something this system's records do not
explain, and destroying what you cannot explain is how a reconciler becomes the
incident. The command prints project ids and a human decides.

### Precise rather than heuristic, because of the stamp

Since ADR-0025 every deployment owns a project named `if-run-…` and carrying a
fixed description. Reconciliation is therefore a set difference over a stamp
infrafactory itself wrote, using `IsInfrafactoryRunProject` — the *same*
predicate that guards teardown, so "infrafactory created this" has one definition
rather than one here and another in the guard. A project without the stamp is
never considered in either direction.

### Three ways it refuses rather than reporting clean

Every one of these renders as "nothing unaccounted for" if handled the easy way,
which is the false green S139 exists to prevent:

- **missing credentials** — the cloud would read as empty and every deployment
  would look accounted for;
- **an unreachable API** — same, and an error is not an empty organization;
- **more than 50 pages** — a truncated estate is a partial answer presented as a
  complete one, so `List` fails instead of returning what it got.

A **released** deployment still accounts for its project. Teardown records the
release, but this ADR's unreclaimable case is exactly a project that outlives it,
so ignoring released records would send an operator to investigate something the
store already explains.

### Verified against real Scaleway

Not against a fake. An empty stamped project was created through the Account API,
left unregistered, confirmed present in `List` with the stamp read correctly,
confirmed flagged as unrecorded by `Reconcile` against an empty store, and
deleted. **Cost: nothing** — Scaleway projects are free; only resources bill.

The baseline that run printed is worth recording: **3 projects in the
organization, 0 carrying infrafactory's stamp.** The account holds no leaked run
projects, so the D6 fix is holding.
