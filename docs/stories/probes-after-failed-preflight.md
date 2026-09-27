---
kind: code
touches: [internal/cli/test_command.go, internal/cli/testdata/golden/commands]
status: ready
---

# Real probes run after Layer 3 preflight has failed

`test` runs the real-probe stage even when Layer 3's preflight failed, so a run that never
applied also reports a `real_probe` failure for a missing `terraform-live.tfstate`. Found in
S202's review; the `run` golden files record it.

**Done when:** probes are reported as not run when nothing was applied, the golden files carry
only the preflight failure, and a test pins it.
