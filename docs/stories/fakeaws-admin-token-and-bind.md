---
kind: code
repo: fakeaws
status: ready
epic: fakeaws-rehearsal-instance
depends_on: []
touches: [cmd/fakeaws/main.go, cmd/fakeaws/main_test.go, handlers/handlers.go, handlers/admin.go, handlers/admin_test.go, handlers/regression_test.go, testutil/testutil.go, AGENTS.md, concepts.md, README.md, SECURITY.md, CHANGELOG.md]
risk: high
---

# NewApplication takes Options; -bind, -admin-token and a loopback-Origin guard on /mock

**Options.** `handlers.Options{DBPath, Echo, Bind, Port int, AdminToken}` replaces `NewApplication(dbPath, echo)` (handlers.go:36) at its 4 callers: main.go:24, testutil/testutil.go:35, admin_test.go:268, regression_test.go:1148. `Options.ListenAddr()` returns `net.JoinHostPort(Bind, strconv.Itoa(Port))`, so an empty Bind keeps today's `:port` (main.go:30). The bind invariant's docstring sits in handlers/, the contract audit's scope (contract_audit_test.go:45-47).

This story introduces the Options struct. fakeaws-loaded-identity's first code story must list `depends_on: [fakeaws-admin-token-and-bind]` and add Account/Region to Options, not change the constructor again.

**main.** Split main into a testable `optionsFromFlags(args []string, getenv func(string) string)` (or `run`) that returns the Options; main listens on `ListenAddr()`. Flags: `-bind`, `-admin-token`, else env `FAKEAWS_ADMIN_TOKEN` (a flag value shows in ps). The flag beats the env. The startup log says whether a token is set and never prints it.

**Guard.** When a token is set, one `mr.Use` in registerAdminRoutes (admin.go:48-57) checks Origin first, then the token. The Origin check ports infrafactory internal/api/origin_guard.go:114-134: an absent Origin is allowed; null, non-loopback and lookalikes are 403. Then Bearer via crypto/subtle, else 401. With no token there is no middleware at all. Stdlib only: open dependabot #46 holds go.mod.

**Done when:**
- All 4 callers build against `NewApplication(handlers.Options)`; `go test ./...` passes with existing admin tests unchanged except the constructor call.
- `TestContract_listen_addr_default_all_interfaces`: `Options{Port: 8082}.ListenAddr() == ":8082"`; Bind 127.0.0.1 with port 0 gives a listener on 127.0.0.1; Bind ::1 gives `[::1]:port`.
- A cmd/fakeaws test of optionsFromFlags: `-bind 127.0.0.1 -port 0` yields a listener on 127.0.0.1; `-admin-token x` sets the token; the flag beats FAKEAWS_ADMIN_TOKEN; the env alone sets it; no flag and no env leaves the token empty and the addr `:8082`. The startup log line never contains the token.
- `TestContract_mock_admin_token_required`: with AdminToken set, every /mock route (the 6 at admin.go:50-55 plus an unknown /mock/x) is 401 with no Authorization, a wrong token or a non-Bearer scheme, and the body names no token; with the right token each answers as before; /healthz and provider routes are not gated.
- `TestContract_mock_admin_origin_loopback_only`: with AdminToken set, Origin `https://evil.example`, `null`, `http://localhost.evil` and `http://127.0.0.1.evil` are 403 even with the right token; absent Origin, `http://127.0.0.1:N`, `http://localhost:N` and `http://[::1]:N` pass (cases from infrafactory internal/api/origin_guard_test.go:25-161).
- Without AdminToken, /mock/state and an unknown /mock/x with a non-loopback Origin and no Authorization answer the same status and body as origin/main daa0379 (a test).
- Each new test is shown failing by a cp-backup break of the code it covers, recorded in the PR.
- AGENTS.md Current contracts (line 151) lists the 3 ids and TestAgentsMDListsEveryContract passes. Also updated: the AGENTS.md Layout main.go line, the admin.go:44-47 "no auth" comment, concepts.md item 7, SECURITY.md, CHANGELOG [Unreleased], and README flag rows for `-bind`, `-admin-token` and `FAKEAWS_ADMIN_TOKEN` (README.md:86-97), with the docker note that `-bind 127.0.0.1` inside a container makes the published port unreachable.
