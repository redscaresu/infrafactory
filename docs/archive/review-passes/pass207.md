# Review pass 207 — S196: fast pre-commit, ADR trailer

`codex exec review --base main`, four passes, 2026-09-27. Three findings, all accepted; converged clean.

1. **P3, accepted.** The touched-package list used `--diff-filter=ACMR`, so a commit that only deleted a Go file skipped that package's tests. Now `--no-renames` with no filter, dropping directories that no longer exist.
2. **P2, accepted.** A trailer anywhere in the range waived the ADR check for every commit, so an early comment-only commit could cover a later contract change.
3. **P2, accepted by replacement.** The per-commit fix for (2) missed decision-path edits carried by merge commits. Rather than patch the per-commit walk a second time, it was replaced: the trailer must be on the tip commit, declared with the whole change in view, and a tip without it (merge commits included) fails closed. That design closes (2) and (3) with less code.
