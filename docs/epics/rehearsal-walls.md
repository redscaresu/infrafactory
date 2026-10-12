---
status: active
hld: 2026-09-30-import-production-state-into-the-mocks
depends_on: [rehearsal-prepare-gate, fakeaws-rehearsal-instance]
---

# A prepared work directory is inert: a bare `tofu apply` there can reach only this rehearsal's private fakeaws, for every service the provider knows, and `end` wipes everything (goals 7, 10).

**Done when:**
- The provider override names every key of the locked schema's `endpoints` block: the private fakeaws for modelled services, a dead loopback address for the rest, plus static fake keys and `skip_metadata_api_check`; a test derives the key list from `tofu providers schema -json`. The backend override names no `path`; the state copy and TF_DATA_DIR live outside the configuration copy.
- The rehearsal environment (replace mode): AWS_ENDPOINT_URL_<svc> at the private fakeaws, the catch-all and S3 at dead loopback, HTTP(S)_PROXY dead, fake keys, region. `awsLayer2Env` is split out of cloudEnv so no Scaleway/GCP/Genesys var or fakegenesys proxy is set; generated-HCL behaviour byte-identical (ADR-0039 amended).
- The fakeaws binary comes from a pin compiled into infrafactory (`go install …@SHA` into the data dir, or `--fakeaws-bin` checked against the pin); a binary without `-bind`/`-admin-token`/`-record-requests` is refused. It starts on a free loopback port with `-bind 127.0.0.1 -db <0700 dir> -admin-token -record-requests -account -region`, own process group, /healthz wait, pid and port recorded.
- `rehearse end` checks /healthz with the token on the recorded port before signalling the pid (otherwise only deletes files), then deletes work dirs, the state-config copy, db with -wal/-shm, snapshot, recorded reads and state copies, idempotently, and reports what it deleted. A test asserts every file under the rehearsal directory is 0600 after read, load and snapshot. Nothing is written under `.infrafactory/runs` or `live`.
- Fail-closed tests: remove the provider override, the environment wall or the backend override and `tofu apply` dies at a dead address fast; after `end`, a bare `tofu init && tofu apply` with real-looking shell credentials and AWS_PROFILE, on a resource in a service outside fakeaws's set, dies at a closed or dead address.
- ADR-0042 amended; golden files; docs/operations.md.

**Out of scope:** the gate and inits (rehearsal-prepare-gate); passthrough; discovery onward.

**Constraints:** ADR-0023 containment never weakened; existing runner process-group and interrupt rules (#436, #438); worktrees share a stale ../fakeaws, so never build from it; tofu claims pinned by `TestAssumption_EndpointsOverrideKeepsApplyOffProd`, `TestAssumption_CLIBackendPathKeepsStateOutOfTheCopy` (ADR-0028).

**Your part:** none.

**Areas:** `internal/cli/ (rehearse_walls*.go, rehearse_fakeaws*.go, rehearse_end.go new; test_command.go split; golden files)`; `internal/harness/static.go`; `internal/e2e/`; `docs/decisions/0039, 0042`; `docs/operations.md`
