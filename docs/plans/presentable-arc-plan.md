# Presentable: making infrafactory demonstrable on stage (S144–S150)

Planned 2026-08-23. Driver: a conference talk — *"Letting Agents Change
Production Without Breaking It"* — with a **live PR-gate demo on stage**, 2–6
weeks out.

The arc is scoped by what the talk must be able to *show*, not by internal
tidiness. Anything that does not make the demo work or a claim defensible is
out of scope.

## The gap this arc closes

The abstract makes four claims. Three are already true:

| Claim | Status |
|---|---|
| `plan` proves intent, not safety | **true, with evidence** — 3 canary runs, 4 defects mocks could not produce |
| software factory (generate → validate → test) | **true** — 44 scenarios, 4 clouds, auto-learning |
| real-cloud digital twin: plan → apply → destroy | **partial** — 2 of 16 Scaleway scenarios (`docs/layer3-coverage.md`) |
| "before it ever reaches a production repo" | **false today** — `scenario-gate.yml` gates PRs on *mocks*, and only for scenario-file changes |

The last row is the talk's centre and the obvious hostile question: *"so does it
actually gate your PRs against a real cloud?"* Today: no. This arc makes the
answer yes, and puts a bot comment on a PR to prove it.

## Design decision: keep the LLM out of the live path

The talk has two claims and they want different treatment on stage.

- **"Agents generate faster than you can review"** — record it. Generation is
  40–60s with real variance; the S143 canary hit a transport failure mid-run.
  A live demo blocked on an LLM call is a coin flip.
- **"The twin verifies against the real world"** — run it live. With fixed HCL
  in the PR, the gate is deterministic and fast: `block-paris` completed Layer 3
  in ~9s, `lb-paris` in ~74s.

So the live artifact is: open a PR → gate applies to real Scaleway → destroys →
sweeps → comments the stage summary. Generation is the recorded prologue.

## Slices

| id | slice | why it matters to the talk |
|---|---|---|
| S144 | Real-cloud PR gate + comment | **the demo.** Without it the central claim is aspirational |
| S145 | "Plan lied" corpus | the intellectual core: proof, not assertion |
| S146 | Coverage widening (cost-bounded) | upgrades "accepts TCP" to "serves traffic" |
| S147 | Evidence + numbers | the metrics slide, honest |
| S148 | Demo harness, rehearsal, fallback | live demos die without one |
| S150 | Supply-chain + CI hardening | a talk about safely letting agents change prod invites scrutiny of your own pipeline |
| S149 | Talk-support docs + close-out | the repo survives the audience reading it |

S144 is the spine; S145 and S148 depend on it. S146 and S147 are independent.

## Standing rules

Inherited, restated because this arc spends real money and will be read by
strangers.

- **Codex review loop on every PR**, two consecutive clean passes (AGENTS.md).
- **Layer 3 runs as `infrafactory-layer3`**, never the owner key (ADR-0023).
- **Deny-by-default allowlist** — widening it is a costed decision, per slice.
- **No euro figures invented.** ADR-0010 deferred cost estimation; provisioning
  class and measured wall-clock are the honest units.
- **Nothing claimed on a slide that is not demonstrable in the repo.** The talk
  is a promise the code has to keep.

---

## S144 — Real-cloud PR gate

### Motivation

`scenario-gate.yml` already blocks merges on LLM-pipeline convergence, so the
scaffolding exists — it just runs Layer 2. This extends the pattern to Layer 3
and makes the result visible on the PR.

### Tickets

| id | detail | priority |
|---|---|---|
| S144-T1 | `.github/workflows/layer3-gate.yml`: on a PR carrying the `layer3-gate` label, run plan → apply → destroy → sweep against real Scaleway. Label-gated, not path-gated, so cost is opt-in per PR and the demo is triggerable on cue. | P0 |
| S144-T2 | Post the `StageSummary` back as a PR comment, updating in place rather than appending on re-run. Must render the same stage list the CLI prints — the demo's visual payload. | P0 |
| S144-T3 | Credentials via repo secrets bound to the `infrafactory-layer3` application key. Never the owner key; the workflow must fail closed if the org id is absent. | P0 |
| S144-T4 | Hard timeout and guaranteed cleanup: if the job is cancelled or times out, `reap` runs. A gate that leaks on cancellation is worse than no gate. | P0 |
| S144-T5 | **Fork-PR safety. Read this before writing the workflow.** The gate holds real Scaleway credentials and executes HCL from the PR, so the threat is credential exfiltration by anyone who can open a PR against a public repo. Three things are each necessary and none is sufficient alone: **(a)** the real-cloud path runs **only for same-repo PRs** — fork PRs get the existing mock-only gate and no secrets; **(b)** secrets live behind a **GitHub Environment with required reviewers**, so every run needs explicit approval and approval does not carry over to new commits; **(c)** the job records the head SHA it was approved for and **re-verifies it at run time**, aborting if the PR moved. | P0 |
| S144-T5a | Two patterns that look safe and are not, called out so nobody reaches for them: **`pull_request_target` is not a boundary** if the workflow then checks out or executes the PR's code — it grants base-repo secrets precisely to a job running attacker-controlled content. And a **maintainer-applied label is not a boundary on its own**: labelling is a moment in time, the PR can be updated afterwards, and the workflow would then run new code under the old approval. Label for *intent*, gate on *identity and SHA*. | P0 |
| S144-T6 | Measure and record end-to-end wall-clock from label to comment. Feeds S147 and determines whether the live demo is viable. | P1 |

