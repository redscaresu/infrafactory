# ADR-0040: Whole-Scope Ownership for AWS Layer 3

## Status
Accepted (the AWS counterpart of ADR-0023 rules 3 and 4; ADR-0025 is not carried over)

## Context

Scaleway's Layer 3 owns a project per run: ADR-0025 creates it before the apply, so the blast radius is
one disposable project and Scaleway's refusal to delete a non-empty one is a free orphan detector. AWS
has no such unit. The scope is one dedicated, empty member account (HLD 2026-09-27-aws-web-stack
§ Containment), so a run cannot own a project; it owns the whole account for its duration, proves it
empty afterwards, and the account is never created or closed from code.

The code is in `internal/harness/aws_scope.go` (claim, stamp), `aws_sweep.go` (sweep), `aws_reap.go`
(reap) and `internal/cli/aws_scope_lifecycle.go` (the lifecycle). The planted-leak proof passed on the
real account on 2026-09-29 (`aws-scope-planted-leak-proof`, PR #399; observations in
`docs/layer3/real-vs-mock-deltas.md` D7).

## Decision

1. **The claim is a compare-and-swap with a per-process holder.** It is the SSM parameter
   `/infrafactory/layer3/claim`, taken with `PutParameter` `Overwrite=false`, valued
   `<run id>@<host>:<pid>`. The holder is validated against a fixed shape because it is pasted into
   the recovery command.
2. **A retried put is recognised.** When a put fails for any reason, the stored value decides: our
   holder means our own landed attempt, another value is `ErrAWSScopeClaimed` naming it, and an
   unreadable value is `ErrAWSClaimOutcomeUnknown`, which the caller treats as held. The take ignores
   the caller's cancellation, so an interrupt cannot leave a claim the run never learns it holds.
3. **Release is by value, with a Get-then-Delete ceiling.** `ReleaseAWSClaim` deletes only when the
   stored value is the caller's holder. SSM has no conditional delete, so a claim retaken between the
   Get and the Delete would be deleted; serial runs and operator-driven takeover are the ceiling.
4. **Takeover is only by name.** `reap --take-over <holder>` names the holder it replaces, refuses a
   holder whose process is still running on this host, and then takes the claim as a fresh run does,
   so a third party that took the scope in between is refused, never overwritten.
5. **The claim is taken after the stamp and the default-VPC checks**, in one place, and is released
   in one place, `awsReleaseAfterCleanSweep`, which releases only when the sweep finds the scope
   empty. A test fails if any other code calls `ReleaseAWSClaim`.
6. **One take and one release per test execution.** Every exit after the take goes through the same
   teardown. A run that ends with the claim kept ends the run: the next iteration's take would
   succeed against the run's own claim and build on resources nothing has proven gone.
7. **The stamp** is `/infrafactory/layer3/stamp`, holding the account id. Only the admin writes it,
   by hand at setup; no harness code can (a test audits for a write). Preflight and reap read it and
   refuse without an exact match.
8. **The collection table** is the sweep's fixed list, independent of the allowlist. Each row lists
   its collection in full and names what is in it, except what the account owns by construction (a
   VPC's default group and main route table, terminated instances, deleted NAT gateways, the claim and
   stamp parameters):
   - `instances`, `volumes`, `network interfaces`, `elastic ips`, `security groups`, `vpcs`,
     `subnets`, `internet gateways`, `route tables`, `nat gateways`, `key pairs`, `images`,
     `snapshots`, `launch templates`, `ssm parameters`.
   Real AWS creates an instance's root volume and a NAT gateway's interface; fakeaws models neither,
   so only the real sweep sees a leak of them.
9. **The settle re-polls the whole scope.** While anything is shutting-down, deleting or detaching,
   every collection is listed again, up to 60 polls, then it fails closed. Incomplete pagination
   fails, and a NextToken seen twice fails rather than ending the listing silently.
10. **A token cycle and a 403 fail closed.** A collection that cannot be listed in full fails the
    sweep and returns no strays: "could not check" never looks like "nothing leaked".
11. **`ReapAWSScope` has its gate at the delete site.** Before any request reaches EC2 it proves the
    key is the configured principal in the account, the stamp holds the account, and the caller holds
    the claim. It never touches the claim, so no caller reaches a delete without the gate.
12. **Reap runs in dependency order**, each pair a test: `instances → volumes`,
    `instances → network interfaces`, `instances → security groups`, `instances → subnets`,
    `instances → internet gateways`, `nat gateways → elastic ips`, `nat gateways → subnets`,
    `nat gateways → internet gateways`, `elastic ips → internet gateways`, `images → snapshots`,
    `network interfaces → security groups`, `network interfaces → subnets`,
    `security groups → vpcs`, `subnets → vpcs`, `route tables → vpcs`, `internet gateways → vpcs`.
    A failed item does not stop the rest; the next sweep is the verdict.
13. **The SSM row is report-only.** The key may write and delete only the claim, so reap deletes no
    other parameter: the sweep names it and reap says to remove it by hand.
14. **The IAM policy is tied to the sweep by a test.** `docs/layer3/aws/iam-policy.json` must grant
    the actions the claim, the sweep and the reap send, and no more. A widening fails it.
15. **The SCP is the boundary preflight cannot assert.** It denies every action outside the region
    and every service but EC2, SSM and STS, with one exemption, the organization's account-access
    role, which a test pins. Preflight asserts the default VPC is gone and says it cannot assert the
    SCP; the runbook records it.
16. **Tags annotate and never gate.** The sweep prints the run-id tag beside a stray and treats an
    untagged stray as a leak all the same.
17. **Proof.** 2026-09-29, PR #399: one planted leak per collection named, a held claim refused reap
    by name, reap emptied the account, the release left only the stamp. 19 of the reap table's 20
    mutating actions were sent by the scope's user and none was refused; `DetachNetworkInterface` is
    unreachable because reap terminates instances first.

## Consequences

- A leak fails the run and keeps the claim, so the next run is refused until `infrafactory reap`
  proves the account empty. That is the cost of owning the whole account.
- The claim's Get-then-Delete window is a stated ceiling, not a defence against concurrent runs.
- Adding a sweep row or a reap precedence without updating this ADR fails
  `internal/harness/aws_scope_adr_test.go`.
- Still unproven on real AWS, and carried into `aws-layer3-gate-lift`: `test` and `run` taking and
  releasing the claim; a second `test` refused naming the holder; the failed-apply sweep; the
  interrupt prints from `run` and `test`; `tofu destroy`'s IAM actions.
