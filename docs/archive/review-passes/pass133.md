# Review pass 133 — S163 rework, adversarial-on-tests

Nine findings, every one **verified by mutation**: the reviewer broke the
production code and watched the suite stay green. Seven fixed, two recorded.

## The shape of all of it: units tested, wiring not

**F1** — `cmd.SetErr(&progressCopy)`, dropping the tee: whole Go suite green. The
UI deploy log would be permanently empty, which is the original bug restored.
**F2** — `Run(ctx, dir, env, nil)` at the deploy call site: green. Every stage line
vanishes for CLI and UI alike.

Both were invisible because `deployStderr` was tested in isolation and every test
calling `Deploy` used a runtime factory that failed *before* the writer was ever
touched. Each piece was covered; the path between them was not.

**F3** — the API streaming test asserts content, not timing: a 4KB `bufio.Writer`
between deployer and sink passed. The fixture writes and returns, so it cannot
express "during" — the same wrong assumption as a buffering implementation.

### One test closes all three

`TestStageProgressReachesTheWebsocketThroughTheRealChain` drives the whole path:
`LiveDeployer.Deploy` → `runDeployCommand` → the real `SandboxDeployHarness` →
`ProgressSink` → `Hub` → what a websocket client would receive, with only the
*subprocess* faked. It asserts what a browser could see **while the apply is still
running**.

Both of the reviewer's mutations now fail it, verified.

That required exporting two seams — `harness.CommandRunnerFunc` and
`api.NewTestClient` — and my first attempt at the test **skipped both broken
points**, going harness→sink directly. It looked like coverage and would have
closed nothing.

## The stream promising things it will not do

**F7** — `retrying` was pinned as a substring, so two mutations passed: announcing
a retry on the *final* attempt, and announcing one on the **interrupt path** —
where the operator has asked us to stop touching the API, and the last line they
see would promise the opposite.

Both now emit `giving up after N attempt(s)` instead, with tests that read the
*last* line rather than searching the whole log.

**F8** — the `FAILED` line's reason was unpinned, so dropping `%v` passed. A
stream that stops without saying why is what `finished`'s own docstring exists to
prevent.

## The UI had no test that rendered a progress line at all

**F4, F5, F6.** Every earlier e2e intercepts the POST in the browser, so the
server never broadcasts. Replacing the whole `{#each}` with a constant string,
hiding the disconnected banner entirely, and deleting the Live-page filter all
passed the full suite.

`page.routeWebSocket` lets the test be the server for the socket, so real frames
reach the real filter and the real DOM. Three tests now cover rendering, subject
scoping, and disconnected-vs-quiet; a fourth covers the Live-page leak. All three
mutations fail, verified.

## Recorded, not fixed

- **`forgetDeploy` has no production caller.** A finished deploy's log and its
  success banner reappear on every later visit to that scenario for the rest of
  the session, long after the TTL expired. The store reads as self-cleaning and is
  not.
- **A page reload defeats the in-flight guard.** The store is module state with no
  server-side lock and no rehydration, so reloading during a deploy re-enables the
  button and a second run-owned-project deploy is one click away. My own comment
  in `deploy-store.js` claims it is "the only thing standing between a reader and
  that second deploy" — true, and it does not survive a refresh.

Both are real and neither is a test problem. The second wants a server-side
in-flight lock, which is a design decision rather than a patch.
