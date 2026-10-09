---
kind: lead
status: blocked
blocked_by: [aws-web-live-real-run, aws-layer2-seeds-resolved-ami, aws-web-live-layer2-llm-run]
epic: aws-web-live-on-real-aws
depends_on: [aws-web-live-real-run, aws-layer2-seeds-resolved-ami, aws-web-live-layer2-llm-run]
touches: []
risk: high
---

# A fixture opening 22 to 0.0.0.0/0 fails the holdout on real AWS with no repair, and the scope still ends empty

The epic's second bullet, plus the `run` interrupt leg of the fourth. No PR: the evidence goes to
the closing PR body. The epic's **Checks before every stage** apply.

**You:** say go for a run that opens 22 to the internet on a real instance for its probe window,
with policies/aws/default_deny_ingress.rego bypassed in a throwaway config, in the lead's session.
The lead does not start without it, and tells you the run ids and the cost bound when they end.

"Fixture, not model" needs a stub: `run` has no seed flag (internal/cli/root.go:58-76) and always
calls the agent. A stub `agent.claude.command` in the local config answers each phase with the
step-one fixture: the resolved AMI (seeded into fakeaws by aws-layer2-seeds-resolved-ami), ingress
tcp 22 from 0.0.0.0/0, and no user-data file (infrafactory renders it itself and refuses one the
agent writes, internal/cli/aws_user_data.go:62-63). It must be 22: the holdout's blocked check is a
TCP dial (internal/harness/real_probe.go:142-153) and only sshd answers on AL2023, so 443 would pass
vacuously. default_deny_ingress.rego refuses 22 at Layer 1 on every AWS plan, so the bypass is
deliberate and narrowest: the local `policy_paths` swaps `./policies/aws` for a scratch copy without
default_deny_ingress.rego, and a scenario copy drops only the default_deny_ingress criterion. Record
both, and the stub, in the closing PR body. Then a second stub `run`, interrupted during the apply,
must print `reap --take-over <holder>`; that command releases.

**Done when:**
- Before the go: the cumulative cost bound of the epic's real runs so far plus these runs' worst
  case is written down, dated, and is under €5.
- The run's terminal reason is holdout_failed. Using aws-layer3-run-evidence's per-check records,
  all three holdout checks ran at the same public IP: 22 failing with "unexpectedly succeeded", 443
  blocked, and 80 succeeding. iteration.json holds one iteration, with no repair generation.
- aws_scope_sweep and aws_scope_release pass after the holdout failure. `aws ssm get-parameter` on
  harness.AWSClaimParameter then returns ParameterNotFound and `infrafactory reap --dry-run` sweeps
  empty.
- The interrupted stub `run` prints `reap --take-over <holder>` with its own holder and leaves the
  claim held. That command exits 0, and the next sweep is empty.
- `git -C <throwaway worktree> status --short policies/ scenarios/` shows the bypass never touched a
  tracked file. The evidence states the run ids and the dated cost bound, with no 12-digit run.
