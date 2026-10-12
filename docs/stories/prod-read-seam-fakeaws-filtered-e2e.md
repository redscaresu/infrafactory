---
kind: code
status: blocked
blocked_by: [prod-read-seam-ec2-readers, fakeaws-data-sources-pin]
epic: prod-read-seam
depends_on: [prod-read-seam-ec2-readers, fakeaws-data-sources-pin]
touches: [internal/e2e/testdata/aws-step0/read-set.json, internal/e2e/aws_reader_step0_fakeaws_test.go]
---

# The step-0 read set, committed and proved against fakeaws with filters and AZs

This story waits for fakeaws-data-sources-pin, the FAKEAWS_SHA bump that brings fakeaws's filters and DescribeAvailabilityZones. DescribeSecurityGroupRules is not modelled at that pin (no case in fakeaws handlers/ec2.go); it is owned by fakeaws-loader, which stores the rule ids.

Commit `internal/e2e/testdata/aws-step0/read-set.json` in the ADR-0041 format. It holds:
- DescribeVpcs by `Filter` tag-key=infrafactory-run-id (aws_sweep.go:19). The claim leaves one stack in the account, so no run id is needed.
- DescribeAvailabilityZones.
- DescribeSecurityGroups by the same filter. That covers the SG the wedge imports, whose id is unknown before the apply.
- An SG-within-VPC lookup of a name that does not exist: DescribeSecurityGroups with `Filter` group-name plus tag-key, never `GroupName.N`.
- SSM GetParameter for AWSAL2023AMIParameter (aws_ami.go:20).

The file holds no ids, no run ids and no account ids. The e2e seeds fakeaws with a VPC tagged infrafactory-run-id, plus its subnet and SG, and reads it with this set.

**Done when:**
- In the e2e at the bumped pin, every read-set entry has an exchange. The VPC lookup by tag returns exactly the seeded VPC, the SG filter returns the seeded SG, the absent-name lookup records 200 with no groups, the AZs are recorded, and so is each row prod-read-seam-ec2-readers listed as refused at the old pin, except DescribeSecurityGroupRules (not modelled at this pin, fakeaws-loader's).
- A test fails if the read-set file holds a 12-digit number, a resource id (vpc-, sg-, subnet-, i-), a run-id value or a `GroupName.` key; the file passes ValidateAWSReadSet.
- The PR records a cp-backup break that turns each new test red.
