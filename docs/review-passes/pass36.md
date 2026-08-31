# S166 cutover review pass 36 — clean (guard core)

`codex exec review --base main`:

> The changes add the run-project marker/provenance guard and focused tests
> without breaking existing behavior. I did not find a discrete correctness issue
> in the diff relative to the provided merge base.

The security-critical core, reviewed on its own before anything is wired to it:
`AssertRunProjectDeletable`, the marker read/write, and `Describe`'s
gone-vs-unreachable distinction. Nothing calls it yet, so existing behaviour is
untouched by construction.
