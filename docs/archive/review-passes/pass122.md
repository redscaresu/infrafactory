# Codex review pass 122 — S162b

**Clean.** The endpoint is gated, it uses the CLI deploy path rather than
duplicating the real-cloud flow, and the safety cases are covered.

Four findings, **two of them P1** — the only P1s of the session, and both on the
slice that spends money. Both were the same mistake wearing different clothes:
**assuming what the other side does instead of reading it.**

- Pass 119 assumed the command's output shape. It emits
  `{"schema":…,"result":{…}}`; I unmarshalled the inner object. That parse cannot
  fail, so a *successful* deploy would have been reported as a 409 with the
  infrastructure running.
- Pass 120 assumed the runtime's lifetime. `LoadScenario` caches, so one runtime
  held for the life of the server deploys the first scenario and refuses every
  other one.

In both cases the answer was already open in front of me — `MachineOutput` in
`output_contract.go`, and `uiRunStarter` calling `buildRuntime` inside
`executeRun`, which is the very function this seam was written to imitate. I
copied its shape and not its behaviour.

The seam decision still looks right: driving the real command means the Layer 3
preflight, the credentials check, the per-deployment workdir, the interrupt-guarded
project creation and the write-the-record-even-on-failure step are the CLI's, not
a second copy. Both P1s were in the thirty lines of translation around it, which
is roughly where the risk should sit.
