# Codex review pass 2 — doc-hygiene dependency carve-out

Base: `main` (390d059). Scope: `scripts/check_doc_hygiene.sh` carve-out,
its new test, CI wiring.

## Passes

| Pass | Findings | Note |
|---|---|---|
| 1 | none | "The dependency-manifest exemption is scoped to manifest-only changes, and the added self-tests exercise both the allowed and rejected cases." |
| 2 | none | "Cleanly add an all-or-nothing dependency-manifest exemption and pin it with focused shell tests." |

**Converged** — two consecutive clean passes.

## Note

Nothing to triage, which is the expected shape for a small change with its
own tests. Worth recording anyway: the loop is required on every PR, and a
converged-with-no-findings record is evidence the loop ran, not evidence it
was skipped.
