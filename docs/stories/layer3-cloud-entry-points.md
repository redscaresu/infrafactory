---
kind: code
status: ready
epic: aws-layer3-seal-and-dispatch
depends_on: []
touches: ["internal/cli/layer3_cloud.go (new)", "internal/cli/layer3_cloud_test.go (new)", "internal/cli/layer3_hcl_shape.go", "internal/cli/test_command.go", "internal/cli/generate_command.go", "internal/cli/deploy_command.go", "internal/cli/live_upgrade.go", "internal/cli/live_teardown.go", "internal/cli/run_keep.go", "docs/stories/layer3-cloud-entry-points.md (delete)"]
risk: high
---

# The gate and the live paths take the cloud from the scenario or the live record; anything but scaleway is refused before any credential check

New internal/cli/layer3_cloud.go defines type layer3Cloud and parseLayer3Cloud(raw). "" and scaleway map to scaleway: "" appears only in Go fixtures (runtime_test.go:47,233), since the schema requires cloud, and in livestore records written before Cloud existed (livestore.go:131-134), when Scaleway was the only live cloud. aws maps to aws. Anything else is an error naming it. There is no runtime-derived default: every caller parses the string it holds.

layer3PreflightHCLForCloud: scaleway calls layer3PreflightHCL unchanged, and aws and every other cloud are refused with a message naming the cloud (aws-layer3-gate replaces the aws arm). It is called with sc.Cloud at test_command.go:615 and deploy_command.go:106 (sc loaded at :552 and :69), and with d.Cloud at live_upgrade.go:89 (record read at :59-63). generate_command.go:739-748 keys on scenarioMeta.Cloud (:682-685) and refuses when that unmarshal failed with sandbox_deploy enabled.

Live paths, each before any credential check:
- assertKeepable (run_keep.go:20-35) and deploy (deploy_command.go:95-98) refuse aws with a message naming the live path (HLD:356-357), and refuse other clouds fail-closed.
- live upgrade refuses any record cloud but scaleway.
- tearDownDeployment (live_teardown.go:32) returns unreclaimable for a non-scaleway record before its env builds (:100,:150). This covers live teardown, live reap and LiveActions (live_teardown.go:335,408; live_service.go:68,107). The message names the cloud and the state path and says to destroy by hand and then `live forget`; it never says Scaleway or reap.

**Done when:**
- parseLayer3Cloud table: "" and scaleway give scaleway, aws gives aws, and gcp, genesys and azure error, naming the value
- Through runTestCommand with sandbox_deploy enabled: aws and gcp scenarios whose HCL passes validateLayer3HCLShape for Scaleway fail at sandbox_deploy/allowlist with a message naming the cloud. Recording SandboxDeploy, RunProject, OrphanSweep, AutoCreated and SandboxDestroy fakes see zero calls
- Generation with an aws scenario and a Scaleway-valid stack refuses, naming aws. A scenario payload whose cloud cannot be unmarshalled refuses with sandbox enabled
- Through the run and deploy commands: run --keep and deploy on aws and gcp fail before assertSandboxCredentials with zero fake calls, and the aws message names the live path
- live upgrade of records with cloud aws and gcp is refused before any credential check with zero fake calls, while a record with "" passes the cloud check
- live teardown and live reap of an aws record holding state and a marker are unreclaimable naming aws, with zero fake calls, the record not released, and the detail containing neither 'Scaleway' nor 'reap <'
- AST test: no non-test call in internal/cli passes the layer3Scaleway or layer3AWS constant, or a layer3Cloud(...) conversion, as an argument; it fails naming the call site
- layer3_hcl_shape_test.go, layer3_undestroyable_test.go, vpc_required_lockstep_test.go, run_keep_test.go, deploy_command_test.go, live_upgrade_test.go and live_teardown_test.go pass unmodified
- The doc-hygiene CI check passes, with the tip commit carrying 'ADR: none — fail-closed per-cloud dispatch under ADR-0023; aws-layer3-claim-sweep-reap records the AWS rules'
