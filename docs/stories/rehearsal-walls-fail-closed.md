---
kind: code
status: blocked
blocked_by: [rehearsal-walls-prepare-e2e, rehearsal-walls-env]
epic: rehearsal-walls
depends_on: [rehearsal-walls-prepare-e2e, rehearsal-walls-env]
touches: [internal/cli/rehearse_walls_e2e_test.go, internal/cli/export_test.go, .github/fakeaws-e2e-tests]
risk: high
---

# Fail-closed e2e: with all walls an apply lands on fakeaws; remove any one wall and it dies at a dead address, fast

New e2e test file `internal/cli/rehearse_walls_e2e_test.go` (cli_test, e2e.SkipUnlessEnabled; added to .github/fakeaws-e2e-tests). It runs a real prepare with the fakeaws binary from rehearsal-walls-prepare-e2e's helper (built from FAKEAWS_DIR, a clone at FAKEAWS_SHA). The root has a provider with max_retries = 1, an aws_vpc (EC2, modelled), an aws_lb (ELBv2, not modelled) and a user `backend "s3"`. The shell carries real-looking AWS keys (AKIAIOSFODNN7EXAMPLE), AWS_PROFILE naming a profile with real-looking keys in a planted HOME/.aws, and HTTPS_PROXY/HTTP_PROXY at a recorder.

Cases:
1. All walls, rehearsalWallsEnv under replace mode: `apply -target=aws_vpc` succeeds and the vpc is in fakeaws's request log (fetched with the token); applying aws_lb dies at the dead address.
2. The provider override removed: the aws_lb apply under the walls env dies at the dead address (the AWS_ENDPOINT_URL catch-all).
3. The environment wall removed: TF_DATA_DIR and HOME keep their walls values (so the providers stay installed), and every AWS_* and proxy variable comes from the shell as is. The aws_lb apply dies at the dead address through the override's elbv2 key.
4. The backend override removed: `apply` and `init -reconfigure -plugin-dir=<data>/providers` under the walls env each die.

In every case the recorder counts zero requests, and each failure comes within 60s.

**Done when:**
- Case 1 shows CreateVpc in /mock/requests and the aws_lb failure naming the dead address.
- Cases 2-4 each exit non-zero within 60s with the recorder at zero, and the errors of cases 2 and 3 name rehearsalDeadProxy's address (not a missing provider). A cp-backup break that points the walls env's AWS_ENDPOINT_URL catch-all at the recorder turns case 2 red, and one that points the override's elbv2 key at the recorder turns case 3 red.
- The test name is in .github/fakeaws-e2e-tests (sorted) and passes in the 'AWS against fakeaws' CI step.
- Checks before every stage in docs/epics/rehearsal-walls.md hold.

**From review (binding; overrides the scope above where they differ):**
- Case 3 keeps TF_DATA_DIR and HOME and drops only the AWS_*/proxy wall, and asserts the error names the dead address; with the shell's env as is, apply would die at "provider not installed" and pass for the wrong reason (critic).
- fakeaws is built from FAKEAWS_DIR, defaulting to a scratch clone at FAKEAWS_SHA, never the shared ../fakeaws (critic). Depends on rehearsal-walls-prepare-e2e, which owns that helper.