### Exit criteria

1. Labelling a PR produces a real apply/destroy and a comment, unattended.
2. Cancelling the job mid-apply leaves nothing behind, proven once deliberately.
3. Wall-clock from label to comment is recorded and under three minutes.

---

## S145 — The "plan lied" corpus

### Motivation

The talk asserts `plan` proves intent, not safety. Today that rests on defects
found in infrafactory's *own harness*. The talk needs cases where a
**generated infrastructure change** passes `validate`, passes `plan`, passes the
mock layer, and is then rejected by the real cloud.

Each case must be reproducible on demand — a demo that depends on catching a
flake is not a demo.

### Candidate classes

The failure must occur in an **allowlisted** type, or infrafactory's own
allowlist stops it first — which is a different (also good) story, but not this
one.

| class | mechanism | reproducible? |
|---|---|---|
| Invalid commercial type | LB or volume type the zone does not offer; the provider does not validate at plan | yes, deterministic |
| Global name collision | a name already taken; plan cannot know | yes, seed it first |
| IAM scope | a type inside the allowlist but outside the policy (`scaleway_domain*` today) | yes, zero setup |
| Post-create consistency | resource exists, provider errors on read-back | **no** — observed once, never reproduced; do not build the demo on it |

### Tickets

| id | detail | priority |
|---|---|---|
| S145-T1 | Build 2–3 scenarios or fixed-HCL fixtures, one per reproducible class, each with a captured `plan` output that is clean and green. | P0 |
| S145-T2 | For each, capture the real-apply failure verbatim. The contrast slide is the plan output beside the API error. | P0 |
| S145-T3 | Record honestly which classes a *mock* could have caught. Some could be; the argument is that the mock did not, and the real apply did. Overclaiming here is the easiest way to lose a technical audience. | P0 |
| S145-T4 | Add to `docs/layer3-real-vs-mock-deltas.md` as the standing evidence record. | P1 |

### Exit criteria

1. At least two classes reproduce on demand, ten times out of ten.
2. Each has a clean `plan` and a real failure, captured side by side.
3. The honest limits of the argument are written down, not glossed.

---

## S146 — Coverage widening, cost-bounded

### Motivation

`docs/layer3-coverage.md`: 2 of 16 runnable, and the cheap pool is empty.
Widening is deliberate. The talk needs `http_probe` — the difference between
*"the load balancer accepts TCP"* and *"the load balancer serves traffic"* is
the difference between a plumbing demo and a convincing one.

### Tickets

| id | detail | priority |
|---|---|---|
| S146-T1 | Add `registry-paris`: `RegistryFullAccess` on the policy plus the allowlist entry. Cheapest expansion, instant provisioning. | P1 |
| S146-T2 | ~~Grant `InstancesFullAccess`~~ **DONE 2026-08-23** — granted by explicit decision, verified (`201 Created` on an Instance IP), recorded in ADR-0023 along with what it costs: `openclaw-prod` is protected by software again rather than also by the API. `IPAMFullAccess` is likely still needed for private networking (see `docs/layer3-coverage.md`) and was **not** granted — add it only when a scenario actually needs it. | P0 |
| S146-T3 | Upgrade `lb-paris` (or a new scenario) to `http_probe` with a backend that actually serves. | P0 |
| S146-T4 | Re-run the coverage audit and refresh `docs/layer3-coverage.md`. | P1 |

### Exit criteria

1. A real load balancer serves a real HTTP 200 to a real probe.
2. ADR-0023 records the widened policy and why.

---

## S147 — Evidence and numbers

### Tickets

| id | detail | priority |
|---|---|---|
| S147-T1 | Collate every defect Layer 3 caught that Layer 2 and unit tests did not — four so far, with the mechanism of each. | P0 |
| S147-T2 | Measure: wall-clock per gate run, scenarios covered, defects per real run. Report the denominator too; three canary runs is a small sample and saying so is stronger than implying a trend. | P0 |
| S147-T3 | Cost per gate run in provisioning terms plus observed duration. No invented euro figures. | P1 |

---

## S148 — Demo harness and rehearsal

### Tickets

