---
kind: code
status: blocked
blocked_by: [rehearsal-walls-end, rehearsal-walls-fail-closed]
epic: rehearsal-walls
depends_on: [rehearsal-walls-end, rehearsal-walls-fail-closed]
touches: [internal/cli/rehearse_walls_teardown_e2e_test.go, .github/fakeaws-e2e-tests]
risk: high
---

# Teardown e2e: every file 0600 after read and snapshot; after end nothing is left, and a bare tofu dies at a closed address

New e2e test file `internal/cli/rehearse_walls_teardown_e2e_test.go` (added to .github/fakeaws-e2e-tests), using rehearsal-walls-prepare-e2e's helper for the fakeaws binary (built from FAKEAWS_DIR, a clone at FAKEAWS_SHA).

First, prepare. Then `rehearse read --out <id>/reads` against a stand-in prod fakeaws from the same binary (TestAWSReadAgainstFakeaws pattern), and POST /mock/snapshot with the token. Then walk <id>: dirs 0700, terraform-provider-* 0700, every other file 0600; a fakeaws db, -wal, -shm or snapshot at another mode fails.

Before end, the test copies <id>/work and <id>/data/providers to a temp dir, standing in for a user who kept a copy. Then `rehearse end <id>`: <id> is gone, the report lists the db, -wal/-shm, snapshot, recorded read, state and work, and the recorded port refuses a dial. In the kept copy, a bare `tofu init -plugin-dir=<copied providers> && tofu apply` runs with the shell's real-looking keys, AWS_PROFILE and HTTPS_PROXY at a recorder, on an aws_lb. It dies at the closed fakeaws port or the dead address, and the recorder counts zero. Nothing exists under .infrafactory/runs or live at any point.

**Done when:**
- The mode walk fails when the test chmods one fakeaws file to 0644 before walking (cp-backup), and passes otherwise.
- After end, the bare apply exits non-zero naming 127.0.0.1 and the recorder is zero. A cp-backup break that rewrites the kept copy's infrafactory_aws_override.tf sts key to the recorder turns it red.
- A second `rehearse end <id>` exits 0 with nothing deleted.
- The test name is in .github/fakeaws-e2e-tests (sorted) and passes in CI.
- Checks before every stage in docs/epics/rehearsal-walls.md hold.

**From review (binding; overrides the scope above where they differ):**
- fakeaws, the rehearsal's and the stand-in prod's, is built from FAKEAWS_DIR, defaulting to a scratch clone at FAKEAWS_SHA, never the shared ../fakeaws (critic).
- The 0600 check after load is no longer here: it moved with the load story to rehearsal-against-the-copy (critic).
