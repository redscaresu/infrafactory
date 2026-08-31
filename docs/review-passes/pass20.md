# S165 review pass 20 — one finding, acted on

### [P2] `create_run_project` was a silent no-op

The first increment added the config field *and* advertised it in
`infrafactory.yaml`, while nothing in the runtime reads it. An operator setting
it to `true` would get no run-owned project and no error — resources without
`project_id` would still land in the shared fallback, which is the exact failure
ADR-0025 exists to fix.

Correct, and acted on by narrowing rather than by rushing the wiring: the field
stays (the client needs somewhere to hang off), but it is no longer advertised in
`infrafactory.yaml`, and `TestCreateRunProjectIsNotYetWired` fails the moment it
is — so the "not yet honoured" comment cannot outlive its own truth.

Env plumbing is the next increment. Deliberately not rushed into this commit:
S165's wiring touches `sandboxCommandEnv`, which every Layer 3 path shares.
