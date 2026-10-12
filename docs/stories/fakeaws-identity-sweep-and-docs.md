---
kind: code
repo: fakeaws
status: ready
epic: fakeaws-loaded-identity
depends_on: [fakeaws-identity-other-services, fakeaws-ami-fixtures-suppressed-when-loaded, fakeaws-allowed-account-ids-example]
touches: [handlers/account_sweep_test.go, handlers/admin.go, AGENTS.md, README.md, concepts.md]
---

# No response carries the default account under a stored identity; the fakeaws docs updated

**The class, closed end to end.** New `account_sweep_test.go` runs under 123456789012. It creates and reads one resource per modelled service: IAM, S3, EC2 VPC and instance, RDS, DynamoDB, EKS, SQS, Route53, Secrets Manager, KMS, SSM and STS. It asserts that neither any response body nor the GET /mock/state body contains 000000000000. The source scan already landed in fakeaws-identity-other-services. This test checks the wire.

admin.go gets a MUST docstring on the AccountID accessor; nothing else in admin.go changes.

**Docs.**
- AGENTS.md: the contract list; the layout's flags (:25) and admin.go row (:28); STS is no longer "000000000000 only" (:56-58).
- README.md: the STS row (:48). The flag rows, the /mock/identity row and CHANGELOG already landed in fakeaws-identity-store-and-flag.
- concepts.md:64: the admin list.

**Done when:**
- TestContract_responses_carry_the_stored_account passes.
- It is shown failing by two cp-backup breaks, recorded in the PR: kms.go:163 restored to the literal, and one ec2.go OwnerId restored to the constant.
- AGENTS.md lists the id, and the contract audits pass.
- `grep -n '000000000000' AGENTS.md README.md concepts.md` shows only lines that call it the default.
- concepts.md's admin list names POST /mock/identity.
