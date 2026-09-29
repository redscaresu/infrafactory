---
kind: operator
status: ready
---

# Decide: add an `OPENROUTER_API_KEY` secret so the scenario gate runs the model

The user's call. `scenario-gate` runs the model on any PR that adds or changes a training scenario,
but with no `OPENROUTER_API_KEY` secret it skips and reports green having run nothing (it did so on
#355). Adding the secret makes the gate real, at a small OpenRouter cost per scenario PR.

**Done when:** the decision is made; if yes, the secret is set (Settings → Secrets and variables →
Actions) and the next scenario PR's `scenario-gate` log shows a run, not a skip. This file is deleted
either way.
