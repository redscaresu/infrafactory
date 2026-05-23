<!--
PR template. Delete sections that aren't relevant. Keep the test plan.
-->

## Summary

<!-- One paragraph: what changed and why. -->

## Ticket(s)

<!-- e.g., closes S52-T3, refs M41, or "no ticket — drive-by fix". -->

## Test plan

- [ ] `make test` green locally
- [ ] Pre-commit hook (gitleaks + go test) passed
- [ ] New tests cover the behavior change (if applicable)
- [ ] Manual smoke (if UI / e2e / cross-cloud): <!-- describe -->

## Docs touched

- [ ] `STATUS.md` updated
- [ ] `BACKLOG.md` ticket status flipped
- [ ] `CONCEPT.md` updated (if architecture/contract changed)
- [ ] ADR added/updated under `docs/decisions/` (if decision-impacting)
- [ ] N/A — pure refactor / dependency bump

## Reviewer checklist

- [ ] Single focused change (not a feature + refactor + bump combo)
- [ ] No secrets in diff (`gitleaks protect` should have caught this; double-check anyway)
- [ ] Error messages are explicit and actionable
- [ ] No new dependencies added without justification

🤖 Generated with [Claude Code](https://claude.com/claude-code) — `Co-Authored-By:` trailer expected on AI-assisted commits per CONTRIBUTING.md § AI-assisted work.
