---
kind: lead
status: blocked
blocked_by: [fakeaws-provider-exact-pin]
epic: fakeaws-step-one-surfaces
depends_on: [fakeaws-provider-exact-pin]
touches: ["docs/hld/2026-09-27-aws-web-stack.md", "docs/epics/aws-layer-neutral-hcl.md"]
---

# Item 1 (record): the HLD and aws-layer-neutral-hcl name the pin and the env-form proof

Write hashicorp/aws 5.100.0 and the env_endpoints proof into docs/hld/2026-09-27-aws-web-stack.md:111-113, and close its Open question (:461-464). Name the pin at docs/epics/aws-layer-neutral-hcl.md:10.

**Done when:**
- grep finds 5.100.0 in both files; the HLD's Open questions no longer lists the AWS_ENDPOINT_URL_<SERVICE> question
- make doc-hygiene passes; pitfalls/aws.yaml is unchanged
