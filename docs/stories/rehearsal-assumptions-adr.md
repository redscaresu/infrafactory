---
kind: docs
status: blocked
blocked_by: [rehearsal-override-assumptions, rehearsal-encryption-assumptions, rehearsal-backend-init-assumptions, rehearsal-refresh-discovery-assumptions]
epic: rehearsal-assumption-tests
depends_on: [rehearsal-override-assumptions, rehearsal-encryption-assumptions, rehearsal-backend-init-assumptions, rehearsal-refresh-discovery-assumptions]
touches: [docs/decisions/0028-factual-claims-are-tests-not-comments.md, docs/decisions/README.md, docs/hld/2026-09-30-import-production-state-into-the-mocks.md, docs/epics/rehearsal-walls.md, docs/epics/rehearsal-prepare-gate.md, docs/epics/rehearsal-state-pull.md, docs/epics/rehearsal-against-the-copy.md, docs/epics/rehearsal-assumption-tests.md]
---

# ADR-0028 lists the rehearsal's tofu claims, their tests and verdicts; the relying epics name their tests; the epic closes

This is the only story that edits ADR-0028. It adds a dated amendment:
- **The claims table:** one row per claim (a)–(g). Each row gives its TestAssumption_* name(s), the HLD section it backs, the relying epic, and a verdict cell of `true` or `false` (with what tofu actually does when false).
- **Refinements recorded:**
  - (d) runs at a backend-initialising init, not at `init -backend=false`.
  - (b) holds because the command-line path wins.
  - (c) is "no proxied connection": direct dials are beyond what the test can observe, and (c) depends on a pinned third-party registry module.
- **The file exception:** gated claims live in `rehearsal_assumptions_test.go` as package cli_test, because internal/e2e imports internal/cli.
- **The bounds:**
  - the provider is `harness.AWSProviderVersion`, not a lock file
  - CI's tofu floats, so a tofu release can turn main red with no repo change

Title and status stay unchanged; run `make adr-index`.

**Rewording the HLD.** The HLD's "Prepare" sentence "That this second init opens no socket" becomes "makes no proxied connection".

**Naming tests in the relying epics.** Add each test name to the **Constraints** of its relying epic (rehearsal-walls, rehearsal-prepare-gate, rehearsal-state-pull, rehearsal-against-the-copy), so the relying site's comment can cite it (ADR-0028: citation both ways).

**Closing the epic.**
- **If every verdict is `true`:** delete `docs/epics/rehearsal-assumption-tests.md`, and replace its HLD `## Epics` line with the plain slug `- rehearsal-assumption-tests — done (#…)`, with no link.
- **If any verdict is `false`:** keep the epic, and add a `kind: operator` story (`epic: rehearsal-assumption-tests`) naming the false claim and the HLD sections for the user to revise.

**Done when:**
- Each of the new names appears both as a `func` in `internal/cli/*assumptions_test.go` and in the ADR. The names, or the renames a code PR records:
  - EndpointsOverrideKeepsApplyOffProd
  - UserOverrideMergesAfterOurs
  - RootOverrideMissesChildModuleProvider
  - ExternalKeyProviderRunsAtBackendInit
  - StatePullEmitsDecryptedState
  - CLIBackendPathKeepsStateOutOfTheCopy
  - SecondInitMakesNoProxiedConnection
  - RefreshOnlyUnreadableDataSourceExitsOne
  - DataSourcesAllSendBeforePlanFails
- The amendment table has one row per claim (a)–(g), each with a verdict of `true` or `false`.
- Each name appears in its relying epic's Constraints.
- `go test ./cmd/adr-index` passes, and `make adr-index` leaves docs/decisions/README.md unchanged.
- `make doc-hygiene` passes (the docs link test included).
- If every verdict is `true`, the epic file is deleted and the HLD line is the plain slug with `— done (#…)`. Otherwise the epic stays and a `kind: operator` story exists naming the false claim.
