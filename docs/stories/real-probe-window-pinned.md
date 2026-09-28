---
kind: chore
status: ready
epic: aws-layer-neutral-hcl
depends_on: []
touches: ["infrafactory.yaml", "internal/config/config.go", "internal/config/real_probe_window_test.go (new)", "docs/stories/real-probe-window-pinned.md (delete)"]
---

# One shared real-probe window, justified, with the repo config pinned to the code default

The operative values already agree: config.go:352 and infrafactory.yaml:233 both say retries 60. Only comments are stale: infrafactory.yaml:190-192 and config.go:335 say "24 attempts ~120s" and "24 x (5s + 5s)". They become 60 x (5s + 5s) ~= 600s for a black-holed endpoint, with the reason the value is shared: the probe returns on its first success (infrafactory.yaml:223-228). RealProbeConfig stays one struct. ADR: none — comment and test only.

**Done when:**
- A test loads the repo's infrafactory.yaml through config.Load and asserts validation.real_probes equals the config default's. Changing either retries value alone fails it
- The same test reads both files and fails if "24 attempts" or "24 x (5s" remains
