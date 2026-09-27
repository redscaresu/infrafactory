# Review pass 209 — S202: destruction is refused as a criterion

`codex exec review --base main`, 2026-09-27.

1. **P3, declined.** The `run` golden fixture now declares a `connectivity` criterion, and with
   `sandbox_deploy` enabled and no credentials the snapshot records a `real_probe` failure
   (missing `terraform-live.tfstate`) next to the credentials failure. Declined because:
   - the old snapshot was clean only because the fixture's sole criterion was the one nothing
     evaluated, which is the defect this PR removes;
   - no criterion type keeps it clean. `policy` fails for want of a `constraint_policies`
     mapping in the harness config, and `http_probe` and `dns_resolution` probe too;
   - the extra line is a failure, not a pass. The run already fails on credentials, so the
     probe cannot turn a failing run green. Probes running after a failed Layer 3 preflight is
     earlier behaviour of `test`, and changing it is not part of this change.
2. **P3, accepted.** The `init` scaffold declared the `region_restriction` criterion twice. My
   fixture rewrite had run over `root.go` after it was already edited by hand.
