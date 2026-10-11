---
status: active
hld: 2026-09-30-import-production-state-into-the-mocks
depends_on: []
---

# fakeaws runs as a private, observable, owner-only instance: loopback bind, per-instance admin token, loopback-Origin refusal on /mock/*, a request log in the read-set format, owner-only files.

**Done when:**
- `fakeaws -bind 127.0.0.1 -port N` listens on loopback only; the default stays `:port` so Makefile and CI users are unchanged.
- With `-admin-token <t>` (or env), every `/mock/*` request without `Authorization: Bearer <t>` is 401; without the flag the surface is unchanged; a non-loopback `Origin` on `/mock/*` is 403 even with the token (ADR-0026 extended).
- A recorder middleware on the service routes stores every provider request as a read-set entry `{service, protocol, action, params}` (the shape ADR-0041 defines; EC2/IAM/STS Action from the form body, SSM from `X-Amz-Target`) plus HTTP status and a classification: `answered-from-rows | not-modelled-404 | unimplemented-501 | filter-refused | empty-collection`. `GET /mock/requests` returns it; `POST /mock/requests/clear` empties it; `/mock/reset` clears it. `/mock/state` stops hard-coding `audit: []`.
- The classification distinguishes EC2's 404 "not yet implemented" default arm, the router's 501 and the four unmodelled empty-set Describes (only those count as `empty-collection`; an empty answer from rows is `answered-from-rows`), each with a contract test. `filter-refused` uses the one status fakeaws-data-sources picks.
- A file-backed database, its `-wal`/`-shm`, the snapshot and a restored copy are created 0600.
- Each new invariant has a CRITICAL/MUST docstring, a `TestContract_<id>` and an AGENTS.md "Current contracts" entry; concepts.md and infrafactory `docs/operations.md` describe the routes; ADR-0026 is amended and FAKEAWS_SHA bumped in a final story.

**Out of scope:** `/mock/load`, stored identity (fakeaws-loaded-identity), filters and AZs (fakeaws-data-sources); mockway and fakegcp admin surfaces.

**Constraints:** fakeaws AGENTS.md (no silent 200, no bare t.Skip, contract audit pairing, regression manifest, OSS-mature day one); existing admin tests keep passing; one chi middleware on the `/mock` group, one recorder on the service routes. The read-set shape comes from prod-read-seam's ADR-0041 story; the recorder story waits for it.

**Your part:** none.

**Areas:** `fakeaws:cmd/fakeaws/main.go`; `fakeaws:handlers/admin.go, handlers.go, recorder.go (new)`; `fakeaws:repository/repository.go`; `fakeaws:AGENTS.md, concepts.md, CHANGELOG.md`; `.github/workflows/ci.yml (FAKEAWS_SHA)`; `docs/decisions/0026-*`; `docs/operations.md`