| id | detail | priority |
|---|---|---|
| S148-T1 | `make demo-gate`: one command that opens the PR, applies the label, and tails the result — so the on-stage path is one keystroke, not five. | P0 |
| S148-T2 | Recorded fallback of a full successful run, good enough to narrate if the venue network fails. **Non-negotiable for a live demo.** | P0 |
| S148-T3 | Rehearse end to end at least three times, on a network resembling the venue's. Record failures and fix them. | P0 |
| S148-T4 | Pre-flight checklist: credentials valid, quota headroom, account clean, mockway running, fallback recording to hand. | P0 |

---

## S149 — Talk-support docs and close-out

| id | detail | priority |
|---|---|---|
| S149-T1 | README section a stranger can follow after seeing the talk: what this is, what it proves, what it does not. | P0 |
| S149-T2 | An explicit "why only Scaleway" note — GCP, AWS and Genesys bake endpoints into the provider block, so the same-HCL dual-apply contract does not hold. Pre-empts the hostile question and is more credible than glossing. | P0 |
| S149-T3 | Arc close-out: ARCHIVE section, `STATUS.md`, `docs/NEXT_SESSION.md`. | P0 |

---

## S150 — Supply-chain and CI hardening

### Motivation

Two reasons, one external and one internal. A talk about letting agents change
production safely invites the audience to look at *your* pipeline — and the
repo is public, so they can. And S144 adds a workflow that holds real cloud
credentials and writes to PRs, which raises the stakes on everything around it.

Prompted by a comparison against `redscaresu/goldfinger`. The honest result is
that **infrafactory is already ahead on most measures** — both repos have
gitleaks with a `.gitleaks.toml` allowlist, `go test -race -count=2`,
`-trimpath` builds, `SECURITY.md`, and dependabot. This slice is the small
genuine delta, not a wholesale import.

### Tickets

| id | detail | priority |
|---|---|---|
| S150-T1 | **`govulncheck` job in CI.** goldfinger has one; infrafactory has none, so a known-vulnerable Go dependency currently ships silently. `go run golang.org/x/vuln/cmd/govulncheck@latest ./...`. The single clearest gap. | P0 |
| S150-T2 | **Adopt goldfinger's secret-guard pattern for S144.** Its `e2e` job already solves exactly the problem S144-T5 describes: a job doing real external mutations that needs a secret, guarded so it skips cleanly and green when the secret is absent — including on fork PRs, where secrets are unavailable. Reuse the shape rather than reinventing it. | P0 |
| S150-T3 | **`permissions:` blocks on every workflow.** `ci.yml` and `doc-hygiene.yml` have none, so `GITHUB_TOKEN` gets the default scope. Least-privilege matters more once S144's workflow needs `pull-requests: write` to comment — that workflow should hold that permission and no other, and the rest should hold `contents: read`. | P0 |
| S150-T4 | **Pin third-party actions to commit SHAs.** Both repos use mutable tags (`actions/checkout@v7`). A tag can be repointed at malicious code by a compromised upstream, and this pipeline holds cloud credentials. Pin at least the workflows that can see secrets; `dependabot` already knows how to bump SHA pins. Neither repo does this today — it is a shared gap, not a goldfinger import. | P1 |
| S150-T5 | Record the comparison in `docs/` so "did we copy goldfinger's security setup?" has a written answer, including the measures infrafactory already had. | P2 |

### Exit criteria

1. `govulncheck` runs on every PR and fails on a known vulnerability.
2. Every workflow declares an explicit `permissions:` block.
3. Any workflow that can see cloud credentials pins its actions by SHA.

## Out of scope

- **Multi-cloud real apply.** Structurally blocked; needs per-layer
  provider-block rendering and its own ADR. The talk says so instead.
- **Gating every PR by default.** Cost. The gate is label-triggered.
- **Cost estimation in currency.** Still deferred per ADR-0010.
- **New clouds, pitfall-pruning, the AGENTS/README sweep.** Unrelated to the
  talk; they can wait.

## Risks

| risk | mitigation |
|---|---|
| Live demo fails on stage | S148-T2 recorded fallback, rehearsed three times |
| Real cloud latency makes the demo drag | keep the LLM out of the live path; `block-paris` Layer 3 is ~9s |
| Widening Instances weakens the `openclaw` protection | taken knowingly 2026-08-23; recorded in ADR-0023. The allowlist still excludes `scaleway_instance_*`, so two gates remain and only one moved |
| An audience member asks about multi-cloud | S149-T2 makes it a slide, not a stumble |
| Gate leaks resources under cancellation | S144-T4, proven deliberately once |
| Fork PR exfiltrates cloud credentials | S144-T5/T5a: same-repo only, Environment with required reviewers, run-time SHA re-verification. The repo is public, so this is an open invitation if got wrong |

## Fresh-context checklist

1. `AGENTS.md` § "Codex review loop" and § "Scaleway Bootstrap"
2. This plan
3. `docs/layer3-coverage.md` — what can run, and what widening costs
4. `docs/decisions/0023-*` — sealed env, orphan verification, credential scope
5. `docs/layer3-real-vs-mock-deltas.md` — the existing evidence
6. `.github/workflows/scenario-gate.yml` — the pattern S144 extends
