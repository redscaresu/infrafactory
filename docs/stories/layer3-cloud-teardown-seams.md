---
kind: code
status: ready
epic: aws-layer3-seal-and-dispatch
depends_on: [layer3-cloud-entry-points]
touches: ["internal/cli/test_command.go", "internal/cli/run_command.go", "internal/cli/reap_command.go", "internal/cli/run_project_lifecycle.go", "internal/cli/destroy_retry.go", "internal/cli/stray_run_projects.go", "internal/cli/deploy_command.go", "internal/cli/live_upgrade.go", "internal/cli/live_teardown.go", "internal/cli/destroy_retry_test.go", "internal/cli/reap_command_test.go", "internal/cli/run_project_lifecycle_test.go", "internal/cli/stray_run_projects_test.go", "internal/cli/layer3_teardown_cloud_test.go (new)", "docs/stories/layer3-cloud-teardown-seams.md (delete)"]
risk: high
---

# Every Layer 3 seam helper takes the cloud; non-Scaleway runs, reaps and interrupts make no Scaleway call and never print Scaleway or reap advice

These helpers gain a layer3Cloud parameter: assertSandboxCredentials (test_command.go:1083), sandboxCommandEnvForProject (:1098), ensureRunProject (run_project_lifecycle.go:25), releaseRunProject (:120), assertRunProjectDeletable (:294), destroySandbox (destroy_retry.go:53), appendOrphanSweepResult (test_command.go:1674), reportStrayRunProjects (stray_run_projects.go:51) and withSandboxInterruptGuard (reap_command.go:144). The scaleway arm is today's body. sandboxEnvWithProjectDefault is untouched, so the audit at run_project_lifecycle_test.go:190-231 holds. Every other arm refuses with no dep call, including destroySandbox's aws arm: it has no without-config fallback because CaptureSweepTarget needs the Scaleway marker (orphan_sweep.go:96-100). The OrphanSweepRunner interface and wiring are unchanged (runtime.go:84-86,444-446).

Callers pass the value parsed once from sc.Cloud (test, run, reap, deploy) or d.Cloud (live upgrade, live teardown). runTestCommand loads the scenario before installing the guard at test_command.go:38; LoadScenario caches (runtime.go:152-156).

Non-scaleway branches cut early and write their own text: '<cloud> Layer 3 teardown is not built; resources recorded in <state> may exist: destroy them by hand'. It never says Scaleway and never names reap. The branches are: the run failure path (run_command.go:767-882), before the marker read at :788; the stray check at :913-918, which records a skip stage; reap after LoadScenario (reap_command.go:34), before :48; and the interrupt guard right after the sigCtx check, before the state and marker reads (:160-184). annotateWithRecoveryCommand, logLayer3RecoveryHint and reportAbandonedResources stay Scaleway-only.

Why tests change: destroy_retry_test.go, reap_command_test.go, run_project_lifecycle_test.go and stray_run_projects_test.go gain only the scaleway argument at their calls, because the seams now take the cloud.

**Done when:**
- Production callers, cloud aws and then gcp, with SCW_* set and recording RunProject, SandboxDestroy, AutoCreated, OrphanSweep and SandboxDeploy fakes: a failing run whose outdir holds terraform-live.tfstate with an aws_instance and a stale Scaleway marker records the failure naming the cloud, with zero calls and zero RunProject.List, and records the stray-check skip stage
- reap with the same outdir exits non-zero naming the cloud, with zero calls
- runTestCommand interrupted through a fake notify, once with state present and once with only a marker, prints the report naming the cloud, with zero calls
- In each of those outputs and logs, the aws text contains neither 'Scaleway' nor 'infrafactory reap'. The existing Scaleway tests still pin today's text
- The gate returns first for aws in `test` (test_command.go:642-660), so every helper is also called directly with aws and gcp and refuses with zero dep calls. layer3-cloud-entry-points' AST test stays green over the new calls
- Every existing test passes, and the only edits to the four named test files are the added cloud argument
- The doc-hygiene CI check passes, with the tip commit carrying 'ADR: none — fail-closed per-cloud dispatch under ADR-0023; aws-layer3-claim-sweep-reap records the AWS rules'
