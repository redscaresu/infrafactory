---
kind: chore
status: blocked
blocked_by: [fakeaws-identity-sweep-and-docs]
epic: fakeaws-loaded-identity
depends_on: [fakeaws-identity-sweep-and-docs]
touches: [.github/workflows/ci.yml, docs/hld/2026-09-30-import-production-state-into-the-mocks.md, docs/epics/fakeaws-loaded-identity.md]
---

# Pin FAKEAWS_SHA to the fakeaws main that holds the stored identity, and close the epic

Bump the `FAKEAWS_SHA` line in .github/workflows/ci.yml (:226) to fakeaws main after fakeaws-identity-sweep-and-docs merged. The line is shared, so if fakeaws-rehearsal-instance-pin-and-docs landed first, the new pin must contain that epic's last merge too.

No infrafactory code consumes `-account`, `-region` or /mock/identity yet; the rehearsal epics do. So this adds no new e2e. The e2e tests that hard-code the default must stay green unchanged: internal/e2e/aws_preflight_fakeaws_test.go:19-20, aws_env_only_test.go:94, and aws_services_test.go:449,466.

Close the epic:
- In the HLD's `## Epics`, replace the fakeaws-loaded-identity link line with `- fakeaws-loaded-identity — done (#<PR>)`, as rehearsal-assumption-tests is marked.
- Delete docs/epics/fakeaws-loaded-identity.md.
- If the sibling pin story is open, rebase; it edits the same block.

The tip commit carries `ADR: none — fakeaws answers its default account unchanged; infrafactory consumes no new surface yet`.

**Done when:**
- `git -C ../fakeaws merge-base --is-ancestor <sweep-and-docs merge sha> <FAKEAWS_SHA>` exits 0. If the sibling pin landed first, the same check holds for fakeaws-rehearsal-instance's last merge.
- CI "AWS against fakeaws" is green at the new pin, with every test in .github/fakeaws-e2e-tests printing its own PASS line.
- The HLD Epics line reads done with the PR number, and docs/epics/fakeaws-loaded-identity.md no longer exists. `grep -rn 'epics/fakeaws-loaded-identity' docs STATUS.md` prints nothing.
- `make doc-hygiene` passes.
