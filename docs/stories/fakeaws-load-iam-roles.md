---
kind: code
repo: fakeaws
status: ready
epic: fakeaws-loader
depends_on: [fakeaws-load-exchange-rules, fakeaws-role-inline-policies]
touches: [handlers/load_iam.go, handlers/load_iam_test.go, handlers/iam.go, handlers/iam_test.go, repository/load_iam.go, repository/iam.go, testdata/iam/]
---

# IAM roles load with their inline and attached policies and instance profiles, ids as recorded

Loaders for the IAM reader's calls, GetRole, ListRolePolicies, GetRolePolicy,
ListAttachedRolePolicies and ListInstanceProfilesForRole (infrafactory
internal/harness/aws_reader_table.go:60-69), in handlers/load_iam.go.

Widen roleToXML and instanceProfileToXML (handlers/iam.go:335-362, :639-664): RoleId, CreateDate
as recorded, MaxSessionDuration, Description, PermissionsBoundary, RoleLastUsed or its ignore
entry with a cite, InstanceProfileId. Widened fields are struct fields in the row's data JSON
(iam_roles and iam_instance_profiles have one), so no column changes.

- Inline documents load into role_inline_policies, byte-exact once URL-decoded.
- Attached ARNs load into role_policy_attachments without seeding a policy row. An AWS-managed ARN
  reads as today; a customer-managed one is unresolved, since GetPolicy is never read.
- A profile from ListInstanceProfilesForRole loads with its role binding (instance_profile_roles).
- Tags come from GetRole.

Fixtures: testdata/iam/ from the IAM API reference. IAM is proved on these only: the step-0 read
holds no IAM (SCP).

**Done when:**
- The five actions, re-sent with the recorded params, answer bodies load_diff.go finds equal to
  the fixtures in both directions.
- RoleId `AROA…` and InstanceProfileId `AIPA…` answer as recorded; GetRole's CreateDate equals the
  fixture's.
- Mutate once: after UpdateRole MaxSessionDuration=7200 on the loaded role, GetRole equals the
  recording except at Role/MaxSessionDuration, RoleId and CreateDate included.
- A customer-managed attached ARN loads and is answered under unresolved; GetPolicy on it is
  NoSuchEntity, as before.
- A ListRolePolicies name with no GetRolePolicy exchange refuses, naming role and policy.
- An unknown element under GetRoleResult/Role refuses, naming GetRole and its path.
