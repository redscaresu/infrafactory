---
kind: code
repo: fakeaws
status: blocked
blocked_by: [fakeaws-sg-rule-model]
epic: fakeaws-loader
depends_on: [fakeaws-sg-rule-model]
touches: [handlers/ec2.go, handlers/ec2_test.go, handlers/ec2_filters.go, repository/ec2.go, repository/ec2_test.go, examples/known_red_test.go, coverage_matrix.yaml, testdata/ec2/DescribeSecurityGroupRules.xml]
---

# DescribeSecurityGroupRules answers from the rule rows, rules take tags, and Revoke works by rule id

Part two of the rule model, on fakeaws-sg-rule-model's rows. DescribeSecurityGroupRules falls to
the default arm today, 404 (handlers/ec2.go:186-210). Carried from fakeaws-data-sources:
prod-read-seam-fakeaws-filtered-e2e leaves it out until this lands.

- DescribeSecurityGroupRules takes `SecurityGroupRuleId.N` and the filters `group-id`,
  `security-group-rule-id`, `tag:<key>`, `tag-key` and `tag-value` through the filter evaluator
  (handlers/ec2_filters.go). An unknown id is InvalidSecurityGroupRuleId.NotFound. The item shape
  (securityGroupRuleId, groupId, groupOwnerId, isEgress, ipProtocol, fromPort, toPort,
  cidrIpv4/cidrIpv6/prefixListId/referencedGroupInfo, description, tagSet) follows
  testdata/ec2/DescribeSecurityGroupRules.xml, from the EC2 API reference with a source comment on
  line 1.
- CreateTags and DeleteTags take `sgr-` ids: the prefix table (handlers/ec2.go:3612) gains them.
- Revoke* takes `SecurityGroupRuleId.N` as well as permissions.
- ModifySecurityGroupRules stays refused: 501 naming it.
- examples/updates/update_security_group_rules turns green, so its knownRed entry goes
  (examples/known_red_test.go:35), and its coverage_matrix row stops naming the gap.

**Done when:**
- DescribeSecurityGroupRules `group-id` answers both rules of a two-CIDR permission with isEgress
  false, and an egress rule with isEgress true; the answer has exactly the fixture's element paths
  (load_diff.go with leaf values blanked on both sides reports nothing).
- `security-group-rule-id` and `SecurityGroupRuleId.1` each select one rule. CreateTags Name=web on
  an `sgr-` id, then `tag:Name=web`, `tag-key=Name` and `tag-value=web` each select only that rule.
- Revoke by one rule id leaves the other rule of its permission, and DescribeSecurityGroups shows
  the permission with one source.
- An unknown SecurityGroupRuleId answers InvalidSecurityGroupRuleId.NotFound; ModifySecurityGroupRules
  answers 501 naming it.
- updates/update_security_group_rules passes provider-smoke with its knownRed entry gone;
  TestContract_ec2_sg_rule_source_descriptions_round_trip passes unchanged.
