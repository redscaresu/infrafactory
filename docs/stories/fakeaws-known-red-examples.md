---
kind: code
status: later
repo: fakeaws
---

# The fakeaws smoke examples no story owns go green

fakeaws #24 put the provider smoke harness in CI and listed 11 red examples in
`examples/known_red_test.go`. Eight have no owning story:

- `working/eks_cluster` and `misconfigured/eks_node_group_subnet_outside_cluster`: main.tf is
  invalid HCL (`;`), so they fail at `init` and test no EKS behaviour.
- `working/s3_bucket`, `updates/update_s3_bucket_versioning`: `GetBucketPolicy` missing.
- `misconfigured/route53_apex_cname`: answers 409 `UnknownError`, not `InvalidChangeBatch`.
- `updates/update_iam_role_description`: `UpdateRoleDescription` 404.
- `updates/update_rds_parameter_group`: `DeleteDBParameterGroup` 409 while in use.
- `updates/update_security_group_rules`: `DescribeSecurityGroupRules` 404.
- `updates/update_sqs_queue_visibility`: `SetQueueAttributes` is a no-op, the provider waits 3m.

Split into one story per service when picked up.

**Done when:** each example above passes in the fakeaws `provider-smoke` job and its `knownRed`
entry is removed.
