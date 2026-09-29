# Codex review pass 119 — S162b

One finding, **P1**, and the highest-severity defect of the session.

## [P1] A successful deploy would have been reported as a failure

`--output json` emits `{"schema": ..., "result": {...}}`. I unmarshalled stdout
straight into the inner `OutputResult`.

That does not error. Unknown keys are ignored, so the parse **succeeds with every
field zero** — `Status` empty, no stages, no failures. `Clean` is then false, and
the endpoint answers **409 after the infrastructure was created**.

The operator's reading of that is "the deploy failed", while a project, an
instance and a load balancer are running and billing. It is the exact inversion of
the property this whole arc is built around, and it would have been found by the
first real deploy — which is to say, by spending money.

## A parse that cannot fail is worse than one that does

There was nothing to notice: no error, no panic, no empty-string check. The fix
requires the envelope and treats anything else as a failure saying *whether
infrastructure was created is unknown* — because the apply may well have created
some, and an empty result would say the opposite.

## The test was written to my assumption, not to the system

`TestDeployOutcomeReadsTheCommandsOwnVerdict` marshalled a bare `OutputResult` —
the shape I *believed* the command emitted — and passed. A test that constructs
its own input from the same wrong premise as the code confirms the premise.

This is the third time this session: S156d's synthetic Terraform address, S161's
banner-only assertion, and now this. The common shape is a test whose fixture is
built from the implementation's assumptions rather than from what the other side
actually produces. **The fixture has to come from the real producer**, and here
that meant marshalling `MachineOutput`.
