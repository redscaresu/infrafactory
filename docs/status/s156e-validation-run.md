# S156e — the validation run: result

Run 2026-09-06 against real Scaleway. **The experiment reached step 3 of 7 and
stopped there, as the plan said it should.** The result is negative on the bar the
arc set itself, and the plan is explicit that this closes the arc as honestly as a
positive one.

## What was deployed

`web-unversioned-paris` — `web-live-paris` with one field added, `version_path: /`.

The manufactured failure is **healthy-but-unconfirmed-version**, which
`promotion.go` calls "the most dangerous shape live observation can find, because
every other signal reports it as healthy".

An earlier draft manufactured a *missing health path* instead, and it was wrong:
the generated load balancer polls `service_health_path` with `health_check_http`,
so a path the service does not serve marks the backend DOWN and fails the
`http_probe` criterion. That is a terraform-visible failure, not the live-only
class — it would have proved nothing, and it was caught by reading `real_probe.go`
and `loadbalancer.tf` before spending anything.

| signal | result |
| --- | --- |
| `tofu apply` | success, 33s |
| `run_project` | created `if-run-web-unversioned-paris-…` before the apply (ADR-0025 is live on main) |
| load balancer | backend up, frontend serving |
| `http_probe` criterion | **pass** — HTTP 200 at `/` |
| orphan sweep | clean |
| **`live observe`** | **fail — the record claims `nginx:1.27`, `/` does not mention "1.27"** |

The service served `infrafactory layer3 backend`: a 200 that names no version. Every
signal short of the live probe called it green.

## Step 2 — promotion: PASSED

Three consecutive probes on one deployment cleared the gate:

> 1 observation(s) have reproduced (3 consecutive probes, or 2 distinct
> deployments) … persistent across 1 deployment(s), longest run 3 — **version
> UNCONFIRMED, so nothing may be blamed on a tag**

The S155a attributability guard fired exactly as designed.

## Step 3 — attribution: FAILED (falsification condition 2)

`live learn` wrote a rule, and filed it under **`scaleway_lb_ip`**.

That is by design: `resourceForCandidate` attributes a candidate to "the resource
the probed ADDRESS was resolved from", and the address is the load balancer's IP.
The design is honest about its limits — it is the only resource a live probe can
know — but the consequence is that **a lesson about a container image is keyed to
a load balancer IP**.

The corpus is keyed by resource and the generator steers on it. So this rule will
be surfaced when generating `scaleway_lb_ip` — where it is useless — and never when
generating the `scaleway_instance_server` whose `user_data` is the only place the
remedy could go.

It is also **descriptive, not prescriptive**: "An apply reaching its desired state
does not mean the service restarted or picked up the new configuration." There is
no instruction a generator can follow. That is falsification condition 3 as well.

Steps 4–7 (upgrade, re-learn, the generation A/B) were not run: the plan says stop
and report if step 3 is not attributable, and an A/B on a rule filed under the wrong
resource would measure nothing.

## What this says

The machinery runs end to end — deploy, probe, promote, write — against real
infrastructure, first time, with the account clean afterwards. **What it does not
yet do is produce a lesson a generator can act on**, and the reason is structural
rather than a bug: a live probe observes an address, and attribution by address
cannot reach the resource that caused what was observed.

Anything built on "live observation feeds the corpus" should assume that gap is
open until an attribution path exists that can name a resource the probe never
touched.

## A defect the run found that no review would

`live reconcile` reported the deployment we had just legitimately torn down:

> deployment … names project … which the API says does not exist — the record
> outlived its infrastructure

`live teardown` destroys the project and *then* marks the record released, so
**every successful teardown left `live reconcile` permanently non-zero**. The check
could not tell "gone because we destroyed it" from "gone unexpectedly".

Fixed, with the ordering that keeps the two released cases apart: a released record
whose project **still exists** is ADR-0024's unreclaimable case and must stay
Accounted; only a released record whose project is **gone** is the success path. An
existing test caught the first version of the fix, which skipped released records
outright and would have silenced the expensive case.

## Cost

One DEV1-S instance and one LB-S, live for about four minutes. Under €0.01.
Teardown clean: destroy, orphan sweep, and a `auto_created_purge` that removed the
`project_default` security group Scaleway creates and Terraform never owns — the D6
leak, caught and named rather than silently succeeded.

`live reconcile` after the fix: *examined 3 project(s) and 0 live record(s); the
cloud and the store agree.*
