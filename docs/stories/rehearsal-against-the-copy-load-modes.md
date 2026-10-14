---
kind: code
status: blocked
blocked_by: [rehearsal-walls-docs, fakeaws-loader-pin]
epic: rehearsal-against-the-copy
depends_on: [rehearsal-walls-docs, fakeaws-loader-pin]
touches: [internal/cli/rehearse_fakeaws.go, internal/cli/rehearse_fakeaws_test.go, internal/cli/rehearse_walls_teardown_e2e_test.go, docs/decisions/0042-rehearsal-command-contract-and-walls.md, docs/operations.md]
risk: high
---

# Once the CI pin holds /mock/load: start fakeaws with -load-max-bytes, and prove every file 0600 after load

Moved from rehearsal-walls, so that epic closes without waiting on fakeaws-loader-pin. It waits for fakeaws-loader-pin (FAKEAWS_SHA and rehearsalFakeawsPin at a fakeaws main with /mock/load) and for rehearsal-walls to close.

Add `-load-max-bytes <rehearsalLoadMaxBytes>` (a named constant) to the start args in rehearse_fakeaws.go and to the stub's expected argv. The teardown e2e (rehearse_walls_teardown_e2e_test.go) loads the recorded read through POST /mock/load with the token, between read and snapshot, before the mode walk. ADR-0042 § Start and docs/operations.md name the flag.

**Done when:**
- The start test's exact-argv assertion includes -load-max-bytes, and dropping it from the start args turns the test red.
- The teardown e2e shows /mock/load returning 200 before the walk, and the walk fails when a loaded-db file is chmodded 0644 (cp-backup).
- `grep -n 'load-max-bytes' docs/operations.md docs/decisions/0042-rehearsal-command-contract-and-walls.md` finds both files.
- `make doc-hygiene` passes; each new assertion is shown red by a cp-backup break recorded in the PR; the e2e builds fakeaws from FAKEAWS_DIR (a clone at FAKEAWS_SHA), never the shared ../fakeaws; ADR-0042 § Start is amended in the same PR.

**From review (binding; overrides the scope above where they differ):**
- Moved from rehearsal-walls to rehearsal-against-the-copy, which already depends on fakeaws-loader; walls closes in rehearsal-walls-docs instead (critic). The epic-closing scope is dropped here.
