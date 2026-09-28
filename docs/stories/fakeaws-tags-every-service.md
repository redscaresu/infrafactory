---
kind: code
status: ready
repo: fakeaws
---

# sqs, iam, rds, route53, dynamodb and eks round-trip tags, so infrafactory can carry a run-id tag through every service a full-stack scenario touches

infrafactory's `aws-layer-neutral-hcl` epic (ADR-0039 decision 9) deferred a `default_tags` block
carrying the run id because six services don't round-trip tags yet: a tag set at create would
silently vanish rather than come back on a describe/list, and nothing would fail to say so. Each
service needs create-time tag acceptance (`Tags` on Create*, or the service's tag-on-resource
call) plus a `ListTagsForResource` / `DescribeTags`-shaped read that returns exactly what was set,
matching how the real AWS API for that service does it:

- **sqs**: `CreateQueue` `tags`, `TagQueue`, `ListQueueTags`, `UntagQueue`
- **iam**: `CreateRole`/`CreateUser`/`CreatePolicy` `Tags`, `TagRole`/`TagUser`/`TagPolicy`,
  `ListRoleTags`/`ListUserTags`/`ListPolicyTags`, matching `Untag*`
- **rds**: `CreateDBInstance`/`CreateDBParameterGroup`/`CreateDBSubnetGroup` `Tags`,
  `AddTagsToResource`, `ListTagsForResource`, `RemoveTagsFromResource`
- **route53**: `ChangeTagsForResource`, `ListTagsForResource` (hosted zones)
- **dynamodb**: `CreateTable` `Tags`, `TagResource`, `ListTagsOfResource`, `UntagResource`
- **eks**: `CreateCluster` `tags`, `TagResource`, `DescribeCluster` (tags field),
  `UntagResource`

**Done when:**
- Each of the six services accepts tags at create and a subsequent describe/list-tags call
  returns exactly the tags set, including a tag with no value and a tag key containing `:` or `/`
- Each service's tag/untag mutation calls (`TagQueue`, `TagRole`, `AddTagsToResource`,
  `ChangeTagsForResource`, `TagResource` ×2) add and remove tags on an existing resource without
  disturbing the others already present
- A round-trip through the real `hashicorp/aws` v5 provider (`default_tags` at the provider level
  plus one resource per service) applies, and a subsequent `plan -detailed-exitcode` is 0 — proving
  the provider's own tag-diffing sees nothing to change
- An unknown resource id/ARN to any tag call returns the service's real not-found shape, not a 200
- Existing examples for these six services are unaffected; no `known_red_test.go` entry regresses
