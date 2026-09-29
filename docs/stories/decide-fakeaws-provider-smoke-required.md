---
kind: operator
status: ready
---

# Decide: make fakeaws's `provider-smoke` a required check on fakeaws `main`

The user's call. `provider-smoke` runs every example against the real hashicorp/aws provider and
has caught real gaps, but it takes about 30 minutes, so every fakeaws PR would wait that long.
If yes, the lead adds it to the fakeaws ruleset's required checks.

**Done when:** the decision is made; if yes, `gh api repos/redscaresu/fakeaws/rulesets` lists
`provider-smoke` as required. This file is deleted either way.
