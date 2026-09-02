# Codex review pass 120 — S162b

One finding, **P1**, and the second in two passes on the same slice.

## [P1] A shared runtime deploys once and refuses everything after

`CommandRuntime.LoadScenario` **caches**: a runtime that has loaded one scenario
refuses a different path with *"scenario already loaded from …"*. That is correct
for a CLI process, which handles exactly one command and exits.

`LiveDeployer` held one runtime, built at startup, for the life of the server. So
the first deploy would work and every deploy of a different scenario would fail —
on a long-lived process, which is the only kind this endpoint runs in.

## The pattern I copied already solved it

The seam was written to mirror `uiRunStarter`, deliberately, so that the guards
came from the CLI rather than a second implementation. `uiRunStarter` calls
`buildRuntime` **inside** `executeRun` — per run, not per server. I copied the
shape of it and not that.

Both P1s in this slice are the same mistake in different clothes: assuming what
the other side does instead of reading it. Pass 119 assumed the output shape;
this assumed the runtime's lifetime. In both cases the answer was ten lines away
in code I had already opened.

## The startup build stays

Rebuilding per deploy removes the reason the startup build existed — failing
loudly when `--allow-deploy` cannot be honoured. So both happen: one probe at
startup so the operator learns from the command they typed rather than from a
click minutes later, and a fresh runtime per deploy.
