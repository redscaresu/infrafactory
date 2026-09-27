---
kind: code
risk: high
status: ready
touches: [pitfalls/scaleway.yaml, internal/cli/layer3_hcl_shape.go]
---

# The private-IP guardrail prescribes an address the provider rejects

The curated `scaleway_lb_backend` pitfall says never to index `private_ips[0]` directly, because an
expression that cannot be evaluated breaks `tofu destroy` as well as apply. That is right. But it
prescribes `try(scaleway_instance_private_nic.web.private_ips[0].address, "")`, and the fallback
`""` is not an IP: in run `20260927T171147Z` iteration 1 the generator wrote
`server_ips = [try(scaleway_instance_server.web.private_ips[0].address, "")]`, the provider
rejected it (`expected server_ips.0 to contain a valid IP, got:`) at apply **and** at destroy, and a
full stack leaked (see `teardown-survives-config-error`). Iteration 2 passed with
`server_ips = [scaleway_instance_ip.web.address]`.

**Done when:** the pitfall prescribes an address that is valid whenever it is evaluated, keeps the
reason indexing is dangerous, and stays under the 1000-byte cap; and the Layer 3 HCL gate refuses a
`try(..., "")` whose fallback is an empty string in an IP field such as `server_ips`, with a test.
Edit the pitfall by hand here deliberately: it is curated guidance, not a sweep's output.
