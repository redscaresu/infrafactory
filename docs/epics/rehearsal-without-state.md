---
status: active
hld: 2026-09-30-import-production-state-into-the-mocks
depends_on: [rehearsal-against-the-copy]
---

# New code with no tfstate (it only adds alongside prod) rehearses from an empty starting state: it finds the real VPC by tag, adds a subnet and imports a real security group against the copy (Rollout 4, after the state path).

**Done when:**
- A rehearsal with no `--backend-credentials` starts from an empty state; `import` blocks resolve against the real ids the copy holds; the base step is skipped when no base is given, and the report says so.
- The e2e runs with a second fakeaws as prod, loaded with the scrubbed fixture plus an unrelated planted VPC: the recorded read holds the tagged VPC, its surroundings and the imported SG, and not the unrelated VPC; plan and apply exit 0; the SG keeps its real id.
- ADR-0042 amended; golden files; docs/operations.md.

**Out of scope:** everything rehearsal-against-the-copy already owns (it reuses discovery, read, load, check, classifier and report).

**Constraints:** no second copy of any step; reuse only.

**Your part:** none.

**Areas:** `internal/cli/ (rehearse_*.go)`; `internal/e2e/`; `docs/decisions/0042`; `docs/operations.md`
