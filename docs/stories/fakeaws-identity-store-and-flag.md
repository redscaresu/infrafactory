---
kind: code
repo: fakeaws
status: blocked
blocked_by: [fakeaws-admin-token-and-bind]
epic: fakeaws-loaded-identity
depends_on: [fakeaws-admin-token-and-bind]
touches: [repository/identity.go, repository/identity_test.go, handlers/handlers.go, handlers/admin.go, handlers/admin_test.go, cmd/fakeaws/main.go, cmd/fakeaws/main_test.go, testutil/testutil.go, AGENTS.md, README.md, CHANGELOG.md]
risk: high
---

# Stored identity: a one-row table, -account/-region, POST /mock/identity, and a refusal on a non-empty database

**Table.** New `repository/identity.go` holds the table:
`identity(id INTEGER PRIMARY KEY CHECK(id=1), account_id TEXT NOT NULL, region TEXT NOT NULL DEFAULT '', loaded INTEGER NOT NULL DEFAULT 0)`.
It registers from init() through registeredMigrations and prependResetTables (repository.go:153,233), so Reset deletes it, snapshots carry it, and repository.go is not edited. The region is accepted and stored, and nothing reads it yet. Every regional route still takes the region from its URL.

**Accessor.** `Application.AccountID() (string, error)` reads the database on every call; it is never cached. `sql.ErrNoRows` returns `awsproto.FakeAccountID`; any other error is returned. Callers read it at their entry, before any transaction (one connection, repository.go:99). `collectState` reads it once before any gather, and on an error GET /mock/state answers 500.

**Setting it.** This story adds `AccountID` and `Region` fields to the `handlers.Options` that fakeaws-admin-token-and-bind creates. main.go gains `-account` (12 digits) and `-region`, validated by a parse function that main_test.go calls.
- Boot without `-account` writes nothing.
- Boot with it stores the identity when the database is empty, does nothing when the stored account is equal, and fails NewApplication naming both ids when the database is not empty and the account differs.
- POST /mock/identity `{"account_id","region"}` sits inside the guarded `/mock` group. DisallowUnknownFields, one object only (as admin.go:87-96). 400 names the bad field, 409 names both ids, 200 otherwise.
- The empty check and the upsert run in one transaction.
- The boot fixture seed (handlers.go:55) and validateSeedImage (admin.go:171-181) use the stored account; at the default, the message is unchanged.

**Empty.** Empty means no row in any reset table outside one named exclusion list in identity.go: operations, audit, identity, and the fixture AMI ids. An AMI seeded through /mock/images counts. fakeaws-data-sources must register any seed rows it adds in this list.

**Known gap.** KMS keys live in the process-global kmsStore (kms.go:46-53), not in rows. Empty does not count them, and a key keeps the account it was created under. That is left to a future story.

**Interim, until fakeaws-identity-iam-sts and fakeaws-identity-ec2 land.** `-account` stores the id, and the boot fixtures and /mock/images rows go under it. IAM, STS and EC2 still read the constant, so RunInstances cannot see those AMIs. The default is unaffected, and nothing consumes it before the pin.

**Order:** never open at the same time as fakeaws-owner-only-files or fakeaws-request-log (same admin.go `/mock` block and AGENTS.md:151). Rebase onto main.

**Done when:**
- TestContract_identity_change_refused_on_non_empty_db: a role or VPC row exists under the default. POST /mock/identity 123456789012 answers 409 naming both ids, and AccountID still answers 000000000000. On an empty database the same POST answers 200 and AccountID answers 123456789012. NewApplication with a different Options.AccountID on a non-empty file database errors, naming both ids.
- TestContract_identity_stored_in_database: on a file database, set 123456789012/eu-west-1, snapshot, set 210987654321, restore: both account and region come back. Reopening with no AccountID keeps them. POST /mock/reset returns 000000000000.
- Identity table dropped through Repository().DB(): AccountID returns an error, POST /mock/identity answers 500, GET /mock/state answers 500.
- A fresh boot followed by DescribeImages in an unseeded region still counts as Empty (POST /mock/identity answers 200). This test fails when a new seeded table is added without joining the list.
- POST /mock/identity answers 400 on a non-12-digit id, an unknown field or trailing data. With an admin token set and no bearer, it answers 401.
- `-account 12` is rejected by the parse function, naming the flag. Boot without it writes no row.
- Both contract docstrings sit in admin.go. The AGENTS.md contracts line lists both. README has the `-account` and `-region` flag rows and the POST /mock/identity admin row, and says reset returns to the default. CHANGELOG [Unreleased] has one entry. The contract audits pass.
- Existing tests pass with no assertion line changed. Each new test is shown failing by a cp-backup break of the code it guards, recorded in the PR.
