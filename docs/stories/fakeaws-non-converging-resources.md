---
kind: code
status: ready
repo: fakeaws
---

# Four resource shapes fakeaws serves never converge under the real provider, tags aside

Found while proving `default_tags` round-trips (a throwaway fixture applied twice through
`infrafactory run` at fakeaws 1e5e37a plus the tag fixes): each of these fails apply or plans a
diff on its second plan whatever its tags, so an AWS scenario that uses one stops on drift.

- `aws_iam_instance_profile` with `role`: the next plan shows `+ role`, and a second apply fails
  `AddRoleToInstanceProfile` 409. `iamInstanceProfileXML.Roles` nests `iamRoleXML`, whose
  `XMLName` is `Role`, under `Roles>member`. The phase-1 prompt tells the model to use one
  before any `aws_instance` that needs a role.
- `aws_rds_cluster`: the next plan replaces it (`engine_mode` "provisioned" forces replacement;
  `port`, `backup_retention_period` and others read back 0).
- `aws_rds_cluster_parameter_group`: apply fails, `DescribeDBClusterParameters` 404.
- `aws_eip`: apply fails, `DescribeAddressesAttribute` 404.

**Done when:** each has a smoke example that passes apply, `plan -detailed-exitcode` 0 and
destroy; the three that take tags (`aws_rds_cluster`, `aws_rds_cluster_parameter_group`,
`aws_eip`) round-trip them, including a changed provider `default_tags` value.
