---
kind: code
status: ready
repo: mockway
touches: ["repository/repository.go", "repository/repository_test.go"]
---

# mockway keeps the volume type and size the provider sent (RDB instance, block volume)

Found when the opt-in e2e suites got past the M81 guard (2026-09-28). Each of these applies
cleanly, then its converge plan is not empty and the run ends `drift`:

- `scaleway_rdb_instance` with `volume_type = "sbs_5k"` plans `volume_type = "lssd" -> "sbs_5k"`:
  `TestE2E_FullStackParis`, `TestE2E_WebAppParis`, `TestE2E_ScalewayRDB`
  (`INFRAFACTORY_ENABLE_E2E=1`) and `TestRunCommandRealToolIncrementalMockwayE2E`
  (`INFRAFACTORY_ENABLE_REALTOOL_INCREMENTAL=1`). mockway's RDB create ignores the request's
  `volume_type` / `volume_size` and, with no `volume` object, stores `volume{type: "lssd", size:
  10000000000}` (`repository/repository.go`, the "Fields required by the TF provider's
  ResourceRdbInstanceRead" block).
- `scaleway_block_volume` with `size_in_gb = 10` plans a replacement, `size_in_gb = 20 -> 10`:
  `TestE2E_ScalewayBlock`. `CreateBlockVolume` reads only a top-level `size` and defaults it to
  20 GB; the block API's create sends the size inside `from_empty`.

Fix both in mockway: store what was sent, keeping the current defaults only when nothing was.
Never seed a pitfall for either.

**Done when:**
- mockway repository tests: an RDB create with `volume_type: "sbs_5k"`, `volume_size:
  20000000000` reads back `volume{type: "sbs_5k", size: 20000000000}`; a block create with
  `from_empty: {size: 10000000000}` reads back `size: 10000000000`; creates that send neither
  keep today's defaults. The first two fail today
- With the fix built, those five infrafactory tests no longer drift on `volume_type` or
  `size_in_gb`
