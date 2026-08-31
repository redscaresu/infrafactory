# S168: the cutover canary

Run 2026-08-31 against real Scaleway from branch `s166-cutover` at `b9b4227`,
after codex passes 30–37. Three applies, `fr-par-1`, roughly €0.005 total.

The claim being tested is narrow: **the run's project is now created through the
Account API before the apply and destroyed after it, and no path depends on
Terraform having owned it.** Only a real run can test that — every mechanism in
it is a behaviour of the API rather than of the configuration.

| # | Scenario | Result | Elapsed |
|---|---|---|---|
| 1 | `block-paris` | **pass** | 12s |
| 2 | `lb-serving-paris` | fail — `real_probe` 503 | 113s |
| 3 | `lb-serving-paris` | **pass**, real HTTP 200 through a real LB | 79s |

## What run 1 proved

```
sandbox_deploy/run_project: pass (created 26543d9f… (if-run-block-paris-20260831t142704z)
  before the apply; it is the provider default project for this run)
sandbox_deploy/run_project_delete: pass (deleted 26543d9f…)
sandbox_deploy/orphan_sweep: pass (project 26543d9f… destroyed; no resources left outside it)
```

The fixture declares no `scaleway_account_project` and sets no `project_id`
anywhere — the shape the gate now *requires*. The project is stamped, is the
provider default, is deleted by infrafactory rather than by tofu, and the sweep
reads its blast radius from the marker.

## What run 3 proved: D6 moved, exactly as pass 32 predicted

```
sandbox_deploy/destroy: pass                    <-- destroy no longer fails on the project
sandbox_deploy/auto_created_purge: pass (destroy was blocked by 1 resource(s) the API
  created but Terraform did not own: security_group b82cbd1b… (Default security group) in fr-par-1)
sandbox_deploy/run_project_delete: pass (deleted 2ab4a19e… after purging 1 resource(s)
  the API created but Terraform did not own)
```

Before the cutover the `resource is still in use` 412 landed on `tofu destroy`,
which is why `destroySandbox` purges and retries. Here **destroy passes** — the
project is not its resource — and the 412 lands on the Account API delete
instead, where `releaseRunProject`'s purge unblocks it.

Reproduced on both `lb-serving-paris` runs, so it is the mechanism and not a
coincidence. And it says what it removed: a silent fix and no fix look identical
from the outside, which is how D6 survived unnoticed the first time.

## The one failure, and why it is not the cutover

Run 2's `real_probe` got a 503 from the LB. `sandbox_deploy/apply` reported
`succeeded on attempt 2 (real API returned a retryable error)` — the D1
signature — so the instance was younger than usual when the probe ran, and the
LB had no healthy backend yet. Run 3 passed with no code change.

Not diagnosed from the log: re-run, because a transient and a regression read
the same on one sample.

## The account afterwards

Verified against the API directly, not from the sweep's own verdict:

- `if-run-*` projects remaining: **0** (3 projects total: `default`, `openclaw`,
  `infrafactory` — all pre-existing)
- servers: 1, volumes: 1, load balancers: 0 — all of them `openclaw-prod` and
  its root volume, created 2026-02-21, untouched by the canary.

The account is identical to how it started.

## What this canary does not cover

- `deploy` / `live teardown` / `live reap` were not exercised against real
  Scaleway; the run path was. Their project handling shares
  `releaseRunProject` and its guard, but sharing an implementation is not the
  same as having run it.
- The interrupt guard's cleanup path is unit-tested only.
- One fixture with compute, one without. `lb-serving-paris` has no private NIC,
  so the NIC containment ADR-0025 was written for is proven by the apply
  succeeding with the run's project as provider default, not by this fixture.

## The gate cannot validate this change before it merges

Running the `layer3-gate` label on PR #177 fails at `sandbox_deploy/allowlist`
in 0s, before any API call. **That is structural, not a defect in the change.**

The gate builds its binary from the **base** branch, deliberately — S144-T5a:
`pull_request_target` loads the workflow from base, and the binary must come
from base too, or a same-repo PR could rewrite the checks that are supposed to
be judging it. So the gate ran *main's* shape check, which requires exactly one
`scaleway_account_project`, against fixtures the cutover strips it from.

Any change that **inverts a check living in the trusted binary** is unverifiable
by its own gate until it is merged. The alternatives are all worse: building the
binary from the PR is the exfiltration path the gate exists to close, and a
compatibility window is the dual model this arc dropped. The honest handling is
to know it, say it, and re-run the gate on the next PR after merge.

Nothing leaked. The account was checked directly afterwards: three projects,
all pre-existing.

## What the run did catch: the gate's own cleanup read the wrong signal

The reap step keyed entirely off `terraform-live.tfstate`, and on a missing one
reported:

> tofu may have created resources before it was killed. Nothing records them, so
> reap cannot run. Check the Scaleway account by hand.

Post-cutover that is wrong in the dangerous direction. The project is created
before tofu starts and the marker is written at the same moment, so a run can
own a real project and never write state — an apply failing at preflight, init
or plan does exactly that. **The marker exists if and only if a project does**,
which makes it both the safer signal and the more precise one: it turns "check
the account by hand" into a reap that knows what to remove.

The reap step now checks the marker first. This is the same defect the codex
loop found seven times inside the Go code — a path that had been depending on
`tofu destroy` owning the project without saying so — and it survived fourteen
review passes because it lives in YAML.

## S154 verification: the live lifecycle, end to end against real Scaleway

Run 2026-08-31, same account, ~€0.01. This closes the gap the canary above
named: `deploy`, `live teardown` and `live reap` had **never** touched real
Scaleway — only the `test` path had.

| step | result |
|---|---|
| `deploy web-live-paris --ttl 30m` | **pass**, 36s — project created before the apply, deployment registered with project id, address and expiry |
| `curl` the recorded address | **HTTP 200**, serving |
| `live observe` | **healthy**, recorded against the deployment |
| `live observe` again | second observation appended, ring intact |
| `live ls` | `HEALTH` column shows `healthy`; the released deployment shows `unobserved` |
| `live teardown` | **pass**, 39s — destroy, purge, project delete, sweep |
| account afterwards | 3 projects (all pre-existing), 1 server, 0 LBs — `openclaw-prod` only |

D6's purge fired here too, in `live teardown`, and reported what it removed.

### What this run did NOT verify

Only the **healthy** observation path ran against real infrastructure.
`unhealthy` and `unreachable` are unit-covered and were not reproduced against a
real service, because breaking a live backend on purpose costs more than the
signal is worth at this stage. Worth stating rather than letting the table imply
otherwise.

### And the falsehood the plan predicted, demonstrated

The record says `nginx:1.27`. What is actually serving is
`python3 -m http.server` printing that string, because `ubuntu_jammy` has no
docker and the fixture never installed one. `deploy` recorded the **declared**
image without checking what runs.

This is exactly the attribution failure `live-learning-loop-plan.md` decision 4
warns about — a loop that blamed `nginx:1.27` for a failure here would be
learning a falsehood. It is now demonstrated rather than asserted, and
**verifying the running version stays a prerequisite for S155**, not a nicety.
