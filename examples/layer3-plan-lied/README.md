# The "plan lied" corpus

Infrastructure changes that pass `tofu validate`, pass `tofu plan`, pass a
**mock apply**, and are then refused by real Scaleway.

Each case is the same HCL run twice — once against mockway, once against
`api.scaleway.com`. The only variable is which API answers.

| case | validate | plan | mock apply | real apply | reproduces |
|---|---|---|---|---|---|
| `commercial-type` | pass | clean, `2 to add` | **2 added** | refused | 10/10 |
| `iam-scope` | pass | clean, `2 to add` | **2 added** | refused | 10/10 |

Captured output for all three stages of both cases is in `evidence/`.

## commercial-type

A block volume asks for `iops = 9000`. Every layer that reads the
configuration is satisfied: it is a valid integer in a valid field. The real
API enumerates what it will actually sell you.

    Error: scaleway-sdk-go: invalid argument(s): perf_iops is required, The
    provided 'perf_iops' value is not available. If provided, please choose
    from the following available values: [5000, 15000].

**Could a mock have caught this? Yes.** The set `[5000, 15000]` is static, and
a higher-fidelity mockway could enforce it. This case is a mock fidelity gap,
and saying otherwise to an audience would be dishonest.

The argument is not that it was uncatchable. It is that the gap is *unbounded*:
this is one enumerated field on one resource, and closing it by hand does
nothing for the next one. The real API knows its own catalogue for free.

## iam-scope

A DNS zone for a domain nobody here owns. `scaleway_domain*` is inside the
deploy allowlist, so infrafactory's own check passes it through; the
credential is not permitted to create one.

    Error: scaleway-sdk-go: http error 403 Forbidden: permission denied

The mock's answer to the identical configuration:

    scaleway_domain_zone.app: Creation complete after 0s [id=gate.infrafactory-demo.com]
    Apply complete! Resources: 2 added, 0 changed, 0 destroyed.

The mock issued a DNS zone for a domain that is not ours, in zero seconds,
to a pipeline that was never allowed to create one.

**Could a mock have caught this? No — not without becoming the real cloud.**
The refusal is not a property of the configuration. It is a property of the
IAM policy attached to the key that happens to be running the apply. Two
identical plans, two different keys, two different outcomes. To predict it, a
mock would have to hold a faithful replica of your live IAM state, at which
point it is no longer a mock.

This is the stronger of the two cases, and it is the one to lead with.

## A refuted candidate

Duplicate **project names** were expected to collide, and do not — Scaleway
permits two projects with the same name in one organization. The apply
succeeded. Recorded here because a corpus of near-misses is only trustworthy
if the misses are in it too.

## Running them

    cd commercial-type
    tofu init && tofu plan      # clean
    tofu apply                  # refused

Real credentials, real API, no `SCW_API_URL`.

**`iam-scope` only reproduces with a credential that is not permitted to
create DNS zones** — that is the whole point of the case. Run it with an
organization-owner key and the apply behaves differently. Use the dedicated
restricted Layer 3 application key the captured evidence was produced with;
do not run this one under a default profile.

The Layer 3 key is deliberately never granted Domains permissions, so this
fixture stays valid as the policy widens for other resource types.

`commercial-type` has no such dependency. The API refuses `iops = 9000`
whoever asks, so it is the safer of the two to demo live.

Both cases create their own disposable `scaleway_account_project`
(ADR-0010), so a failed apply leaves a project and nothing billable;
`tofu destroy` removes it.
