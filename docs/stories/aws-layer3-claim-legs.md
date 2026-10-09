---
kind: lead
status: blocked
blocked_by: [aws-layer3-iam-measured, aws-interrupt-names-take-over, aws-web-live-layer2-llm-run]
epic: aws-web-live-on-real-aws
depends_on: [aws-layer3-iam-measured, aws-interrupt-names-take-over, aws-web-live-layer2-llm-run]
touches: []
risk: high
---

# On real AWS, a second `test` is refused naming the holder, and an interrupted `test` keeps the claim until `reap --take-over` releases it

The `test` claim legs of the epic's fourth bullet, after aws-layer3-iam-measured has shown the
policy covers the run. Same staging as that story (generated step-one fixture, throwaway worktree,
local `--config`); the epic's **Checks before every stage** apply before each run. No PR: the
evidence goes to the closing PR body.

**You:** say go for these runs in the lead's session (one go covers the pair below). The lead does
not start without it, and tells you the run ids and the cost bound when they end.

1. Contention: start a `test` from one worktree; while it holds the claim, start a second `test`
   from a second worktree. The second must fail at aws_scope_claim naming the first's holder, and
   must write nothing. The first then finishes and releases.
2. Interrupt: Ctrl-C a `test` during the apply. The print must name
   `infrafactory reap --take-over <holder>` (aws-interrupt-names-take-over). The claim stays held.
   Run that command; it destroys what is left and releases.

**Done when:**
- Before the go: the cumulative cost bound of the epic's real runs so far plus these runs' worst
  case is written down, dated, and is under €5.
- The second `test` fails at aws_scope_claim with output naming the first run's holder, and the
  first `test` still ends with aws_scope_sweep and aws_scope_release passing.
- The interrupted `test` prints `reap --take-over <holder>` with its own holder. `aws ssm
  get-parameter` on harness.AWSClaimParameter then shows that holder. `reap --take-over <holder>`
  exits 0, after which get-parameter returns ParameterNotFound and `infrafactory reap --dry-run`
  sweeps empty.
- CloudTrail over these runs shows no refused event by infrafactory-layer3. The evidence states
  the run ids and the dated cost bound, with no 12-digit run.
