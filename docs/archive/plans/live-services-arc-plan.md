# Live services: deploy something that stays up, and learn from it (S151–S158)

Planned 2026-08-30. Driver: infrafactory can build infrastructure and prove the
cloud accepted it. It cannot yet run a **service** — something with a version,
serving traffic, that outlives the run that created it — and so it cannot learn
from anything that only goes wrong *after* apply succeeds.

## The gap this arc closes

Every `infrafactory run` today ends `destroy → sweep`. That is deliberate and it
is what makes the project's safety claims true. It also means the learning loop
has exactly one input: `failure.Detail` strings raised inside a run's stages
(`internal/cli/run_command.go:226`, `:618`, `:650`). Infrafactory can therefore
learn only one kind of lesson — *the cloud refused this at apply time*.

An entire class of real failure is structurally invisible to it:

| Failure class | Visible today? | Why not |
|---|---|---|
| API refuses the resource | **yes** — this is Layer 3's whole job | — |
| Healthy at apply, degraded 40 min later | no | nothing exists 40 min later |
| Health check flaps under real traffic | no | one probe, then destroy |
| Upgrade v1 → v2 drops connections | no | there is no v2; there is no v1 |
| `plan` reads clean but takes the service down | no | nothing is up to take down |
| Drift: console edits, provider-side mutation | no | state is discarded each run |

Rows 2–6 are the failures that actually page people. This arc makes them
producible, observable, and — the point — **learnable**, by feeding them into
the pitfall machinery that already exists.

## What "live service" means here, concretely

Bounded by the Layer 3 allowlist (`infrafactory.yaml:95-122`), which is
deny-by-default with k8s, RDB and Redis deliberately commented out. The largest
stack infrafactory may build on real Scaleway is `lb-serving-paris`:

    scaleway_account_project  →  scaleway_instance_ip
                              →  scaleway_instance_server   (DEV1-S)
                              →  scaleway_lb_ip
                              →  scaleway_lb                (LB-S)
                              →  scaleway_lb_backend
                              →  scaleway_lb_frontend       (:80)

So: **one small instance behind a load balancer, running a versioned app, that
stays up.** No Kubernetes, no managed database. Opening those is a separate
costed decision and explicitly out of scope.

Today that instance serves `python3 -m http.server 80` written by cloud-init
(`examples/layer3-gate/lb-serving-paris/main.tf:19-24`) — no name, no version,
no build. That placeholder is what this arc replaces.

## Design decisions

Four, taken up front. They are the load-bearing ones; the rest is mechanics.

### 1. The app is a public image pinned by tag

`nginx:1.27` → `nginx:1.28`. A registry of our own (Scaleway Container Registry,
instance-side credentials, a build pipeline) is an arc in itself and is not the
thing being learned about. A pinned public tag buys the only property that
matters here — **version identity** — for free, which is what makes *upgrade* an
event that can succeed or fail. Swapping to a first-party image later is a
one-field change in the scenario.

### 2. Live deployments get their own projects; the ephemeral invariant is untouched

The orphan sweep, `no_orphans`, `AssertProjectDeletable` and the entire
blast-radius story rest on *nothing survives, the account ends clean*. That
claim is not weakened here, it is **scoped**: a live deployment creates its own
`scaleway_account_project` and keeps it, recorded in the live registry.
Ephemeral per-run projects continue to be destroyed and swept, and the sweep's
meaning does not change by one word.

The rejected alternative was teaching the sweep to discriminate live resources
from strays via a registry lookup. That makes sweep correctness depend on
registry accuracy, so a stale entry becomes a silent leak — precisely the
failure class D6 and the S144/S146 work kept surfacing. Not worth it.

### 3. TTL is mandatory and fail-closed

A scenario that asks to persist without an expiry is **refused at validation**.
There is no default that means "forever". The reaper ships in the same slice as
persistence itself (S153) — shipping the ability to leave things running without
the thing that stops them is how this project has historically leaked money.

