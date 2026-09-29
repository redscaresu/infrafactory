# Review pass 131 — S163, by three fresh-context agents

Codex is out of quota until 2026-09-07. On the user's instruction the review was
run by three fresh-context subagents with separate lenses — correctness,
truthfulness-to-the-reader, and an adversarial pass on the tests, that last one
primed with this session's three cases of a fixture built from the same wrong
assumption as the code.

The caveat stated at the time still holds: a fresh-context agent is independent of
my *reasoning* but shares my *model*. Codex caught two P1s partly by being a
different model.

## The finding that mattered: the streaming did not stream

Two of the three found it independently, and I verified it directly.

`LiveDeployer` tee'd `cmd.ErrOrStderr()`. `runDeployCommand` writes to that stream
exactly three times: twice before any cloud work, once after the apply returns.
Nothing in between touches it — `ensureRunProject` never prints, and
`SandboxDeployHarness.Run` executes `tofu init/plan/apply` through
`CommandRunner.Run`, which returns a **fully buffered** `CommandResult`. The
output does not exist until each process exits.

So the shipped behaviour was: two lines, silence for the whole apply, one line.
**Worse than before**, because a terminal-styled log pane that has stopped moving
is a stronger "hung" signal than the button label beside it. The exact reader-state
ADR-0027 says the slice prevents.

### The tests could not have caught it, and that was demonstrated

The reviewer replaced the live tee with buffer-then-dump and ran the suite: green.
The fake deployer writes its whole fixture in one synchronous call, so it cannot
distinguish "streamed during" from "dumped at the end". Both e2e tests intercept
the POST in the browser, so the server never broadcasts at all — mutating
`resetDeployState` to leave the socket open, and typo-ing the event type to kill
the stream entirely, both passed all 19.

**Fourth instance this session of a fixture built from my own assumption**, and
the first that hid a feature which simply did not function.

## What the fix actually is

The harness is the only thing that knows a stage has *begun*, so it has to be the
one reporting. `SandboxDeployHarnessRunner.Run` takes an `io.Writer` — a parameter
rather than a field, because the harness is built once and shared and two deploys
must not write into each other's stream.

Stage granularity, not raw tofu output: it is what the plan asked for
("init → plan → apply → register should scroll"), it is a predictable handful of
lines, and it does not drown a terminal.

**Retries are now visible.** A silent retry is indistinguishable from a slow
stage, and a Layer 3 apply really does retry after a transient provider error.

`TestProgressIsVisibleWhileTheApplyIsStillRunning` asserts what a watcher can see
**from inside the apply**, which buffer-then-dump cannot satisfy. Verified by
mutation: removing the stage reporting fails it.

## Also fixed, all confirmed

- **`deploying = false` in `resetDeployState` let a second real deploy start.**
  Navigate away mid-deploy and back, and the button was enabled again — two
  projects, double billing. A regression this slice introduced.
- **`confirmDeploy`'s `finally` had no ownership guard.** An earlier deploy
  finishing cleared a later one's state, freezing its log and putting a green
  success message belonging to the first beneath it. Guarded by generation now.
- **The last line raced the HTTP response.** `deployingScenario` was cleared
  synchronously when the response landed, so the `Deployed as dep-…` line — the id
  needed for `live teardown` — was filtered out. Cleared on navigation instead.
- **A dropped socket rendered identically to "nothing has happened yet."** Now
  distinguished: "not receiving progress" rather than "Starting…".
- **The progress filter had no test**, because it was inline in the component and
  no test ever reached it. Extracted to `$lib` and tested, including the typo that
  killed the stream silently.
- **The nil-progress test never reached the nil handling.** Its factory always
  failed first, so deleting the guard entirely still passed while production would
  panic. `deployStderr` is testable directly now.
- **`Close()`'s comment claimed a bug it does not fix.** `deploy` terminates every
  line, so Close flushes nothing for today's caller. It is a guarantee about the
  type, and now says so.

## The limit that stays, stated rather than implied

The event subject is the **scenario**, not the deployment id — the id is minted
inside the command, after the request is accepted. Two concurrent deploys of one
scenario therefore share a stream and a reader sees both. Written down in
`acceptProgressEvent`'s doc comment rather than left to be discovered.
