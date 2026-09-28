---
kind: code
status: ready
epic: aws-layer3-gate
depends_on: []
touches: ["internal/cli/destroy_retry.go", "internal/cli/aws_destroy_arm_test.go (new)", "internal/cli/layer3_teardown_cloud_test.go", "docs/stories/aws-layer3-destroy-arm.md (delete)"]
risk: high
---

# destroySandbox gets an aws arm: tofu destroy with the sealed env, with no Scaleway purge, no deletable check and no without-config fallback

destroy_retry.go:62-65 refuses aws today. aws-layer3-claim-sweep-reap owns the sweep, reap and failure paths, but its Areas leave out destroy_retry.go (aws-layer3-claim-sweep-reap.md:24). This story owns the arm.

For aws, destroySandbox calls runtime.Deps.SandboxDestroy.Run(ctx, workDir, sandboxEnv) once and returns its result and error. It never calls resolveRunProjectID, destroyAndPurge, AutoCreated, assertRunProjectDeletable or RunWithoutConfig. RunWithoutConfig would refuse an AWS state anyway (sandbox_destroy.go:211-218). An empty sandboxEnv, or one without AWS_ACCESS_KEY_ID, is refused before any call. gcp and any other cloud still refuse.

This changes no reachable behaviour today: every caller that could pass aws returns earlier. Those callers are the test gate (test_command.go:719-736), run's failure path (run_command.go:778), reap (reap_command.go:51), the interrupt guard (reap_command.go:181) and live teardown (live_teardown.go:70). aws-layer3-gate-lift proves the arm through a production caller.

Why a test changes: the aws row of TestEveryTeardownSeamRefusesAnotherCloud (layer3_teardown_cloud_test.go:262-283) no longer lists the destroy seam. gcp keeps it.

**Done when:**
- destroySandbox(aws), with a recording SandboxDestroy, calls Run exactly once with the given workDir and sealed env. AutoCreated, RunProject and RunWithoutConfig get zero calls, and so does any Scaleway HTTP call
- When SandboxDestroy.Run fails, destroySandbox(aws) returns that error. There is no second Run and no RunWithoutConfig call
- destroySandbox(aws) refuses a nil env and an env without AWS_ACCESS_KEY_ID, with zero calls. destroySandbox(gcp) still refuses with zero calls
- TestAllDestroyCallsGoThroughTheRetryWrapper and destroy_retry_test.go pass unmodified. TestEveryTeardownSeamRefusesAnotherCloud passes with only the destroy seam removed from its aws row
- doc-hygiene passes, and the tip commit carries 'ADR: none — AWS destroy arm of the seam aws-layer3-seal-and-dispatch parameterised; ADR-0023's AWS record is aws-layer3-claim-sweep-reap's'
