# S165 review pass 24 — one finding, declined with reasoning

### [P2] Handle run-owned projects in sweep capture — DECLINED (S166/S167 scope)

The finding: once the HCL stops declaring `scaleway_account_project`,
`CaptureSweepTarget` fails closed (it reads the project id from that resource in
state), so a successful apply/destroy would still append an `orphan_sweep`
failure, and `destroySandbox` would get an empty project id and skip the
auto-created purge. It proposes using `runProjectID` as the sweep target instead.

**The diagnosis is correct and the proposed fix is the right eventual design.**
It is declined here because of where it lands, verified rather than assumed:

- `CaptureSweepTarget`'s output flows into `destroySandbox`, which calls
  `harness.AssertProjectDeletable` (`destroy_retry.go:69`). Changing where the
  sweep target comes from **is** changing what feeds that guard — the
  state-derived cross-check that stands between an automated destroy and real
  infrastructure. ADR-0025 assigns that to S166 precisely because it needs
  human review, and this slice was instructed to stop before it.
- The failure is not reachable today: `examples/layer3-gate/*/main.tf` still
  declare `scaleway_account_project`, so `CaptureSweepTarget` still finds one.
  S167 is the slice that removes them, and it must not land before S166.

**What the finding did surface, and what has been documented as a result:** with
the flag ON *before* S167, a run gets **two** projects — the pre-created
run-owned one and the one its HCL still declares. Nothing leaks (the run-owned
one is empty and deleted by the cleanup, the declared one is destroyed and swept
as always), but it is wasteful and confusing, and the flag should not look ready
when enabling it today does that.

Said plainly now in `infrafactory.yaml`, the config comment, and the PR: **the
flag is mechanically complete but not coherent until S167**, and the ordering
S166 → S167 is a safety requirement, not a preference.
