# S165 review pass 21 — one finding, acted on

### [P2] The unwired `create_run_project` key was silently accepted

Pass 20 had me stop advertising the flag in `infrafactory.yaml`. Codex pointed
out the sharper version: the struct field means `Load` now *accepts*
`create_run_project: true` from an operator's own config, with no error and no
effect. They would believe Layer 3 was using a run-owned project while resources
kept landing in the shared fallback — precisely the outcome ADR-0025 exists to
fix.

Right, and not the same finding twice: pass 20 was about advertising, this is
about acceptance. `validate` now refuses `create_run_project: true` outright
while the flag is unwired, with an error saying why. The check is written to be
deleted in the same commit that honours the flag, and
`TestCreateRunProjectIsRefusedUntilWired` pins the behaviour until then.

Also in this increment: `sandboxCommandEnvForProject`, the seam ADR-0025 needs.
`sandboxCommandEnv` delegates to it with an empty project, so the pre-ADR-0025
path is unchanged by construction. The organization-default refusal deliberately
applies to a run-supplied project too — the check is about where strays land, and
that does not change with where the project id came from. The test suite caught
me on this: my first version of the test reused the fixture's organization id as
the run project and the guard refused it, which is the guard working.
