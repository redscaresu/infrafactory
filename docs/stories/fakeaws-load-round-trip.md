---
kind: code
repo: fakeaws
status: blocked
blocked_by: [fakeaws-load-volumes-d7, fakeaws-load-iam-roles]
epic: fakeaws-loader
depends_on: [fakeaws-load-volumes-d7, fakeaws-load-iam-roles]
touches: [examples/load_round_trip_test.go, coverage_matrix.yaml, internal/audit/audit_test.go, .github/workflows/ci.yml, examples/README.md]
---

# Every working example whose types are all loadable applies, is recorded, loads into a fresh fakeaws and plans clean; coverage_matrix gains loadable

examples/load_round_trip_test.go, beside the smoke harness (examples/provider_smoke_test.go:58-90,
:284-330). It proves load, not real shape.

- It takes each working example whose every resource type's matrix row is `loadable: true`, and
  applies it against fakeaws A.
- A body-capturing recorder written here (fakeaws cannot import infrafactory's reader) sends the
  reader-table actions for each loadable type and writes an ADR-0041 envelope. It does not record
  the Describe and List calls it uses to find ids.
- The test POSTs that envelope to a fresh fakeaws B, runs `tofu plan -detailed-exitcode` with A's
  state against B, which must exit 0, then destroys on A.

coverage_matrix.yaml: every row gets `loadable: true` or a `loadable_exempt_reason`.
CoverageMatrixEntry gains both fields, and the matrix is decoded with KnownFields(true), so an
unknown column fails. TestFullCoverageAudit requires exactly one of the two per row, and that each
loadable row's type is in at least one round-tripped example. Rows outside the reader table (S3,
RDS and the rest), and aws_ssm_parameter, whose non-public parameters are refused, are exempt with
a reason. CI runs the test in the smoke shards (TestShardsPartitionTheSmokeRun).

Drift between the recorder's action list and infrafactory's AWSReaderTable is caught in
infrafactory CI by fakeaws-loader-e2e's TestAWSReaderTableLoadableByFakeaws.

**Done when:**
- In CI, every working example whose types are all loadable round-trips with plan exit 0. The test
  logs the list, and the list includes vpc_network and web_step_one.
- A cp-backup break of one loader's rendering fails vpc_network's round trip at load, naming the
  type and the path.
- A misspelled column (`loadabel: true`) fails TestFullCoverageAudit, and so does a row with
  neither field.
- The recorder's envelope keys are exactly account, region, timestamp, duration_ms, calls and
  exchanges, and each exchange's keys match ADR-0041's (a golden from the ADR's example).
