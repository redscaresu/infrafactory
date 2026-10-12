---
kind: code
repo: fakeaws
status: blocked
blocked_by: [fakeaws-schema-version-refuses-older]
epic: fakeaws-loader
depends_on: [fakeaws-schema-version-refuses-older]
touches: [handlers/iam.go, handlers/iam_test.go, repository/iam.go, repository/iam_test.go, repository/testdata/schema.golden, coverage_matrix.yaml, examples/working/iam_role_inline_policy/main.tf, examples/README.md]
---

# PutRolePolicy, GetRolePolicy, ListRolePolicies and DeleteRolePolicy answer from a role_inline_policies table

Today Put and DeleteRolePolicy are iamNoOpSuccess, GetRolePolicy is a constant empty document and
ListRolePolicies is empty (handlers/iam.go:190-205, :1166-1182, :1249-1263).

Copy user_inline_policies (repository/iam.go:114-122, :784-840): role_inline_policies
`(account_id, role_name, name, document)`, FK to iam_roles ON DELETE CASCADE, before iam_roles in
resetTables. GetRolePolicy URL-encodes the document as GetUserPolicy does; an absent name is
NoSuchEntity. DeleteRole of a role holding an inline policy is 409 DeleteConflict, as the IAM API
reference documents (an `UNVERIFIED(aws-code)` comment: no recording). It is a new table, so there
is no schema bump; it is appended to the schema golden.

Audits. coverage_matrix.yaml gains an aws_iam_role_policy row: working
`iam_role_inline_policy` (new), misconfigured and updates rows exempt with reasons or pointing at
examples, and integration_test_func_name naming the new handlers test. LandedServices
(handlers/regression_manifest.go) already lists `iam`, and no new handlers file prefix is added,
so the manifest audit needs no edit; the done_when checks it still passes.

**Done when:**
- Put then Get answers the document byte-exact once URL-decoded; List names it; a second Put
  replaces it; Delete removes it; Get of an absent name is NoSuchEntity.
- DeleteRole of a role holding an inline policy answers DeleteConflict and keeps both; after
  DeleteRolePolicy it succeeds.
- Reset empties the table; snapshot, Put another, restore holds exactly the first.
- examples/working/iam_role_inline_policy passes provider-smoke (apply, idempotent plan, destroy),
  and TestFullCoverageAudit passes with the aws_iam_role_policy row.
- TestRegressionSeedAuditManifestMatchesHandlers passes, and `git diff main --
  handlers/regression_manifest.go` is empty.
- TestSchemaGolden passes with role_inline_policies appended and dbSchemaVersion unchanged.
