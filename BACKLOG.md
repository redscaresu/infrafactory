# BACKLOG

Single source of ticket status across slices.

Legend: `todo` | `in_progress` | `blocked` | `done`

| id | slice | title | status | deps | owner |
|---|---|---|---|---|---|
| S1-T1 | Slice 1 | Wire Cobra root and commands (`init`, `generate`, `validate`, `test`, `run`, `mock start`) | in_progress | - | codex |
| S1-T2 | Slice 1 | Implement `internal/config` loader with defaults and required-field validation | todo | S1-T1 | codex |
| S1-T3 | Slice 1 | Implement `internal/scenario` loader + JSON Schema validation | todo | S1-T1 | codex |
| S1-T4 | Slice 1 | Add Slice 1 focused tests (config/scenario valid+invalid paths) | todo | S1-T2,S1-T3 | codex |
| S2-T1 | Slice 2 | Define `SeedGenerator` interface and generator contracts | todo | S1-T4 | codex |
| S2-T2 | Slice 2 | Implement prompt rendering helpers and feedback context injection | todo | S2-T1 | codex |
| S2-T3 | Slice 2 | Implement `# File:` parser with code-fence stripping + duplicate handling | todo | S2-T1 | codex |
| S2-T4 | Slice 2 | Add generator/parser focused tests | todo | S2-T2,S2-T3 | codex |
| S3-T1 | Slice 3 | Implement static harness (`tofu init/validate/plan/show -json`) | todo | S2-T4 | codex |
| S3-T2 | Slice 3 | Integrate OPA evaluation against plan JSON | todo | S3-T1 | codex |
| S3-T3 | Slice 3 | Add structured static-layer failure reporting + tests | todo | S3-T1,S3-T2 | codex |
| S4-T1 | Slice 4 | Implement mock deploy orchestration (`tofu apply`, mock reset/state client) | todo | S3-T3 | codex |
| S4-T2 | Slice 4 | Add topology checks and state policy checks in harness | todo | S4-T1 | codex |
| S4-T3 | Slice 4 | Add mock deploy layer tests (opt-in where external deps required) | todo | S4-T2 | codex |
| S5-T1 | Slice 5 | Implement destroy flow + orphan verification | todo | S4-T3 | codex |
| S5-T2 | Slice 5 | Implement run store persistence on disk | todo | S5-T1 | codex |
| S5-T3 | Slice 5 | Add destroy/run-store tests | todo | S5-T2 | codex |
| S6-T1 | Slice 6 | Implement feedback loop + max-iteration control | todo | S5-T3 | codex |
| S6-T2 | Slice 6 | Implement stuck detection using failure-signature subset logic | todo | S6-T1 | codex |
| S6-T3 | Slice 6 | Implement criteria-only holdout flow | todo | S6-T2 | codex |

## Operating notes
- Update `status` and dependencies as work evolves.
- Keep exactly one `in_progress` ticket at a time.
- Use `CURRENT_TICKET.md` for session-level execution details.
