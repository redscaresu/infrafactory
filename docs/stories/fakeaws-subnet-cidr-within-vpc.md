---
kind: code
status: ready
repo: fakeaws
depends_on: []
touches: [handlers/ec2.go, handlers/regression_test.go]
---

# fakeaws refuses a subnet whose CIDR is outside its VPC, as EC2 does

Seen on real AWS (infrafactory docs/layer3/real-vs-mock-deltas.md D8, run 20261010T114213Z):
`ec2CreateSubnet` stores any `CidrBlock`, so `aws_subnet { cidr_block = "10.81.1.0/24" }` in a
`10.80.0.0/16` VPC applies at Layer 2 and fails only on real AWS with
`InvalidSubnet.Range: The CIDR '10.81.1.0/24' is invalid.` Answer it as EC2 does: 400
`InvalidSubnet.Range` when the subnet CIDR is not within the VPC's CIDR block (net/netip prefix
containment), and when the prefix is not between /16 and /28. Keep overlapping-subnet checks out
(YAGNI) unless a run shows them.

**Done when:**
- A regression test (AWS SDK v2) creates a VPC 10.80.0.0/16, a subnet 10.81.1.0/24 fails 400
  `InvalidSubnet.Range`, a subnet 10.80.1.0/24 succeeds, and a /29 inside the VPC fails. Shown to
  fail with the check removed (mutation).
- Provider smoke examples still apply.
