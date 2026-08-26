# Recorded generation

The HCL in `block-paris/` was written by the LLM, not by hand:

    infrafactory generate scenarios/training/block-paris.yaml

It is committed so the on-stage path does not have to make a 40–60s model
call with real variance in front of a live audience — the decision recorded
in `docs/plans/presentable-arc-plan.md` (keep the LLM out of the live path,
run the verification half live).

`make demo-gate` replays this by default. `make demo-gate GENERATE=live`
regenerates it instead, which is the honest thing to do when the network and
the schedule allow.

Refresh it with `make demo-gate GENERATE=live` and commit the result. If the
generator's output stops passing the Layer 3 preflight, that is worth knowing
before a talk rather than during one — `TestRecordedGenerationPassesPreflight`
fails in CI when it does.
