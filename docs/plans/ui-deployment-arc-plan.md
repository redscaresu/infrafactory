# Driving a deployment from the UI (S159–S164)

Planned 2026-08-30. Driver: `deploy`, `live ls` and `live teardown` exist and are
proven against real Scaleway (canary, 2026-08-30) — but only from a terminal. The
goal is to take the basic scenario from mock all the way to a running service
**from the UI**, in one journey a person can watch.

This arc is a **presentation layer over S151–S153**, not new machinery. That is
what makes it tractable; it is also what makes its risks concentrate in one
place, which is the safety model rather than the code.

## The gap this arc closes

| capability | CLI | API | UI |
|---|---|---|---|
| generate / validate / mock apply | ✅ | ✅ `/api/runs` | ✅ |
| real apply + destroy (Layer 3) | ✅ | ✅ (run flag) | ✅ |
| **deploy and keep** | ✅ | ❌ | ❌ |
| **see what is running** | ✅ `live ls` | ❌ | ❌ |
| **tear one down** | ✅ `live teardown` | ❌ | ❌ |
| **reap expired** | ✅ `live reap` | ❌ | ❌ |

The bottom four rows are the arc. Everything above them already works.

## Design decisions

### 1. The estate does not live at `/live`

`/live` is already the live **run console**. A deployments page there would
collide with it and confuse the one journey this arc exists to make clear. The
estate goes at **`/deployments`**.

### 2. The safety model is designed before the button exists (S160)

The UI is an unauthenticated web app bound to localhost. Adding
`POST /api/deployments` turns it into something that creates public
internet-facing infrastructure and spends money **with no credential check**, and
any page in the user's browser can issue a POST to localhost. That is a
materially different proposition from a CLI command someone types deliberately.

This is not a reason to abandon the arc; it is a reason to decide the model
first. Candidates, to be settled in an ADR rather than in a pull request:

- an explicit config opt-in (`ui.allow_deploy: false` by default), so the
  capability does not exist unless someone turned it on;
- an origin/CSRF guard, since a drive-by POST is the realistic attack;
- a typed confirmation naming the scenario, so a click cannot do it alone;
- refusing to bind anything but loopback while deploy is enabled.

### 3. Deploy is a distinct action from run, in the UI as in the CLI

`run` proves a change is safe; `deploy` keeps it. S153 deliberately split those
verbs so deploy could not become a way around the layers. The UI must not
re-merge them into one "go" button — the whole point is visible, and the
distinction is the thesis the talk rests on.

### 4. Cost and TTL are shown before the click, not after

Every deploy confirmation states what it will create, what it costs
(**€0.042/hour** for this shape — DEV1-S + LB-S + two IPv4, list prices read
2026-08-30), and when it expires. A UI that hides the cost of a button is worse
than a CLI that never mentioned it, because the CLI at least required typing.

## Slices

| id | slice | why it matters |
|---|---|---|
| S159 | `/api/deployments`: list, deploy, teardown, reap — plus **reconcile-against-API**, pulled forward from S157 | the spine; a button makes the reconcile hole worse, so it lands with it |
| S160 | Deploy safety model + ADR | decided before the capability exists, not after |
| S161 | `/deployments` estate page — TTL countdown, clickable address, per-row teardown, unreadable-record banner | the "what is running" view; carries fail-closed visibility into the UI |
| S162 | Deploy from the scenario page, with a confirmation showing cost, TTL and blast radius | the button |
| S163 | Stream deploy/teardown over the existing websocket | 36s of silence reads as broken; init → plan → apply → register should scroll |
| S164 | Playwright e2e over the whole journey, then one real run | the demo path, tested rather than rehearsed |

**S159 → S160 → S162 is the ordered spine.** S161 depends only on S159. S163 can
land any time after S162. S164 is last by definition.

## Refresh, 2026-09-01: what S154–S156a changed

This plan was written on 2026-08-30, before live observation and upgrade existed.
Two changes, and the first matters more than the arc's own contents.

### S164's e2e moves out, into S158

S164 is *"Playwright e2e over the whole journey, then one real run."* That was
the **only** end-to-end coverage of the live lifecycle anywhere, and it sat at
the end of an arc that has not started — so every CLI command shipped in
S151–S156a has no journey test underneath it.

Pulled forward as **S158** (`docs/plans/live-lifecycle-e2e-plan.md`), which
covers the CLI journey against mockway plus one real pass. S164 keeps its
Playwright half: the UI journey still needs testing, and now it can sit on top of
a CLI journey that is already pinned rather than being the first thing to pin it.

### The estate page has more to show than TTL and address

S161 was specified before a deployment could be **observed**. A page built to the
original description would show what is running and stay silent on whether it is
actually serving — the exact gap S154 and S155a exist to close, reproduced in the
UI.

`livestore.Deployment` now carries observations, and `live ls` already renders a
`HEALTH` column. S161 should show, per deployment:

| field | why the UI specifically needs it |
|---|---|
| last observation status | `healthy` / `unhealthy` / `unreachable`, and **`unobserved`** — silence must not read as healthy |
| version check | `confirmed` / `unconfirmed` / `unchecked`, distinctly. A record claiming `nginx:1.27` while the service says otherwise is the more dangerous state *because it looks fine* |
| `upgraded_at` | a deployment that was rolled forward is a different thing from one that never moved |

**`unobserved` and `unchecked` are the load-bearing states.** A UI that renders
them as blank cells rebuilds the falsehood the three-state design exists to
prevent — that nobody having looked is the same as nothing being wrong.

### Not added: a deploy-time version_path field

`service.version_path` is opt-in and belongs in the scenario, which the UI
already edits as YAML. Adding a dedicated control would be a second place to set
one thing.

### Why reconcile-against-API moves into S159

ADR-0024 always promised it and S153 did not deliver it: the reaper trusts the
live store to know every deployment, so a wiped working directory loses the
records while the cloud keeps the resources. A CLI user does that rarely. A UI
invites repeated deploys from people who have never heard of
`.infrafactory/live`, and the failure is silent. The hole gets worse exactly when
this arc succeeds, so it closes first.

## Standing rules

- **Codex review loop on every PR**, two consecutive clean passes (AGENTS.md).
- The estate page must show unreadable records as loudly as `live ls` exits
  non-zero for them. "We could not check" and "nothing is running" must not look
  alike in a UI either.
- No euro figure that has not been read off a price list (S147).

## Risks

| risk | mitigation |
|---|---|
| **An unauthenticated local endpoint that spends money.** | S160, before any button exists |
| **A UI makes leaking cheap.** Repeated deploys, no memory of the store. | reconcile-against-API in S159; estate page makes the count visible |
| **The demo becomes the product.** Building for the stage rather than for use. | every slice is a real capability; none is demo-only scaffolding |
| **Credentials absent at runtime.** The UI process needs `SCW_*`. | deploy control disabled with a stated reason, never a failed request after the click |

## Out of scope

Authentication and multi-user access, remote (non-loopback) serving, deploying
anything but Scaleway, and editing a live deployment in place. Upgrade is S155's
job in the live-services arc, not this one's.
