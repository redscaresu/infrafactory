---
kind: code
status: blocked
blocked_by: [aws-layer3-stage-order-test]
epic: aws-web-live-on-real-aws
depends_on: [aws-layer3-stage-order-test]
touches: [internal/harness/real_probe.go, internal/harness/real_probe_test.go, internal/cli/aws_post_apply.go, internal/cli/run_command.go, internal/cli/test_command.go, internal/cli/holdout.go, internal/cli/aws_layer3_order_test.go]
---

# A passing aws `run` keeps its sandbox_deploy stages in iteration.json, with each probe's address, status, attempts and time to first success

The epic's first bullet is read from the run's own evidence, and today a passing run throws most of
it away:
- On a pass, `run` folds every executeTest stage into `iteration_N_test` and keeps only the holdout
  and `aws_scope_*` stages (internal/cli/run_command.go:1215-1216, 1260-1261; awsScopeStages
  :1738). So aws_ami_resolve, account_check, user_data_check and real_probe never reach
  iteration.json (PR #412 hit this).
- The probe stage is `real_probe` (internal/cli/test_command.go:1508-1533), and its pass stage has
  no detail. The real capture lives in RealProbeHarness: retry (internal/harness/real_probe.go:241-270)
  does not return an attempt count, runHTTPProbe (:178-208) accepts any status below 400 and drops
  resp.StatusCode, and a passing blocked dial (:135-176) records no host:port. RealProbeResult
  (:45-47) carries failures only.
- user_data_check's pass stage is built in appendAWSPostApplyChecks
  (internal/cli/aws_post_apply.go:40-67); the instance id stays inside awsUserDataCheck (:80).

Change: retry returns its attempt count. RealProbeResult carries one record per check, filled by
the real harness: kind, host:port or URL, expect, status (HTTP status, or the dial outcome),
attempts, and seconds to first success. The `real_probe` pass stage (keep that name everywhere; no
`http_probe` stage is added) puts the records in its Detail, and so does each holdout stage
(internal/cli/holdout.go:129-160). user_data_check's pass detail names the instance id. For cloud
aws, `run` carries aws_ami_resolve, account_check, user_data_check and real_probe into
iteration.json alongside the holdout and aws_scope_* stages. No account id in any detail. No
pass/fail rule changes. Every RealProbeResult fake in internal/cli/*_test.go keeps compiling.
Commit trailer: `ADR: none — evidence fields only`.

**Done when:**
- In internal/harness/real_probe_test.go, an injected getHTTP that fails once and then returns 302
  yields a record with status 302 and attempts 2, and the check passes. An injected dialFunc that
  refuses yields a passing blocked record naming `<host>:22`. Deleting the status capture
  (cp backup, restore by cp) fails that test; the PR records it.
- The injected-fake `run --holdout` (cloud aws, sandbox_deploy on, the stage-order fakes from
  aws_layer3_order_test.go) writes an iteration.json with aws_ami_resolve, account_check,
  user_data_check (naming the fake's instance id) and a `real_probe` pass stage whose detail names
  `http://<the fake state's aws_instance.public_ip>:80` and the fake's status, plus holdout detail
  naming `<same ip>:22`, `<same ip>:443` and `<same ip>:80`. The test reads each value back from the
  file and reads the ip from the fake state, not from the fake probe.
- No stage detail in that iteration.json matches `[0-9]{12}`.
- TestAWSRunWalksTheLayer3StagesInOrder, TestAWSRunEndsAtTheLayer3StageThatFails and
  `go test -tags noui ./internal/cli/... ./internal/harness/...` pass, with the wiring-proof guards
  unweakened.
