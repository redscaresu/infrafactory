---
kind: code
status: ready
repo: fakeaws
---

# fakeaws KMS returns the key description it was given, so an aws_kms_key with a description converges

`kmsKeyMetadata` (`handlers/kms.go`, at 76ea106) answers `"Description": ""` whatever
`CreateKey` or `UpdateKeyDescription` sent. An `aws_kms_key` with a `description` applies, then
its second plan shows `+ description`, so every AWS run that sets one stops on drift. infrafactory's
sealed every-service e2e (`internal/e2e/testdata/aws-env-only/every-service.tf`) leaves the
description out for this reason.

**Done when:** `CreateKey` stores `Description`, `UpdateKeyDescription` changes it, and
`DescribeKey` returns it; a `working/kms_key` smoke example with a description passes apply,
`plan -detailed-exitcode` 0 and destroy; and after the infrafactory CI's `FAKEAWS_SHA` is bumped
past it, every-service.tf sets `description` on its key.
