---
kind: code
repo: fakeaws
status: ready
epic: fakeaws-loader
depends_on: [fakeaws-describe-security-group-rules]
touches: [handlers/load_ec2_groups.go, handlers/load_ec2_groups_test.go, repository/load_ec2.go, repository/ec2.go, handlers/ec2.go, testdata/ec2/DescribeSecurityGroups.xml]
---

# Security groups and their rules load with recorded group and rule ids; the default group is the recorded one, never minted

Loaders for DescribeSecurityGroups and DescribeSecurityGroupRules in handlers/load_ec2_groups.go.
The rule row's columns are fakeaws-sg-rule-model's, so there is no schema change here. Fixtures:
testdata/ec2/DescribeSecurityGroups.xml (new, EC2 API reference) and the
DescribeSecurityGroupRules.xml fakeaws-describe-security-group-rules added.

- Groups load with recorded ids, ownerId and tags.
- The default group loads as recorded and is never minted: BackfillDefaultGroups returns early on
  a loaded db (repository/ec2.go:289-300).
- Rules load with their recorded `sgr-` ids, groupOwnerId, description and tags, in the recorded
  order within each group (perm_ord, src_ord), so DescribeSecurityGroups aggregates them back.
- A group's ipPermissions must equal the aggregate of its recorded rules, or the load is refused
  naming group and path.
- A rule referencing a group of the loaded account that the recording lacks is refused. One
  referencing another account's group, or a prefix-list id, is unresolved.

**Done when:**
- Both Describes, re-sent with the recorded params, answer bodies load_diff.go finds equal to the
  fixtures in both directions, `sgr-` ids included.
- The same group in a DescribeSecurityGroups `tag-key` exchange and a `vpc-id` exchange (the
  step-0 shape) loads once, and is counted once under loaded.
- A VPC whose recording holds no default group has none after load, nor after snapshot and restore.
- Mutate once: after AuthorizeSecurityGroupIngress of a new CIDR on a loaded group, both Describes
  equal the recording except for the new rule, and every recorded `sgr-` id is unchanged.
- A group whose ipPermissions holds a CIDR no recorded rule holds refuses, naming the group and the
  path.
- A rule referencing an absent group of the loaded account refuses, naming rule and group.
