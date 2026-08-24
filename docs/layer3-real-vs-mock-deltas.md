# Layer 3: real Scaleway vs mockway behavioural deltas

Observed while running the `block-paris` canary (S143) against
`api.scaleway.com`. These are the differences a mock standing in for the
real API did not reproduce — the fidelity payload of the Layer 3 arc, and
the reason Layer 3 exists at all.

Scope: one scenario, one resource pair (`scaleway_account_project` +
`scaleway_block_volume`), two runs (run 1 seeded HCL, run 2 LLM-generated).
Small sample. Recorded because these are observations from the real API,
not inferences.

> Not to be confused with `docs/mock-gaps.md`, which is a git-ignored
> runtime artifact written by the mock-server-bug classifier. This file is
> hand-authored and committed; the classifier owns that one.

## D1 — A create can fail *after* the resource exists

The highest-value finding, and the one no mock produced.

On one run 2 apply, `scaleway_block_volume` creation returned an error to
the provider **after** the volume had been created server-side. OpenTofu
marked the resource tainted and recorded it with `srn` and `zone` absent,
while the API had both populated:

```
# scaleway_block_volume.app_data has changed
  + srn  = "srn://block.scw.eu/zones/fr-par-1/volumes/46e2ff4a-..."
  + zone = "fr-par-1"

# scaleway_block_volume.app_data is tainted, so it must be replaced
```

A straight re-apply replaced the tainted volume and succeeded, so the
error was transient, not a defect in the generated HCL. The same HCL
applied cleanly on the immediately preceding and following runs.

**Why mockway will not reproduce this, and should not.** mockway creates
atomically and always returns fully-populated computed fields. Teaching it
to inject transient failures is exactly the "more realistic" fault
injection the project has already declined (`feedback_mock_design.md`) —
it would slow the inner generate/test loop to simulate something the loop
cannot act on anyway.

**What was done instead** — tolerated in infrafactory, not in the mock.
`SandboxDeployHarness.Run` now retries apply once (`sandboxApplyAttempts`).
A second apply replaces the tainted resource, which is documented tofu
behaviour, so this is a real fix rather than a hope. It is one retry, not
a loop — a genuinely broken plan should fail fast rather than bill for
every attempt — and it never retries a cancelled context, because on the
interrupt path a retry would create exactly what the operator just asked
us to stop creating. Retries are reported (`succeeded on attempt 2`), not
swallowed: a silent retry looks identical to a clean first run, and the
difference is a real API flapping.

## D2 — Diagnostics were the real blocker

Chasing D1 was harder than it should have been because the failure
surfaced as, in full:

```
- sandbox_deploy/apply check=apply command="sandbox deploy harness" detail="exit status 1"
```

The provider's own message was captured by the harness in
`SandboxDeployError.Apply.Stderr` and then discarded when building the
`FailureSummary`. Layer 2 had solved this long ago
(`mockDeployFailureDetail` appends a trimmed, ANSI-stripped stderr); the
two Layer 3 paths never got the same treatment. The sandbox **destroy**
path had the same hole, which is worse — a failed real destroy is the
orphaned-billing case, and it reported `exit status 1` too.

Fixed in this slice by extracting the shared `stderrFailureDetail` helper
and using it in all four paths. This is a mock-fidelity finding only
indirectly: against a mock, `exit status 1` is cheap to reproduce under a
debugger. Against a real API it costs money and a project-create round
trip, so the first report has to carry the message.

## D3 — The SDK reads `~/.config/scw/config.yaml` even when env is set

Every real plan/apply emits:

```
Warning: Multiple variable sources detected, please make sure the right
credentials are used

SCW_ACCESS_KEY               Active Profile in config.yaml, Environment
                             variable    → Currently using: Environment variable
SCW_DEFAULT_PROJECT_ID       Active Profile in config.yaml, Environment
                             variable    → Currently using: Environment variable
...
```

This confirms two things the sealed-environment design depends on, and
which had never been observed against the real provider:

1. The config file **is** consulted — S139-T4a's concern was real, not
   theoretical. On this machine the default profile's `default_project_id`
   is the `openclaw` project, which holds live infrastructure.
2. Environment variables **win**. Because `sandboxCommandEnv` sets
   `SCW_DEFAULT_PROJECT_ID` to `scaleway.fallback_project_id`, a resource
   that omits `project_id` lands in the disposable `infrafactory` project
   rather than next to real infrastructure. The containment rule
   (ADR-0023, fallback-project amendment) works as designed.

