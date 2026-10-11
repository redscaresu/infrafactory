---
kind: code
repo: fakeaws
status: blocked
blocked_by: [fakeaws-owner-only-files]
epic: fakeaws-rehearsal-instance
depends_on: [fakeaws-owner-only-files]
touches: [handlers/recorder.go, handlers/recorder_test.go, handlers/handlers.go, handlers/admin.go, cmd/fakeaws/main.go, cmd/fakeaws/main_test.go, handlers/regression_audit_test.go, AGENTS.md, concepts.md, README.md, CHANGELOG.md]
risk: high
---

# -record-requests: an in-process request log with status and classification, /mock/requests and clear

**The flag.** `-record-requests` (Options.RecordRequests) is off by default. fakeaws refuses to start with `-record-requests` and no admin token (flag or FAKEAWS_ADMIN_TOKEN): the log will hold request values.

**The recorder.** New handlers/recorder.go: one root middleware, installed only when recording, and installed before `middleware.Recoverer` (handlers.go:48) so it sees Recoverer's 500. It wraps the ResponseWriter to catch the status and a class marker. It skips /mock/ and /healthz, so the router's NotFound/MethodNotAllowed 501s (handlers.go:107-108) are recorded. Entry: `{method, path, status, classification}`.

**Classes.** A marker helper on the writer sets the class (a no-op when not recording). writeUnimplemented (handlers.go:120) marks unimplemented-501 as a default: the last explicit marker wins, so a site that calls writeUnimplemented and then marks another class (fakeaws-request-log-gap-markers' SSM filter sites) overrides it. Unmarked: a status >= 500 records unimplemented-501, anything else answered-from-rows. A 5xx is never an answer from rows.

**Storage.** A mutex-guarded slice on Application, not SQLite: the audit table has no ALTER path, a write per request would serialise behind SetMaxOpenConns(1), and the db and snapshot must never hold params. /mock/reset clears it; /mock/snapshot and /mock/restore leave it; a restart empties it. `GET /mock/requests` and `POST /mock/requests/clear` exist only when recording. collectState's audit (admin.go:259-262) reads the log, but /mock/state audit only ever carries `{service, protocol, action, status, class}`, never params or values (admin.go:33 promises no values; infrafactory writes /mock/state into run artifacts). This story fills service (from the route prefix), status and class; fakeaws-request-log-read-set fills protocol and action.

**Done when:**
- `TestContract_request_log_off_by_default`: without RecordRequests, GET /mock/requests and POST /mock/requests/clear answer the router 501 exactly as on daa0379, /mock/state audit is [] after provider traffic, and nothing is recorded.
- A cmd/fakeaws test: `-record-requests` without a token exits non-zero naming the missing token; with `-admin-token` it starts.
- With RecordRequests: provider requests append in completion order; /mock/* and /healthz never appear; clear empties the log; /mock/reset empties it; snapshot and restore leave it unchanged; a second NewApplication on the same db file starts empty.
- `TestContract_request_log_unimplemented_501`: an unknown path (NotFound), GET /ec2/region/us-east-1 (MethodNotAllowed), STS AssumeRole (sts.go:42) and an unknown SSM target (ssm.go:56) record unimplemented-501; an unmarked test route answering 503 records unimplemented-501; a test route that marks unimplemented-501 and writes 200 records unimplemented-501, so the class does not come from the status; a route marking unimplemented-501 then another class records the later one.
- `TestContract_request_log_panic_recorded`: a panicking test route is recorded with status 500 and never as answered-from-rows.
- `TestContract_state_audit_no_values`: each /mock/state audit entry has exactly the keys service, protocol, action, status, class.
- 50 concurrent provider requests under `go test -race` are all recorded. With AdminToken set, /mock/requests and clear are 401 without the bearer.
- recorder.go is in knownNonServiceFiles (regression_audit_test.go:232-237) and TestRegressionSeedAuditManifestMatchesHandlers passes.
- Each new test is shown failing by a cp-backup break, recorded in the PR.
- AGENTS.md line 151 and Layout lines, concepts.md item 7, a README flag row for `-record-requests` (README.md:86-97) and a CHANGELOG [Unreleased] entry for the flag are updated.
