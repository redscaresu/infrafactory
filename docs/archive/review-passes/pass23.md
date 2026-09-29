# S165 review pass 23 — one finding, acted on

### [P1] The destroy environment still used the shared fallback project

The asymmetry I missed. The apply built its environment with
`sandboxCommandEnvForProject(runtime, runProjectID)`, but the destroy path a
hundred lines below still called `sandboxCommandEnv(runtime)` — so
`SCW_DEFAULT_PROJECT_ID` pointed back at the shared fallback for teardown.

Destroy refreshes and removes resources that carry no `project_id` of their own,
which is the entire motivating case for this flag
(`scaleway_instance_private_nic`). Looking for them in the wrong project would
fail the teardown, or leave them behind — turning a change meant to make
projectless resources safe into one that strands them.

Confirmed by reading both call sites rather than reasoning about them, then
fixed. Both now use the project-aware helper.

Guarded by `TestPipelineNeverBuildsSandboxEnvWithoutTheRunProject`, a source
audit in the repo's existing idiom (`cloud_prefix_lockstep_test.go`): the defect
lives in *which helper a call site picks*, which is what drifts, and no unit test
of either helper would have caught it. Verified against injected drift — the
first attempt at that verification silently failed to reproduce the regression,
which would have left an audit that never fires.
