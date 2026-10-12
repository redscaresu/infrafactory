---
kind: code
repo: fakeaws
status: ready
touches: [handlers/iam.go, handlers/iam_test.go, handlers/regression_test.go, repository/iam.go, CHANGELOG.md]
---

# IAM answers the error codes and document encoding the provider expects

Found reviewing fakeaws#77 (fakeaws-loader conductor, 2026-10-12); outside that epic's stories.

- GetUserPolicy and GetPolicyVersion return policy documents unencoded; the provider URL-decodes both.
  Route them through the one RFC 3986 encoder #77 added for role inline policies.
- IAM handlers have no single error writer. GetRole on a missing role answers
  ResourceNotFoundException; IAM answers NoSuchEntity. DeletePolicy on an attached policy answers
  ResourceInUseException; IAM answers DeleteConflict.
- DeleteRole cascades managed-policy attachments; IAM refuses with DeleteConflict while any managed
  policy is attached.

**Done when**
- A test reads back a policy document containing `%`, `+`, spaces and non-ASCII through GetUserPolicy and
  GetPolicyVersion, decodes it as the provider does, and gets the original bytes.
- Missing role → NoSuchEntity (404); DeletePolicy while attached → DeleteConflict (409); DeleteRole with
  a managed policy attached → DeleteConflict and the role still exists. One IAM error writer is the only
  place IAM error codes are written, and each of these has a regression test.
- Each test fails with its fix reverted (cp backup), shown in the PR.
