---
kind: code
status: ready
touches: [internal/harness/topology_derive_aws.go, internal/harness/topology_derive_aws_test.go]
---

# The AWS topology reader honours a VPC's main route table

From the fakeaws-loader conductor reviewing fakeaws#84 (2026-10-13). `internal/harness/topology_derive_aws.go`
reads `/mock/state` and ignores route-table associations' `main` flag; its comment says fakeaws exports
none. Since fakeaws#84, a loaded estate's main route tables carry `main: true`, and a subnet routed only
through its VPC's main table would read as unrouted.

Set this `ready` when loaded state reaches the harness (after fakeaws-loader-pin).

**Done when**
- A subnet with no explicit association, in a VPC whose main table routes 0.0.0.0/0 to an IGW, derives as
  public; with the main table removed from the fixture it derives as unrouted. The test fails if the
  main-flag branch is deleted.
- The stale "fakeaws exports none" comment is gone.
