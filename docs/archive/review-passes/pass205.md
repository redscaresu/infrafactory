# Review pass 205 — S194: the pitfall that forbade a firewall

`codex exec review --base main`, 2026-09-27.

## Result

**Clean on the first pass. No findings.**

> No correctness issues were found in the changed code. The new policy, mapping, allowlist
> update, scenario criterion, and focused tests appear consistent with the intended
> behavior.

The review exercised the diff rather than only reading it — it ran
`go test ./internal/feedback ./internal/generator ./internal/cli -run
'Pitfall|DefaultDeny|Layer3|Coverage|Scenario'` (all green) and read `scenario.schema.json`
to check the new `type: policy` criterion against the schema's conditional branch.

## Declines

None. Nothing was raised.

## What the audit tests caught before review did

Worth recording, because it is the mechanism from ADR-0028 working as designed. Two
pre-existing tests failed on the first full run of this slice and both were right:

- `TestLayer3CoverageDocAllowlistMatchesConfig` — the allowlist is written down in a THIRD
  place, `docs/layer3/coverage.md`, which I had not updated. Fixed.
- `TestPitfallsLearnedFromDiffSnippetCap` — the rewritten pitfall was 1786 bytes against a
  1000-byte cap. The cap is correct: a pitfall is injected into every generation prompt, so
  the reversal's history is pure token cost there. Trimmed to the actionable shape and the
  history moved to the config comments and ADR-0034.

Neither was found by reading the diff. Both were found by running it.
