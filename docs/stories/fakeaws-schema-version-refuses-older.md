---
kind: code
repo: fakeaws
status: ready
epic: fakeaws-loader
depends_on: []
touches: [repository/repository.go, repository/repository_test.go, repository/restore_test.go, repository/schema_golden_test.go, repository/testdata/schema.golden, handlers/admin_test.go]
risk: high
---

# A database or snapshot at another schema version is refused by name, so a changed table never runs on its old shape

migrate() runs only `CREATE TABLE IF NOT EXISTS` and always inserts version 1
(repository/repository.go:150-178). There is no ALTER, so a changed column never reaches an
existing file db or snapshot.

**The version.** A new const, `dbSchemaVersion = 2`, in repository/repository.go. It is separate
from `stateSchemaVersion` (handlers/admin.go:49), the /mock/state wire version that infrafactory
reads (internal/harness/topology_derive.go:64-92, internal/generator/orphan_classify.go:199). That
one stays 1 and is not renamed. A fresh db is written at dbSchemaVersion. An existing file whose
schema_version table holds anything but exactly one row equal to the const is refused, naming the
stored and wanted versions and `reset or delete the file`. Nothing is migrated, and the file is
left as it was. No data-preserving migration: a mock db is disposable, and refusing is stricter
than migrating silently.

**Where it is checked.** On open, in New, before migrate runs. On restore, inside stage()
(repository.go:437-452), on the staged copy, before anything is copied over the live file
(:399-402), and whether or not `prepare` is set: Repository.Restore(nil), which
repository_test.go:66, identity_test.go:35, ec2_test.go:338 and ssm_test.go:90 call, must refuse
with the live db untouched, not after copyFile has replaced it. The :419 migrate runs after the
copy and is not the check. Restore's refusal wraps models.ErrConflict, so /mock/restore answers
409 through the existing mapping, with live state unchanged.

**The guard.** repository/testdata/schema.golden holds dbSchemaVersion and the `sql` of every
table, index and trigger in sqlite_master of a fresh db. With the const unchanged, an existing
entry whose SQL differs fails the test, naming it and `bump dbSchemaVersion`. An entry the golden
lacks fails, naming it and `append it to the golden`. A bump regenerates the golden
(`go test ./repository -run TestSchemaGolden -update`). Every later story that changes an existing
CREATE TABLE, index or trigger bumps the const and regenerates the golden in its own PR, and a
story that adds a table appends it.

**Done when:**
- Opening a file built in the test at version 1 fails, naming 1 and 2, and the file is
  byte-identical afterwards (sha256 before and after). A file at version 3 is refused too.
- `repository.Restore(nil)` of a version-1 snapshot returns an error wrapping
  models.ErrConflict that names 1 and 2. The live db's rows and stored version are unchanged, and
  the repository still answers a query afterwards.
- /mock/restore of a version-1 snapshot answers 409 naming both versions, and /mock/state is
  byte-identical to before.
- A fresh file db opens at 2, and snapshot then restore succeeds.
- /mock/state still answers `schema_version: 1`, and handlers/admin_test.go:41-45 is unchanged.
- A cp-backup edit of one existing CREATE TABLE in repository/ec2.go, without a bump, fails
  TestSchemaGolden naming that table; an added table not in the golden fails naming it.
- `git grep -n 'VALUES (1)' repository/repository.go` prints nothing.
