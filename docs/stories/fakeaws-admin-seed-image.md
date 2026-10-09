---
kind: code
status: ready
repo: fakeaws
epic: aws-web-live-on-real-aws
depends_on: []
touches: [handlers/admin.go, handlers/admin_test.go]
---

# fakeaws takes an admin seed for one image, so a caller can register an AMI id it resolved elsewhere

fakeaws refuses RunInstances on any image it has not seeded, with `InvalidAMIID.NotFound`
(handlers/ec2.go:1961 at the infrafactory CI pin, FAKEAWS_SHA ee7656a in infrafactory's
.github/workflows/ci.yml:224). Its only AL2023 image is the fixture `ami-0al2023x8664`
(handlers/ec2.go:2526). The admin surface (`registerAdminRoutes`, handlers/admin.go:43-52, same on
origin/main) has reset, snapshot, restore and state, and no way to add an image. infrafactory's
Layer 2 deploy of an aws Layer 3 run needs one: the gate requires the instance's ami to be the id
read from real SSM, and that id has to exist in fakeaws before the mock apply.

Add `POST /mock/images` (admin, unauthenticated like its siblings). The body names the image id,
name, region and root device name, plus the owner and virtualization type with the AL2023
fixture's values as defaults, and the account it is seeded into. It writes through
`repo.SeedAMI` (repository/ec2_compute.go:538), so DescribeImages and RunInstances see it exactly
as they see a fixture. An id that is not `ami-` followed by hex, or a missing region or root device
name, is a 400 with nothing written. Seeding an id twice is idempotent (SeedAMI's INSERT OR
IGNORE). `/mock/reset` drops it, as it drops all state; the caller seeds again after a reset. No
change to the refusal itself: an unseeded id still fails RunInstances.

**Done when:**
- A handler test seeds `ami-0123456789abcdef0` in us-east-1. DescribeImages by that id returns it
  with the seeded root device name, and RunInstances with it succeeds. RunInstances with any other
  unseeded id still returns `InvalidAMIID.NotFound`.
- A body with a malformed id, no region or no root device name returns 400, and DescribeImages
  then does not list the id.
- After `/mock/reset`, RunInstances with the seeded id returns `InvalidAMIID.NotFound` again.
- The endpoint is listed wherever fakeaws documents its `/mock/*` admin routes, and fakeaws CI
  passes on the PR.