Never visible through mockway, where the endpoint arrives via
`SCW_API_URL` and the profile is irrelevant.

## Verified parity (no gap — do not "fix" these)

Worth recording so a future reader does not re-investigate:

| Behaviour | Real Scaleway | mockway |
|---|---|---|
| Block volume id shape | `fr-par-1/<uuid>` (zone-qualified) | same |
| Project delete refuses a non-empty project | yes | yes (S140) |
| Project create returns the org id | yes | yes |
| `scaleway_account_project` + volume apply order | project first, volume references its id | same |

The zone-qualified id in particular is the kind of wire-shape detail a
mock usually gets wrong; mockway already matched it.

## Timing

Real `init → plan → apply → destroy → sweep` for one project plus one
volume completes in about 9 seconds. Fast enough that Layer 3 latency is
not a reason to keep the canary scope narrow — cost is.

## D4 — Real probes work; the credential surface is wider than block storage

From the `lb-paris` probe canary (2026-08-23), the first exercise of the
real-probe path against anything but a mock.

The path itself is sound: `connectivity` resolved the load balancer's
public IP out of `terraform-live.tfstate` (`scaleway_lb_ip` →
`ip_address`) and opened a TCP connection to it from the machine running
the harness. That is the whole chain the probe layer exists for, and it had
never run for real before.

Two things a mock cannot tell you:

- **IAM is per-service.** A key that creates projects and block volumes
  happily can still fail with
  `scaleway-sdk-go: insufficient permissions: read loadbalancer`. mockway
  authorises everything, so credential scope is invisible until Layer 3
  touches a new service. Expect to widen the key's policy once per service
  the suite grows into, and expect the first failure to look like a code
  bug.
- **`http_probe` needs something behind the load balancer.** `expect:
  reachable` requires an HTTP status below 400, and a Scaleway LB with an
  empty backend answers 503. `lb-paris` declares no compute, so it asserts
  `connectivity` (TCP) instead — the strongest claim the scenario can
  honestly make. Upgrading to `http_probe` means adding a backend that
  serves, which means `scaleway_instance_server`, which is deliberately
  outside the allowlist.

## D5 — `plan` is green, the mock is green, the API refuses

D1–D4 are deltas found in infrafactory's own harness. D5 is the class the
talk actually rests on: a **generated infrastructure change** that clears
every pre-real gate and is then rejected by Scaleway.

Two reproduce 10 times out of 10. Full HCL and captured output for all three
stages live in `examples/layer3-plan-lied/`.

| case | plan | mock apply | real apply |
|---|---|---|---|
| block volume, `iops = 9000` | `2 to add` | `2 added` | `perf_iops ... choose from [5000, 15000]` |
| DNS zone, unprivileged key | `2 to add` | `2 added` | `403 Forbidden: permission denied` |

The honest split between them:

- The **iops** case a mock *could* catch — the valid set is static. It is a
  mockway fidelity gap. The point is that the gap is unbounded, not that it
  was unforeseeable.
- The **IAM** case a mock *cannot* catch without replicating live IAM state.
  The same plan with a different key gives a different answer, so no amount
  of configuration analysis reaches it.

Do not present the first as though it were the second.

**Refuted**: duplicate project names were expected to collide and do not.
Scaleway permits them. Left in the record deliberately.

## D6 — The API puts things in your project that Terraform will not remove

`tofu destroy` cannot delete a project that still contains anything, and
Scaleway puts something in it without being asked: the first Instance in a
fresh project causes a **"Default security group"** to be auto-created there.
It is not in the plan, not in the state file, and not Terraform's to remove.

Destroy therefore ends with

    Error: scaleway-sdk-go: precondition failed: resource is still in use,
    all resources are not deleted

and ADR-0010's disposable project — the thing the entire blast-radius
argument rests on — stops being disposable for **every run that declares
compute**. Nothing billable survives, so a naive cost check reports clean
while projects accumulate one per run.

No mock surfaces this, because the auto-creation is a behaviour of the real
API rather than of the configuration. mockway creates exactly what it is
asked for and nothing else, so the project it never had to delete deleted
fine.

`destroySandbox` now purges what the API auto-created inside the run's
project and retries the destroy once, reporting what it removed:

    - sandbox_deploy/auto_created_purge: pass (destroy was blocked by 1
      resource(s) the API created but Terraform did not own:
      security_group e3e8f632-... (Default security group) in fr-par-1)

Only groups Scaleway marks `project_default` are touched. A security group
the run's own HCL declared is Terraform's to destroy, and purging it here
would hide a real destroy bug.
