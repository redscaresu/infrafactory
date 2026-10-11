---
kind: code
repo: fakeaws
status: ready
epic: fakeaws-loaded-identity
depends_on: [fakeaws-arn-builders-take-account, fakeaws-identity-store-and-flag, fakeaws-identity-iam-sts]
touches: [handlers/ec2.go, handlers/ec2_test.go]
risk: high
---

# EC2 dispatch, owner ids and gather answer the stored account

This story touches ec2.go only.

**Dispatch.** handleEC2's `const account` (:49) becomes a single `AccountID()` read before the switch. A read error answers 500 in ShapeEC2Query, never the default. gatherEC2StateReal (:3236) also reads the accessor; collectState has already failed /mock/state on an error.

**Account sites.** The 11 OwnerId/InstanceOwnerId fields that use the constant directly (:597,632,665,849,946,969,1653,2092,2101,2274,2324) take the account from their caller, as do these helpers:
- ec2VpcToXML, ec2SubnetToXML, ec2IgwToXML and ec2ENIToXML;
- ec2InstanceToXML (:2061-2071), which passes the account on to ec2ENIToXML and to BuildIAMInstanceProfileARN.

The lazy fixture seed (:2227,:2705) then seeds under the stored account. The region still comes from chi.URLParam.

**Order:** this story is in the cross-epic ec2.go chain, one ec2.go story open at a time. It comes after fakeaws-request-log-gap-markers, through arn-builders. When fakeaws-data-sources' stories are planned, each of its ec2.go stories names its order against this story by slug.

**Done when:**
- In an app started with 123456789012, these carry 123456789012:
  - CreateVpc/DescribeVpcs, CreateSubnet, CreateInternetGateway, CreateRouteTable/DescribeRouteTables and CreateSecurityGroup/DescribeSecurityGroups;
  - RunInstances/DescribeInstances: OwnerId, InstanceOwnerId, the ENI, and the IamInstanceProfile Arn of a profile created through the IAM API.

  GET /mock/state ec2 lists the rows with ARNs that carry it.
- Lazy seed: in ap-south-1, a region outside the boot list, RunInstances with the AL2023 fixture id succeeds, and the instance's OwnerId is 123456789012.
- With the identity table dropped through Repository().DB(), DescribeVpcs answers 500 and the body has no 000000000000.
- `git grep -n -e FakeAccountID -e 000000000000 handlers/ec2.go` prints nothing. Existing ec2 tests pass with no assertion line changed.
- Each new test is shown failing by a cp-backup break, recorded in the PR:
  - one OwnerId site reverted to the constant;
  - the lazy seed passed the constant;
  - handleEC2 ignoring the read error.
