---
kind: chore
status: blocked
blocked_by: [fakeaws-request-log-read-set]
epic: fakeaws-rehearsal-instance
depends_on: [fakeaws-request-log-read-set]
touches: [.github/workflows/ci.yml, docs/decisions/0026-the-ui-api-answers-only-loopback-origins.md, docs/decisions/README.md, docs/operations.md, docs/epics/rehearsal-walls.md, docs/epics/fakeaws-rehearsal-instance.md, docs/hld/2026-09-30-import-production-state-into-the-mocks.md]
risk: high
---

# Pin fakeaws for the rehearsal instance; amend ADR-0026; operations doc; close the epic

One infrafactory PR, last in the epic, so the shared infrafactory files are edited once. docs/decisions and ci.yml are outside DECISION_PREFIXES, so no ADR trailer is needed.

- **Pin.** Bump FAKEAWS_SHA (.github/workflows/ci.yml:226) to a fakeaws main commit containing the five fakeaws stories' merges.
- **ADR-0026.** Add a dated amendment: a fakeaws started with an admin token refuses a non-loopback, null or lookalike Origin on /mock/* with 403 before the 401 token check, and allows an absent Origin. The guard is re-implemented in fakeaws, not imported. The amendment cites the tests: `TestContract_mock_admin_origin_loopback_only`, `TestContract_mock_admin_token_required`, `TestContract_listen_addr_default_all_interfaces`. Title and status stay; run `make adr-index`.
- **operations.md.** At docs/operations.md:432, describe `-bind`, `-admin-token` / `FAKEAWS_ADMIN_TOKEN`, `-record-requests` (needs a token), GET /mock/requests, POST /mock/requests/clear, the five classes and the 0600 files. Say 12 services, not 10.
- **HLD.** Rewrite the HLD's request-log sentence (lines ~196-198): the log is in process and behind `-record-requests`, not the `audit` table; /mock/state audit carries no params.
- **rehearsal-walls.** Add `-record-requests` to the fakeaws start line in docs/epics/rehearsal-walls.md:12, and to the binary check (a binary without `-bind`/`-admin-token`/`-record-requests` is refused).
- **Close.** Delete docs/epics/fakeaws-rehearsal-instance.md, and replace its HLD `## Epics` line (478) with the plain slug `- fakeaws-rehearsal-instance — done (#…)`, with no link.

**Done when:**
- `git merge-base --is-ancestor` shows each of the five fakeaws story merges in FAKEAWS_SHA, and the "AWS against fakeaws" job passes with .github/fakeaws-e2e-tests unchanged, each name printing its PASS line (the unflagged default is unchanged).
- The ADR-0026 amendment names the three TestContract ids, each of which exists as a func at the pinned fakeaws SHA; `make adr-index` leaves docs/decisions/README.md unchanged.
- docs/operations.md names every flag, route, class and the 0600 files above, and says 12 services.
- The HLD no longer says the request log is the audit table; rehearsal-walls.md's start line and binary check name `-record-requests`.
- The epic file is gone and the HLD line reads `fakeaws-rehearsal-instance — done (#…)` with no link.
- `make doc-hygiene` passes (the docs link test included).
