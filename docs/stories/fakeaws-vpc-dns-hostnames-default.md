---
kind: code
repo: fakeaws
status: ready
depends_on: [fakeaws-load-volumes-d7]
touches: [handlers/ec2.go, handlers/ec2_test.go]
---

# A created VPC answers AWS's DNS defaults: enableDnsSupport true, enableDnsHostnames false

Filed by fakeaws-loader's planning (2026-10-12), out of that epic's scope. Once
fakeaws-load-vpc-subnet-igw-az stores both attributes per VPC, a created VPC still stores today's
answers, true and true, while AWS creates a non-default VPC with enableDnsHostnames false (EC2 API
reference, ModifyVpcAttribute / DNS attributes for your VPC).

It waits for fakeaws-load-volumes-d7, the last of the loader's serial handlers/ec2.go chain, so
it never shares that file with a loader story.

Change the stored default for CreateVpc to true and false, with an `UNVERIFIED(aws-code)` comment
until a recording confirms it, and fix any example that relied on true.

**Done when:**
- DescribeVpcAttribute enableDnsHostnames on a newly created VPC answers false, and
  enableDnsSupport true.
- Every working example passes provider-smoke, an idempotent plan included.
