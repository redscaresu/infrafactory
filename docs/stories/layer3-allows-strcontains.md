---
kind: code
risk: high
epic: firewall-end-to-end
status: ready
touches: [internal/cli/layer3_hcl_shape.go, internal/cli/layer3_hcl_shape_test.go]
---

# Layer 3's pure-function allowlist admits `strcontains()`

The Layer 3 HCL gate refuses any function not on its pure-function allowlist, because it evaluates
HCL with real credentials in the environment and a function that reads the filesystem could put
them into a resource attribute. `strcontains()` reads nothing; refusing it cost run
`20260927T163652Z` a whole iteration (`loadbalancer.tf: server_ips calls strcontains()`).

**Done when:** `strcontains` is on the allowlist; a test shows HCL using it passes the gate; and a
test shows a filesystem-reading function (`file`, `templatefile`) is still refused. Add only
functions that are provably pure, and say so in the PR.
