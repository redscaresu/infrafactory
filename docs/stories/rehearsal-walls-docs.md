---
kind: docs
status: blocked
blocked_by: [rehearsal-walls-adr, rehearsal-walls-fakeaws-pin, rehearsal-walls-env, rehearsal-walls-provider-override, rehearsal-walls-gate-admission, rehearsal-walls-fakeaws-start, rehearsal-walls-prepare-wiring, rehearsal-walls-prepare-e2e, rehearsal-walls-regate, rehearsal-walls-end, rehearsal-walls-fail-closed, rehearsal-walls-teardown-e2e]
epic: rehearsal-walls
depends_on: [rehearsal-walls-adr, rehearsal-walls-fakeaws-pin, rehearsal-walls-env, rehearsal-walls-provider-override, rehearsal-walls-gate-admission, rehearsal-walls-fakeaws-start, rehearsal-walls-prepare-wiring, rehearsal-walls-prepare-e2e, rehearsal-walls-regate, rehearsal-walls-end, rehearsal-walls-fail-closed, rehearsal-walls-teardown-e2e]
touches: [docs/operations.md, docs/decisions/0042-rehearsal-command-contract-and-walls.md, docs/epics/rehearsal-walls.md, docs/hld/2026-09-30-import-production-state-into-the-mocks.md]
---

# Runbook for the walls and `rehearse end`; ADR-0042 corrected to what was built; close the epic

The epic's last story; it closes rehearsal-walls.

docs/operations.md `## Rehearsal`:
- `### Prepare` gains --fakeaws-bin; the pinned install (`go install github.com/redscaresu/fakeaws/cmd/fakeaws@<pin>` into `<UserCacheDir>/infrafactory/fakeaws/<pin>`; it needs Go and the module proxy once per pin); the two override files, `infrafactory_override.tf` and `infrafactory_aws_override.tf`; the record, `rehearsal.json`; `fakeaws_url`; what a dead fakeaws means (the override points at a closed port; run prepare again); and the alias-conflict warnings every plan shows (`Only one of the following attributes should be set`).
- A new `### End`: the command, the token check before any signal, what it deletes, that it is idempotent, `stop failed` and its retry, and that a `rehearse read --out` outside `<id>/reads` is not the rehearsal's and is not deleted.
- `### The fakeaws instance` notes the rehearsal passes the token by env (`FAKEAWS_ADMIN_TOKEN`), never on argv.

ADR-0042 gets text corrections only where built behaviour differs from rehearsal-walls-adr, each listed in the PR.

Close the epic: delete docs/epics/rehearsal-walls.md, and change its line in docs/hld/2026-09-30-import-production-state-into-the-mocks.md `## Epics` from `- [rehearsal-walls](../epics/rehearsal-walls.md)` to `- rehearsal-walls — done (#<PR>, #<PR>, ...)`, listing every walls PR, in the style of the done lines above it.

**Done when:**
- Every flag that `infrafactory rehearse prepare --help` and `rehearse end --help` print appears in `## Rehearsal`; a loop in the PR checks this.
- A loop in the PR greps `## Rehearsal` of docs/operations.md for each of: `--fakeaws-bin`, `go install github.com/redscaresu/fakeaws/cmd/fakeaws@`, `infrafactory_override.tf`, `infrafactory_aws_override.tf`, `rehearsal.json`, `fakeaws_url`, `run prepare again`, `Only one of the following attributes should be set`, `before any signal`, `idempotent`, `<id>/reads`, `FAKEAWS_ADMIN_TOKEN`, `### End`. Each is found, and the PR shows each one red by deleting its line (cp-backup).
- `grep -nE '^- rehearsal-walls — done \(#[0-9]+' docs/hld/2026-09-30-import-production-state-into-the-mocks.md` prints one line, and `docs/epics/rehearsal-walls.md` does not exist.
- `grep -rn 'epics/rehearsal-walls' docs --include='*.md' | grep -v archive` is empty.
- Checks before every stage in docs/epics/rehearsal-walls.md hold (read from main before the file is deleted).

**From review (binding; overrides the scope above where they differ):**
- The grep loop for each required runbook fact, each shown red by deleting it, replaces the weaker help-flags-and-heading check alone (codex).
- This story closes the epic, taking that job from the load story, which moved to rehearsal-against-the-copy (critic). The done line is grepped exactly, since removing the link alone would pass the old check (codex).
