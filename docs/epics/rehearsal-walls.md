---
status: active
hld: 2026-09-30-import-production-state-into-the-mocks
depends_on: [rehearsal-prepare-gate, fakeaws-rehearsal-instance]
---

# A prepared work directory is inert: a bare `tofu apply` there can reach only this rehearsal's private fakeaws, for every service the provider knows, and `end` wipes everything (goals 7, 10).

**Done when:**
- The provider override names every key of the locked schema's `endpoints` block: the private fakeaws for modelled services, a dead loopback address for the rest, plus static fake keys and `skip_metadata_api_check`; a test derives the key list from `tofu providers schema -json`. The backend override names no `path`; the state copy and TF_DATA_DIR live outside the configuration copy.
- The rehearsal environment (replace mode): AWS_ENDPOINT_URL_<svc> at the private fakeaws, the catch-all and S3 at dead loopback, HTTP(S)_PROXY dead, fake keys, region. `awsLayer2Env` is split out of cloudEnv so no Scaleway/GCP/Genesys var or fakegenesys proxy is set; generated-HCL behaviour byte-identical (ADR-0039 amended).
- The fakeaws binary comes from a pin compiled into infrafactory (`go install …@SHA` into `<UserCacheDir>/infrafactory/fakeaws/<pin>`, or `--fakeaws-bin`); any binary whose build info is not the pin is refused. It starts on a free loopback port with `-bind 127.0.0.1 -db <0700 dir> -record-requests -account -region` and the admin token in `FAKEAWS_ADMIN_TOKEN` only (never on argv), in its own process group. It is ready only when /healthz answers and `GET /mock/requests` answers 200 with the token and 401 without it; pid, port and token are recorded.
- `rehearse end` checks /healthz and the token-guarded `GET /mock/requests` on the recorded port before signalling the pid (otherwise only deletes files), then deletes work dirs, the state-config copy, db with -wal/-shm, snapshot, recorded reads and state copies, idempotently, and reports what it deleted. A test asserts every file under the rehearsal directory is 0600 after read and snapshot (after load: rehearsal-against-the-copy-load-modes). Nothing is written under `.infrafactory/runs` or `live`.
- Fail-closed tests: remove the provider override, the environment wall or the backend override and `tofu apply` dies at a dead address fast; after `end`, a bare `tofu init && tofu apply` with real-looking shell credentials and AWS_PROFILE, on a resource in a service outside fakeaws's set, dies at a closed or dead address.
- ADR-0042 amended; golden files; docs/operations.md.

**Checks before every stage:**
- `make doc-hygiene` passes.
- Each new test is shown red by a cp-backup break recorded in the PR, never by git checkout.
- An e2e that runs fakeaws uses a binary built from FAKEAWS_DIR (a clone at FAKEAWS_SHA), never the shared ../fakeaws.
- TestRehearsePrepare and the TestAssumption_* tests listed in .github/fakeaws-e2e-tests pass with INFRAFACTORY_ENABLE_E2E=1.
- A code story's tip commit carries `ADR: none — ADR-0042 records <decision>`.

**Decided in scoping (2026-10-14):**
- The awsLayer2Env split is already done (test_command.go:234-257), so test_command.go and internal/harness/static.go are untouched; a new rehearsalWallsEnv stands beside it.
- The walls env sends S3 and every unmodelled service to rehearsalDeadProxy, and takes AWS_REGION from --region.
- The re-gate admission (byte-identical, bytes rebuilt from the record) and rehearsalRegate are built here; passthrough only calls them.
- The endpoints keys come from the locked provider's schema (`tofu providers schema -json` after the second init), not a static 5.100.0 list.
- A Go pin const, rehearsalFakeawsPin, is held equal to ci.yml's FAKEAWS_SHA by a lockstep test.
- The pinned install goes to `<UserCacheDir>/infrafactory/fakeaws/<pin>`, outside the rehearsal, and every binary passes a buildinfo check at the pin.
- Flag support cannot be read off a binary, so the pin check plus live checks at start stand in for it.
- fakeaws cannot report an ephemeral port: pick, release and start, with up to 3 tries on a bind race.
- Prepare starts fakeaws as its last step; there is no `rehearse start`, and a new prepare is the restart.
- Start and end both require /healthz and a token-guarded /mock/requests; start also requires a 401 without the token.
- The token goes to fakeaws only as FAKEAWS_ADMIN_TOKEN in its env, never on argv, and is never printed.
- The bare-tofu-after-end test runs in a copy of work/ taken before end.
- `<id>/reads` and `<id>/state-config` are reserved in the layout, and end deletes them with `<id>`.
- Naming every endpoints key draws alias-conflict warnings on every plan; accepted and documented.
- The load story moved to rehearsal-against-the-copy (rehearsal-against-the-copy-load-modes), so this epic does not wait on fakeaws-loader-pin.

**Out of scope:** the gate and inits (rehearsal-prepare-gate); passthrough; discovery onward.

**Constraints:** ADR-0023 containment never weakened; existing runner process-group and interrupt rules (#436, #438); worktrees share a stale ../fakeaws, so never build from it; tofu claims pinned by `TestAssumption_EndpointsOverrideKeepsApplyOffProd`, `TestAssumption_CLIBackendPathKeepsStateOutOfTheCopy` (ADR-0028); the environment sets AWS_REGION from `--region` and sets no other region variable, with a test; the "every file is 0600" test admits 0700 for executables tofu writes (ADR-0042); walls reuses rehearsal-prepare-replace-env's ReplaceEnv and rehearsal-prepare-second-init's rehearsalDeadProxy; the provider override is one of infrafactory's own override files, which the re-gate before every tofu step admits only when byte-identical to what infrafactory writes, so walls exposes the exact bytes it writes for that check (ADR-0042).

**Your part:** none.

**Areas:** `internal/cli/ (rehearse_walls*.go, rehearse_fakeaws*.go, rehearse_end.go new; golden files)`; `docs/decisions/0039, 0042`; `docs/operations.md`
