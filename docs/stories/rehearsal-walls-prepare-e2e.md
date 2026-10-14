---
kind: code
status: blocked
blocked_by: [rehearsal-walls-prepare-wiring]
epic: rehearsal-walls
depends_on: [rehearsal-walls-prepare-wiring]
touches: [internal/cli/rehearse_prepare.go, internal/cli/rehearse_prepare_e2e_test.go, internal/cli/export_test.go, internal/cli/testdata/golden/commands/rehearse_prepare*, .github/workflows/ci.yml, .github/fakeaws-e2e-tests]
risk: high
---

# prepare's --fakeaws-bin and fakeaws_url, and TestRehearsePrepare against a real fakeaws built at the pin

rehearse_prepare.go adds `--fakeaws-bin` (unset by default, which means the pinned install), bound to rehearsePrepare's fakeawsBinPath. Prepare's human and JSON reports gain `fakeaws_url` (http://127.0.0.1:<port>) and never the token. The rehearse_prepare goldens are updated.

rehearse_prepare_e2e_test.go gains a helper that builds fakeaws from FAKEAWS_DIR. When FAKEAWS_DIR is unset it clones github.com/redscaresu/fakeaws into a scratch dir under t.TempDir() and checks out FAKEAWS_SHA (rehearsalFakeawsPin) there. It never builds from the shared ../fakeaws by default, and it fails when the dir's HEAD is not the pin. The CI step 'AWS against fakeaws' sets FAKEAWS_DIR to the clone it already makes at FAKEAWS_SHA (.github/workflows/ci.yml:229-236). fakeaws-loader-pin edits FAKEAWS_SHA in the same step; whichever lands second merges main in.

TestRehearsePrepare passes that binary as --fakeaws-bin. Its cleanup stops the recorded group (through `rehearse end` once it exists). This story also adds TestRehearsalFakeawsInstallAtPin (from rehearsal-walls-fakeaws-pin) to .github/fakeaws-e2e-tests.

**Done when:**
- TestRehearsePrepare (e2e, in .github/fakeaws-e2e-tests already) asserts: the override's endpoints keys equal the keys the test derives itself from `tofu providers schema -json` (the endpointKeys helper); infrafactory_override.tf still equals LocalBackendOverride; infrafactory_aws_override.tf equals rehearsalProviderOverride(record) for the record's one root; GET /mock/requests with the record's token is 200 on the recorded port after Execute returns; assertTightModes passes; the token is absent from stdout; and no .infrafactory/runs or .infrafactory/live dir exists.
- Before Execute, the test sets HTTPS_PROXY and HTTP_PROXY in the shell (t.Setenv) to a recorder, and after prepare the recorder has counted zero requests. rehearsalDeadProxy keeps refusing as today. A cp-backup break that drops ReplaceEnv from the schema call turns it red.
- TestCommandOutputGoldenSnapshots passes with the rehearse_prepare goldens updated to show fakeaws_url, and `rehearse prepare --help` lists --fakeaws-bin.
- With FAKEAWS_DIR pointing at a clone whose HEAD is not the pin, the helper fails naming both SHAs.
- TestRehearsalFakeawsInstallAtPin is in .github/fakeaws-e2e-tests (sorted) and passes in the CI step.
- Checks before every stage in docs/epics/rehearsal-walls.md hold.

**From review (binding; overrides the scope above where they differ):**
- Split from prepare-wiring; this half holds the user-facing flag and output, the goldens and the real-fakeaws e2e (codex).
- The dead-proxy recorder is replaced by a recorder set as the shell's HTTPS_PROXY/HTTP_PROXY with t.Setenv, asserting zero requests; a recorder at rehearsalDeadProxy would make every prepare refuse (skeptic).
- The /mock/requests check after Execute returns stays: it is the only check that fakeaws outlives prepare's ctx (skeptic).
- The e2e builds fakeaws from FAKEAWS_DIR, defaulting to a scratch clone at FAKEAWS_SHA, and CI sets FAKEAWS_DIR; never the shared ../fakeaws (critic).
