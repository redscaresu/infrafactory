---
kind: code
status: later
repo: fakegcp
touches: ["handlers/dns.go", "handlers/sql.go", "handlers/regression_test.go", "handlers/sql_compute_defaults_test.go"]
---

# fakegcp stops returning blocks the config never set (DNS zone update, Cloud SQL maintenance window)

Found when the opt-in e2e suites got past the M81 guard (2026-09-28, against fakegcp
origin/main). Each applies cleanly, then its converge plan is not empty and the run ends `drift`:

- `TestE2E_GCPDNS`, update stage: `google_dns_managed_zone` plans removing an empty
  `private_visibility_config {}`. `CreateDNSZone` runs `stripEmptyDNSZoneSubObjects` on the
  provider's placeholder sub-objects; `UpdateDNSZone` (`handlers/dns.go`) does not, so a PATCH
  stores `privateVisibilityConfig: {}` and the next read reports it as configured.
- `TestE2E_GCPCloudSQL` and `TestE2E_GCPFullStack`: `google_sql_database_instance` with no
  `maintenance_window` plans removing `maintenance_window { day = 0, hour = 0, update_track =
  "stable" }`. Instance create (`handlers/sql.go`) defaults `settings.maintenanceWindow` to that
  value when the request has none.

Fix both in fakegcp. For the maintenance window, return the shape the provider's flatten reads
as unset, without bringing back the nil dereference the server-side defaults were added for
(98476cd). Never seed a pitfall for either.

**Done when:**
- fakegcp handler tests: a zone PATCH carrying `privateVisibilityConfig: {"networks": []}` reads
  back without it; an instance created without `maintenanceWindow` reads back in the unset
  shape. Both fail today
- With the fix built, `TestE2E_GCPDNS`, `TestE2E_GCPCloudSQL` and `TestE2E_GCPFullStack`
  (`INFRAFACTORY_ENABLE_E2E=1`) no longer drift
