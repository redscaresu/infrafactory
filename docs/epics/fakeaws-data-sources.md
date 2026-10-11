---
status: active
hld: 2026-09-30-import-production-state-into-the-mocks
depends_on: []
---

# fakeaws answers data sources from rows and refuses what AWS refuses on create: generic Filter.N, GroupName.N, image owner/name filters, DescribeAvailabilityZones, subnet overlap (Rollout 3).

**Done when:**
- Every Describe a web-stack data source uses (Vpcs, Subnets, SecurityGroups, Instances, Images, RouteTables, InternetGateways, Addresses, KeyPairs, NetworkInterfaces) evaluates `Filter.N` over tags and the documented filter names through one evaluator; an unsupported filter name is refused naming it, never ignored. One status for that refusal, chosen here and shared with fakeaws-rehearsal-instance's `filter-refused` classification, tested end to end.
- `DescribeSecurityGroups` honours `GroupName.N` and `Filter.N`; `DescribeImages` honours `Owner.N` (incl. `amazon`, `self`) and `name`.
- `DescribeAvailabilityZones` exists, answered from an AZ table; an unloaded database answers a fixed set for its region. (Filling it from a recorded read is fakeaws-loader's.)
- `CreateSubnet` refuses a CIDR overlapping a sibling subnet with `InvalidSubnet.Conflict` (D8 second half).
- A data-source example under examples/working/ (VPC by tag, SG by name, AMI by owner+name, an AZ) applies under 5.100.0; each surface has a coverage_matrix row, a regression test and a contract docstring where it is a wire invariant; FAKEAWS_SHA bumped in a final story; deltas.md records D8 closed.

**Out of scope:** loading rows; new resource types; filters for non-EC2 services unless a web-stack data source needs them.

**Constraints:** filter work in the Describe handlers and a new `ec2_filters.go`; its ec2.go stories run one after another with fakeaws-loaded-identity's; whichever of this epic's filter story and fakeaws-request-log-gap-markers lands second updates TestContract_request_log_filter_refused; an AZ table answering a fixed set when unloaded reuses fakeaws-loaded-identity's loaded reader, never a second flag; fix forward at the source, never pitfalls/*.yaml.

**Your part:** none.

**Areas:** `fakeaws:handlers/ec2.go, ec2_filters.go (new)`; `fakeaws:repository/ec2.go (AZ table)`; `fakeaws:examples/working/`; `fakeaws:coverage_matrix.yaml, handlers/regression_test.go`; `.github/workflows/ci.yml (FAKEAWS_SHA)`; `docs/layer3/real-vs-mock-deltas.md (D8)`
