---
kind: code
repo: fakeaws
status: blocked
blocked_by: [fakeaws-request-log, fakeaws-arn-builders-take-account]
epic: fakeaws-rehearsal-instance
depends_on: [fakeaws-request-log, fakeaws-arn-builders-take-account]
touches: [handlers/ec2.go, handlers/iam.go, handlers/rds.go, handlers/dynamodb.go, handlers/sqs.go, handlers/secretsmanager.go, handlers/kms.go, handlers/ssm.go, handlers/recorder.go, handlers/recorder_test.go, AGENTS.md, concepts.md, CHANGELOG.md]
risk: high
---

# Mark the gap sites: not-modelled-404, filter-refused, empty-collection

Through fakeaws-request-log's marker helper, with every wire byte unchanged:
- **not-modelled-404** at the 7 default arms: ec2.go:190-195, iam.go:210-212, rds.go:124-126, dynamodb.go:69-71, sqs.go:71-73, secretsmanager.go:74-76, kms.go:101-103. Their 404 is wire-identical to a real ErrNotFound (awsproto/error.go:27-39), so only the marker tells them apart.
- **filter-refused** at refuseUnmodelled (ec2.go:2777-2786; callers :1170, :1627, :2767), the inline filter checks (ec2.go:896, :1177, :2394, :3168), AssociateAddress AllowReassociation (:1264-1266) and the SSM filter 501s (ssm.go:192, :197, :260). The SSM sites call writeUnimplemented first and then mark filter-refused; the last marker wins. ssm.go:260 is in ssmDecodeTagRequest, shared by ListTagsForResource and the tag writes (ssmChangeTags, ssm.go:293): decided, AddTagsToResource and RemoveTagsFromResource on an unmodelled ResourceType also record filter-refused.
- **empty-collection** only in ec2DescribeNone (ec2.go:2766-2772), after its Filter check.

**Non-goals (stay answered-from-rows, named in concepts.md and AGENTS.md as known ceilings):** GET /s3/{bucket} with an unknown subresource (s3.go:167, a 200 ListBucketResult), the always-empty ListObjects (s3.go:388-399), and the always-404 HeadObject/GetObject (s3.go:416-419).

**Cross-epic.** fakeaws-data-sources rewrites the EC2 filter refusals: whichever of this story and its evaluator story lands second updates TestContract_request_log_filter_refused and keeps the marker. fakeaws-loaded-identity edits iam.go and ssm.go near these sites (ssm.go:199): rebase, different hunks.

**Done when:**
- `TestContract_request_log_not_modelled_404`: one unknown action per default arm (EC2 CreateNatGateway, IAM, RDS, DynamoDB, SQS, Secrets Manager, KMS) records not-modelled-404, the wire still 404 ResourceNotFoundException. DynamoDB DescribeTable on a missing table, wire-identical to the DynamoDB default arm (dynamodb.go:201-204), records answered-from-rows.
- `TestContract_request_log_filter_refused`: unsupported filter names (Filter.1 Name=foo) on DescribeSecurityGroups, DescribeVolumes, DescribeRouteTables, DescribeAddresses, DescribeNetworkInterfaces and DescribeTags; AssociateAddress AllowReassociation=true; SSM DescribeParameters Filters; a non-Name ParameterFilter; and ListTagsForResource, AddTagsToResource and RemoveTagsFromResource with a ResourceType other than Parameter all record filter-refused. A 409 from a modelled conflict (named in the PR) records answered-from-rows. Each site answers the status it answers today.
- `TestContract_request_log_empty_collection`: DescribeVolumes, DescribeNatGateways, DescribeSnapshots and DescribeLaunchTemplates record empty-collection; DescribeVpcs on an empty account records answered-from-rows.
- Existing wire tests pass unchanged (ec2_test.go:157-164, sts_test.go:50-57, ssm_test.go:216).
- Each new test is shown failing by a cp-backup break (the marker deleted at one site per class), recorded in the PR.
- AGENTS.md line 151, the concepts.md ceiling note and CHANGELOG [Unreleased] are updated.
