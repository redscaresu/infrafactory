---
kind: code
repo: fakeaws
status: blocked
blocked_by: [fakeaws-instance-attributes-from-rows]
depends_on: [fakeaws-instance-attributes-from-rows]
touches: [handlers/kms.go, handlers/kms_test.go, handlers/handlers.go, handlers/admin.go, handlers/admin_test.go, repository/identity.go, repository/identity_test.go, AGENTS.md, CHANGELOG.md]
risk: high
---

# KMS keys follow the reset, snapshot and restore lifecycle, and count as rows for the identity guard

Found in fakeaws#61's review and reproduced. `kmsStore` (handlers/kms.go:56) is a package global:
`/mock/reset` does not clear it, snapshot and restore do not carry it, and the empty-database check
behind `identity-change-refused-on-non-empty-db` does not count keys. So CreateKey under the default
account, then `/mock/reset`, then `POST /mock/identity 123456789012` is accepted, and DescribeKey
and GetKeyPolicy still name `000000000000`. Keys also leak between Applications in one test process.
That breaks AGENTS.md anti-pattern 6 and the identity guard, and #61's MUST
`responses-carry-the-stored-account` carries it as a stated exception.

Make the store Application-owned (or SQLite-backed like every other service), cleared by reset,
carried by snapshot and restore, and counted by the identity guard's empty check. Remove the KMS
exception from the MUST and from the comment beside ScheduleKeyDeletion.

**Done when:**
- CreateKey, then `/mock/reset`: ListKeys is empty, and `POST /mock/identity` with a new account
  is accepted with no key left naming the old account.
- CreateKey, then `POST /mock/identity` with a new account: 409 naming both ids (keys count).
- Snapshot, CreateKey, restore: the key created after the snapshot is gone; one created before
  survives a snapshot and restore round trip.
- Two Applications in one test process see separate key sets.
- `MUST[responses-carry-the-stored-account]` has no KMS exception, and the account sweep covers KMS.
- Each new test is shown failing by a cp-backup break recorded in the PR.
