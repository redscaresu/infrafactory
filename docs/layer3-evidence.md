# Layer 3 evidence: what real-cloud validation caught, and what it cost

The claim this file supports is narrow and worth stating precisely:
**running a generated infrastructure change against the real cloud catches
a class of defect that static analysis, policy, and a high-fidelity mock do
not.**

Not "mocks are useless" — the mock catches more, faster, and for free, and
it runs first for exactly that reason. The claim is that a residue remains,
and that the residue is not exotic.

## The denominator, first

Everything below comes from a **small sample**, and the sample is the
weakest part of the argument. Say so before the numbers rather than after.

| | |
|---|---|
| Scenarios ever run against real Scaleway | **3** of 17 Scaleway training scenarios, 18 counting the holdout (`block-paris`, `lb-paris`, `lb-serving-paris`) |
| Real applies during the arc's validation | roughly **50**, most of them repeated runs of the same two fixtures |
| Distinct resource types exercised | 8 (`account_project`, `block_volume`, `lb`, `lb_ip`, `lb_backend`, `lb_frontend`, `instance_server`, `instance_ip`) |
| Clouds with a real-apply layer | **1** of 4 (Scaleway only; GCP, AWS and Genesys are mock-only) |
| CI gate runs on real pull requests | **11** |

Three scenarios is not a trend. The defects below are real and reproducible
individually; the *rate* at which they appear is not something this sample
can establish, and no rate is claimed.

## What Layer 3 caught that Layer 2 and unit tests did not

Six findings. Each names its mechanism, because "the real cloud is
different" is folklore and a mechanism is not.

| # | Finding | Mechanism | Could a mock have caught it? |
|---|---|---|---|
| D1 | A create returned an error **after** the resource existed; tofu tainted it with computed fields missing | Non-atomic create in a real distributed API | No — and it should not try. Fault injection was explicitly declined |
| D2 | The provider's own error was captured and then discarded; failures reported as `exit status 1` | Layer 3's four paths never got Layer 2's `stderr` treatment | Only indirectly. Against a mock, `exit status 1` is cheap to debug; against a real API it costs a project round trip |
| D3 | The SDK reads `~/.config/scw/config.yaml` even when the environment is fully set | Credential precedence in the real SDK | No — with a mock the endpoint arrives via `SCW_API_URL` and the profile is irrelevant |
| D4 | A key that creates projects and volumes fine still fails `insufficient permissions: read loadbalancer` | IAM is **per-service** | No — mockway authorises everything |
| D5 | Two configurations that pass `validate`, `plan` **and a mock apply**, and are refused by the API | Enumerated commercial values; IAM policy | One of the two, yes. See below — this is the honest half |
| D6 | The API auto-creates a resource inside the project that Terraform never destroys, so the disposable project leaks | `project_default` security group on first Instance | No — the auto-creation is a behaviour of the API, not of the configuration |

D6 is the one to dwell on. Nothing billable survived it, so every
cost-based check reported clean while a project leaked on **every run that
declared compute**. It was found by running for real, and it was confirmed
by making the fix report itself: the first verification run passed with the
purge firing invisibly, and only the stage line

    - sandbox_deploy/auto_created_purge: pass (destroy was blocked by 1
      resource(s) the API created but Terraform did not own: ...)

distinguished a working fix from a coincidence. A silent fix and no fix
look identical from the outside.

A seventh, in the mock rather than in infrastructure: mockway's `Reset`
never cleared `account_projects`, so one Layer 3 scenario's bootstrapped
project persisted and counted as an orphan against **every scenario that
ran afterwards**. Only a real Layer 3 run creates such a project, so
nothing else could have surfaced it.

### The honest half of D5

Of the two plan-lied cases in `examples/layer3-plan-lied/`:

- The **`iops = 9000`** case a mock *could* catch. `[5000, 15000]` is a
  static set and mockway does not model it. This is a fidelity gap.
- The **DNS zone** case a mock *cannot* catch without replicating live IAM
  state. The same plan with a different key gives a different answer.

Lead with the second. Presenting the first as though it were the second is
the fastest way to lose a technical audience, and the distinction costs
nothing to make.

## What it costs

No euro figures — Scaleway pricing is not codeable (ADR-0010) and an
invented number is worse than none. Cost is stated in provisioning terms
and observed duration.

### Gate wall-clock, measured

Eleven real gate runs on pull requests. The six that went green:

    60s  69s  71s  73s  74s  95s

That is the **whole job** — checkout, Go build, mockway start, apply,
destroy, sweep, PR comment — not the apply alone. The real
`init → plan → apply → destroy → sweep` for `block-paris` (one project, one
block volume) is about **9 seconds** of it.

The five red runs (13s, 19s, 53s, 66s, 66s) were workflow defects during
S144, not infrastructure failures.

### What a run provisions

| Scenario | Resources | Lifetime | Observed |
|---|---|---|---|
| `block-paris` | 1 project + 1×1GB block volume | seconds | ~9s apply→sweep |
| `lb-serving-paris` | 1 project + LB + LB IP + instance + instance IP | ~2 minutes | 143s, 144s, 144s over three runs |

Block volumes are billed by size and these are 1GB. The load balancer and
the instance bill hourly and live for roughly two minutes. The
`lb-serving-paris` figure is three separate end-to-end runs against the
real API, not one lucky one; most of it is the instance booting and the
load balancer's health check marking the backend up, neither of which
infrafactory controls.

**The way this budget gets blown is a leak that runs for days, not a run
that costs more than expected.** That is why D6 matters more than its
"nothing billable survived" surface reading suggests: it leaked a *project*
every time, silently, and a cost-based check would have kept reporting
clean.

## Why only Scaleway

Four clouds have mocks — Scaleway, GCP, AWS, Genesys Cloud. **One has a
real-apply layer.** That is a deliberate stopping point, and the reason is
mechanical rather than a matter of effort remaining.

The whole contract of Layer 3 is that **the same HCL** runs against the mock
and against the real API, with nothing changed but where the requests go. If
the HCL has to differ, the mock stops being a rehearsal for the real thing
and the argument collapses.

Scaleway is the only one of the four where that holds:

| Cloud | How the mock gets the traffic | Same HCL both ways? |
|---|---|---|
| **Scaleway** | `SCW_API_URL` environment variable, honoured natively by the provider | **Yes** |
| GCP | `ensureGcpProviderWiring` injects per-service `*_custom_endpoint` attributes **into the HCL** | No |
| AWS | `ensureAwsProviderWiring` injects an `endpoints { … }` block **into the provider block** | No |
| Genesys | The provider hardcodes `login.<region>.pure.cloud`; the mock needs a TLS MITM CONNECT proxy | No |

For the middle two, the endpoint is *part of the configuration*. The
generated stack that passed against the mock is not the stack you would
apply for real — it carries a `127.0.0.1` endpoint in the provider block —
so "it deployed against the mock" says less than it does for Scaleway.

There is a second reason, and it is the one that actually governs cost.
Scaleway has `scaleway_account_project`: a first-class API object that
*contains* everything a run creates. That makes the blast radius exactly one
object, so teardown is provable — delete the project, then ask the API
whether it is gone (ADR-0010, ADR-0023). AWS and GCP have no equivalent
primitive at that granularity. Tagging conventions and per-resource sweeps
are the substitute, and they fail differently: a sweep that misses a
resource type reports clean.

**The honest framing.** The pattern generalises; one cloud's worth of
evidence is what exists today. Anyone extending this to AWS or GCP should
expect the containment model, not the apply itself, to be the hard part.
