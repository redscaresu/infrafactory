---
kind: code
repo: fakeaws
status: ready
epic: fakeaws-loaded-identity
depends_on: [fakeaws-arn-builders-take-account, fakeaws-identity-store-and-flag, fakeaws-identity-iam-sts, fakeaws-identity-ec2]
touches: [handlers/admin.go, handlers/iam.go, handlers/ec2.go, handlers/admin_test.go, handlers/ssm.go, handlers/kms.go, handlers/dynamodb.go, handlers/secretsmanager.go, handlers/sqs.go, handlers/eks.go, handlers/rds.go, handlers/route53.go, handlers/s3.go, handlers/ssm_test.go, handlers/kms_test.go, handlers/dynamodb_test.go, handlers/secretsmanager_test.go, handlers/sqs_test.go, handlers/eks_test.go, handlers/rds_test.go, handlers/route53_test.go, handlers/s3_test.go, handlers/account_source_scan_test.go]
---

# SSM, KMS, DynamoDB, Secrets Manager, SQS, EKS, RDS, Route53 and S3 answer the stored account

Every remaining service gets the same swap: each `const account` and each direct `FakeAccountID` becomes an `AccountID()` read at the handler entry, before any transaction. A read error answers 500 in the service's error shape. The gathers read it too; collectState has already failed /mock/state on an error.

Per file:
- ssm.go (9 uses, :132-315): thread the account into the sub-funcs and ssmParameterMetadata. Public `/aws/service` parameters keep their empty account (arn.go:154-155).
- kms.go (:120,:384, kmsKeyMetadata): the default key policy's literal root principal (:163) is built from the account. KMS keys stay in the process-global kmsStore, and a key keeps the account it was created under. That is the known gap fakeaws-identity-store-and-flag records.
- dynamodb.go, secretsmanager.go, sqs.go (sqsQueueURL :101): use the account.
- eks.go: replace the 14 per-route consts (:118-528) and :455. rds.go: handleRDS and gatherRDSStateReal.
- route53.go (10 consts) and s3.go (7 consts, the owner id at :224): the account is the row key and S3's owner. Their ARNs carry none.

New `account_source_scan_test.go` closes the class. Non-test `handlers/*.go` reference `awsproto.FakeAccountID` only inside the accessor, and contain the literal `000000000000` nowhere except awsproto.go.

**Order:** ssm.go, kms.go, rds.go, dynamodb.go, sqs.go and secretsmanager.go are also in fakeaws-request-log-gap-markers, which this story comes after through arn-builders.

**Carried from fakeaws#54's review:**
- collectState reads the account once and passes it to every gather (EC2, IAM and the services here); no gather calls `AccountID()` again, so a second failed read can never list a block under `""` as clean.
- POST /mock/identity's empty-database check and the upsert hold the one connection across the check, so a concurrent create cannot commit a row under the old account between them; a test races a create against the identity change and finds no row under the old account.

**Done when:**
- In an app started with 123456789012, these carry 123456789012:
  - an SSM parameter ARN;
  - a KMS key ARN and its policy principal `arn:aws:iam::123456789012:root`;
  - a DynamoDB table ARN and a secret ARN;
  - an SQS queue URL and ARN;
  - an EKS cluster and node group ARN;
  - an RDS instance, cluster, subnet group and parameter group ARN.

  GET /mock/state lists those rows.
- In the same app, a hosted zone and a bucket created through the API are listed by ListHostedZones/ListBuckets and by /mock/state, and the bucket owner is 123456789012. Rows stored under 000000000000 in the same database are not listed.
- With the identity table dropped, one call per service answers 500 with no 000000000000 in the body.
- The source-scan test passes. `git grep -n -e FakeAccountID -e 000000000000` over the nine files prints nothing. Existing tests pass with no assertion line changed.
- Each new test is shown failing by a cp-backup break, recorded in the PR:
  - kms.go:163 restored to the literal (the KMS test and the source scan both fail);
  - one s3.go const restored.
