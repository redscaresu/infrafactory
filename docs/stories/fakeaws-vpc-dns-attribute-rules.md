---
kind: code
repo: fakeaws
status: blocked
blocked_by: [fakeaws-vpc-dns-hostnames-default]
depends_on: [fakeaws-vpc-dns-hostnames-default]
touches: [handlers/ec2.go, handlers/ec2_test.go, CHANGELOG.md]
---

# ModifyVpcAttribute refuses what real AWS refuses for the DNS attributes

`ec2ModifyVpcAttribute` (handlers/ec2.go) accepts `EnableDnsHostnames=true` on a VPC whose
`enableDnsSupport` is false, and accepts both attributes in one request. Real AWS answers 400
`InvalidParameterValue` to both ("You can only enable DNS hostnames if you've enabled DNS support";
"You cannot modify the DNS resolution and DNS hostnames attributes in the same request"). Since
fakeaws #96 made hostnames default to false, every config that sets `enable_dns_hostnames = true`
goes through this path, so the mock certifies a config real AWS refuses. Found in /code-review of #96.

**Done when:**
- `go test ./handlers/ -run 'TestEC2_ModifyVpcAttribute' -v` passes with cases: hostnames=true while support is false gives 400 InvalidParameterValue with AWS's message; both attributes in one request gives 400; hostnames=true with support true succeeds and reads back true.
- Each new case is shown failing with the check removed (cp backup).
- The provider smoke examples that set `enable_dns_hostnames = true` (web_step_one, eip_instance) still pass.