Default 4h. Scaleway's list prices (read 2026-08-30, before tax, PAR-1) put the
`lb-serving-paris` shape at **€0.042/hour** — DEV1-S 0.00898, LB-S 0.023, two
IPv4 at 0.005 each — so **€0.17 per 4h TTL, €1.01 per day, €30 per 30 days**.
Sources: [virtual-instances](https://www.scaleway.com/en/pricing/virtual-instances/),
[network](https://www.scaleway.com/en/pricing/network/). Recorded in ADR-0024
with the one ambiguity that remains (whether the first IPv4 per asset is
bundled) called out rather than smoothed over.

That changes the design's centre of gravity: the binding constraint on TTL is
**exposure and forgetting, not money**. A month of uptime costs about €30. A
deployment nobody remembers is the real risk, which is what the reaper is for.

### 4. Live failures enter the learning loop through the existing extractors

The pitfall extractors take a failure-detail string. A soak or upgrade failure
that emits in that same shape inherits `NormalizeDetail`, oscillation detection,
holdout checks, dedup and YAML append **unchanged**. This is what makes the arc
tractable rather than a second learning system: the new work is producing the
signal, not consuming it. Live-sourced entries are tagged `source: live` so they
are distinguishable from `descriptive` / `fix` / `avoid` (ADR-0019 vocabulary).

## Canary, 2026-08-30: the persistence path, proven against real Scaleway

Run with **seeded HCL to isolate the harness** — this repo's S143 pattern. The
`lb-serving-paris` fixture was used as `web-live-paris`'s generated output, so
any failure would be S151–S153 code rather than generation.

| step | result |
|---|---|
| `deploy` | success, **35.8s**, project `4bfdb8db-…` created and recorded |
| `live ls` | live, TTL counting down from 4h, address `62.210.39.190` |
| real API | 1 instance, 1 load balancer; recorded address **matched** the real LB IP |
| HTTP through the LB | **200** |
| `live teardown` | success, **36.1s**, destroy + sweep + released |
| account afterwards | back to 3 projects; canary project returns **404**; LB IP no longer answers |

Verified against the API directly rather than by trusting the command's own
report, because a green teardown that leaves a project behind is exactly what a
self-report cannot catch.

**D6 reproduced itself in the new path, and the purge caught it:**

    sandbox_deploy/auto_created_purge: pass (destroy was blocked by 1 resource(s)
      the API created but Terraform did not own: security_group 9508dfd3-…
      (Default security group) in fr-par-1)

Scaleway auto-created the security group on the first Instance, `tofu destroy`
could not delete the project while it existed, and the S146 purge-and-retry
cleared it. The teardown would have read as an ordinary clean run without that
line — which is why the purge reports what it removed rather than fixing things
silently.

**What this canary does not prove.** The HCL was seeded, so nothing here says a
`service:` block generates correct infrastructure. The instance served
`python3 -m http.server` while the record claimed `nginx:1.27`: **`deploy`
records the declared image without verifying what is running.** That gap is real
and belongs in S155, where upgrade makes it load-bearing — an upgrade to a
version nobody confirmed is running proves nothing.

Run 2 — the same path with the LLM generating — is blocked behind the private-NIC
question (mockway answers the `v2alpha1` private-network-interfaces endpoint with
501, and Layer 2 gates Layer 3).

## Slices

| id | slice | why it matters |
|---|---|---|
| S151 | Live deployment registry + TTL vocabulary + `live ls` | bookkeeping first: nothing can persist safely before there is a record of what is out there |
| S152 | `service:` artifact in the scenario schema + generation | gives the app a version identity; without it "upgrade" is not expressible |
| S153 | `deploy` / `teardown` **+ the TTL reaper** | the first slice where something stays up — and the thing that stops it, in the same PR |
| S154 | Soak observation stage | the first genuinely new input to the learning loop |
| S155 | Upgrade / rollout (v1 → v2 against a running service) | the richest signal class, and the one closest to the talk's thesis |
| S156 | Live signals → pitfalls (`source: live`) | closes the loop; until this lands the arc produces logs, not learning |
| S157 | Exposure hardening + cost accounting | a 24/7 public instance is a different security and money proposition than a 144s one |
| S158 | Evidence doc + close-out | what live running caught that ephemeral runs could not, with mechanisms |

**S151 → S152 → S153 is the spine and is strictly ordered.** S154 and S155 are
independent of each other and both depend on S153. S156 depends on either. S157
can land any time after S153 and should not wait for the end.

**S153 must not merge without its reaper.** Stated twice on purpose.

## Standing rules

Inherited, restated because this arc spends real money continuously rather than
in 144-second bursts.

- **Codex review loop on every PR**, two consecutive clean passes (AGENTS.md).
- **Layer 3 runs as `infrafactory-layer3`**, never the owner key (ADR-0023).
- **Drift becomes a failed test** — the project's established pattern. The
  registry/reaper invariants get audit tests, not written rules.
- **No euro figures** until someone reads the price list (S147).
- Every slice that can leave real resources behind states, in its PR, what it
  verified against the API afterwards.

## Risks

| risk | mitigation |
|---|---|
| **Continuous billing.** Ephemeral runs cost ~2 min of resources; a live service costs 24/7 — €0.042/hour for this shape. | mandatory TTL, reaper in the same slice, measured cost accounting in S157 |
| **A leak now looks like a feature.** "Still running" is the intended state, so the signal that used to mean *bug* now means *normal*. | the registry is the only source of truth for what may persist; anything live and unregistered is a leak and fails |
| **24/7 public attack surface.** | S157; :80 on the LB only until then |
| **Reaper fails silently** and things run forever. | reaper reports what it removed — the D6 lesson: a fix that fires invisibly is indistinguishable from a coincidence |
| **Registry and reality diverge.** | reconcile-on-read against the API, and an audit test for the divergence case |

## Out of scope

Kubernetes, managed databases, Redis, private networking/IPAM, a first-party
container registry, multi-region, autoscaling, TLS certificates, and any cloud
other than Scaleway. Each is a costed decision, and none is needed to prove a
versioned service can be deployed, observed, upgraded and learned from.
