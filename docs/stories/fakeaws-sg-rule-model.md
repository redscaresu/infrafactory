---
kind: code
repo: fakeaws
status: ready
epic: fakeaws-loader
depends_on: [fakeaws-load-route-tables-main]
touches: [handlers/ec2.go, handlers/ec2_test.go, handlers/admin_test.go, repository/ec2.go, repository/ec2_test.go, repository/ec2_tags.go, repository/repository.go, repository/testdata/schema.golden, testdata/ec2/AuthorizeSecurityGroupIngress.xml]
risk: high
---

# Security group rules are rows with stable sgr- ids, and Authorize* answers securityGroupRuleSet, while every existing answer stays byte-identical

Part one of the rule model; fakeaws-describe-security-group-rules adds the new action on these
rows. Rules are one JSON array per direction today, in ec2_security_groups.ingress/egress
(repository/ec2.go:109-123), with no rule id. Risk is high because /mock/state's security_groups
feed infrafactory's default_deny_ingress deny_state check
(infrafactory scenarios/training/aws-web-live.yaml:40-44). Built for reuse by
aws-web-stack-load-balancer (infrafactory docs/epics/aws-web-stack-load-balancer.md:10).

**The row**, fixed here so fakeaws-load-security-groups needs no schema change: table
ec2_security_group_rules with `account_id, region, id` (`sgr-`), `group_id`, `group_owner_id`,
`is_egress`, `perm_ord` (the permission's position in its direction), `src_ord` (the source's
position in its permission), `ip_protocol`, `from_port`, `to_port`, exactly one of `cidr_ipv4`,
`cidr_ipv6`, `prefix_list_id` or `referenced_group_id` with `referenced_user_id`, and
`description`. FK `group_id` to the group ON DELETE CASCADE. `referenced_group_id` has no FK: a
pair naming another account's UserId is admitted today (handlers/ec2.go:1611-1614), and deleting a
group another rule references stays allowed, as today, leaving that rule as is. Tags: the table
joins ec2TaggedTables, with an AFTER DELETE trigger. The ingress and egress columns go, so
dbSchemaVersion is bumped and the golden regenerated.

**Behaviour.**
- DescribeSecurityGroups and /mock/state aggregate ipPermissions from the rows by `(is_egress,
  perm_ord, src_ord)`, reproducing today's order exactly, including a permission that keeps its
  slot when a revoke removes its first source (mergeIpPermissions/subtractIpPermissions,
  handlers/ec2.go:2108-2143).
- Authorize* answers `return` and securityGroupRuleSet with the new rows' ids, in the shape of
  testdata/ec2/AuthorizeSecurityGroupIngress.xml (from the EC2 API reference, source comment on
  line 1). Re-authorizing a source already present answers as today (idempotent, ec2.go:2105-2107)
  and creates no row.
- Revoke* by permission works on the rows.

**Admin tests.** The two GetSecurityGroupRules break cases in handlers/admin_test.go
(`ALTER TABLE ec2_security_groups DROP COLUMN ingress`, :142, and `UPDATE ... SET ingress = 'not
json'`, :146) cannot run once the columns go. They are replaced by `DROP TABLE
ec2_security_group_rules` and a corrupt rule row, and TestAdminMockState_UnreadableTableIs500
gains the rule-table read (admin_test.go:122-123). The epic's checks name this exception.

**Done when:**
- /mock/state's security_groups block and DescribeSecurityGroups' ipPermissions are byte-identical
  to a golden captured from main before the change, for every SG setup in ec2_test.go and
  admin_test.go, plus this one: P1[A], P2[B], authorize C on P1, revoke A answers P1 before P2.
- Authorize of two CIDRs in one permission answers two securityGroupRuleSet items with distinct
  `sgr-` ids; an egress authorize answers egress ids; the answer has exactly the fixture's element
  paths (load_diff.go with leaf values blanked on both sides reports nothing).
- Re-authorizing a present CIDR answers as before and leaves the row count unchanged.
- DeleteSecurityGroup and DeleteVpc leave no rule row and no rule tag. Deleting a group that
  another group's rule references succeeds, and the referencing rule stays.
- The replaced admin break cases answer 500 as before; a file at the previous schema version is
  refused.
