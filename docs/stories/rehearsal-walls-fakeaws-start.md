---
kind: code
status: blocked
blocked_by: [rehearsal-walls-adr]
epic: rehearsal-walls
depends_on: [rehearsal-walls-adr]
touches: [internal/cli/rehearse_fakeaws.go, internal/cli/rehearse_fakeaws_test.go]
risk: high
---

# Start a rehearsal's fakeaws on a free loopback port in its own process group, prove it is ours, stop it

New file `internal/cli/rehearse_fakeaws.go` with `rehearsalStartFakeaws(ctx, opts{bin, dir, account, region}) (rehearsalFakeaws{PID, Port, Token}, error)` and `(rehearsalFakeaws) stop()`.

It makes dir 0700 and a 32-byte random hex token, picks a port by listening on 127.0.0.1:0 and closing, then execs bin directly with exec.Command, never exec.CommandContext. It does not use `go run` or the batch runner, which cannot host a long-lived child (exec_runner.go:222-300). The args are exactly `-bind 127.0.0.1 -port P -db <dir>/fakeaws.db -record-requests -account A -region R`. The env is exactly {FAKEAWS_ADMIN_TOKEN=token}. It sets Setpgid, stdin /dev/null, and stdout/stderr to <dir>/fakeaws.log at 0600.

Then it waits, up to a readiness deadline, for: /healthz 200; GET /mock/requests 200 with `Authorization: Bearer <token>` and 401 without; and, when the host has a non-loopback address, no answer from that address on P (a refused dial or a dial timeout both count as no answer). ctx is watched only during this wait. prepare's ctx is cancelled by notifyGuarded's stop() as soon as prepare returns (exec_runner.go:135-146, rehearse_prepare.go:91), so the started fakeaws must outlive it.

A child that exits before it is ready because it could not bind (the log tail shows `address already in use`, fakeaws main.go ListenAndServe) is retried on a new port, 3 tries in all. Any other early exit is refused at once with its exit code and the log's tail. The deadline or a ctx cancel stops the child and returns the error with the log's tail. stop() sends SIGTERM to the group, waits, then sends SIGKILL to the group after a fallback. The readiness deadline and the kill fallback are package vars that tests shorten.

Tests use stubs that are 0700 `/bin/sh` scripts in t.TempDir(). Each writes "$@" and `env` to a file, then runs `exec <os.Args[0]> -test.run=^TestRehearsalFakeawsStub$` with its role baked into the script (exported after env is recorded), so the pid stays the group leader. TestMain (aws_ami_resolve_test.go:27) is not edited.

**Done when:**
- A happy-path test: the stub records its argv and env, and they equal the exact lists, the token in env only. The stub's pid is its own pgid. Port P answers on 127.0.0.1, and /mock/requests with the token is 200.
- After a successful start, cancelling ctx leaves the group alive (kill(-pgid,0) succeeds) until stop(), after which kill(-pgid,0) fails. Starting with exec.CommandContext turns it red (cp-backup).
- Each of these stubs is refused, and no stub process remains (kill(-pgid,0) fails): one that exits 2 on an unknown flag, refused after 1 try naming exit code 2 and the log tail; one that answers /mock/requests 200 without the token; one with no /mock/requests (recording off); one that never becomes healthy, refused at the shortened deadline. Removing the 401 check turns the no-token case red.
- One stub listens on 0.0.0.0. When the host has a non-loopback address, it is refused. When it has none, the test calls t.Skip naming that reason; it never passes silently.
- A stub that fails to bind on its first port (logs address in use and exits) is admitted on the second, and one failing to bind three times is refused, naming 3 tries. Retrying on every early exit turns the unknown-flag case red (it would count 3 tries).
- Cancelling ctx during the wait returns a context.Canceled-wrapping error, and the stub group is gone.
- Checks before every stage in docs/epics/rehearsal-walls.md hold.

**From review (binding; overrides the scope above where they differ):**
- Stubs are shell scripts re-exec'ing the test binary with -test.run and a baked role; a bare re-exec would die on `-bind` in m.Run's flag parse (skeptic).
- Start with exec.Command, never CommandContext; watch ctx only while waiting; prove the group survives a cancel after start (skeptic).
- Retry only on a bind failure; any other early exit is refused at once with its exit code and log tail (skeptic).
- The readiness deadline and kill fallback are injectable (skeptic).
- The 0.0.0.0 case skips with a named reason when no non-loopback address exists, and a dial timeout counts as not answering (skeptic).
