---
kind: code
repo: fakeaws
status: blocked
blocked_by: [fakeaws-request-log-gap-markers]
epic: fakeaws-loaded-identity
depends_on: [fakeaws-identity-store-and-flag, fakeaws-identity-ec2, fakeaws-identity-other-services]
touches: [repository/identity.go, repository/identity_test.go, handlers/handlers.go, handlers/ec2.go, handlers/admin.go, handlers/ssm.go, handlers/ec2_test.go, handlers/admin_test.go, handlers/ssm_test.go, AGENTS.md]
---

# A loaded database gets no fixture AMIs at boot, lazily, from /mock/images, or from the public al2023 parameter

repository/identity.go gains a reader and a setter for the `loaded` column, which already exists. The setter is fakeaws-loader's API; here only tests call it. The setter does not delete existing fixture rows; fakeaws-loader's replace transaction does that.

When the database is loaded, four sites change:
1. NewApplication skips the boot seed (handlers.go:53-60).
2. ensureAMIFixturesForRegion seeds nothing (ec2.go:3413-3420).
3. seedAMI stops matching ec2AMIFixtures in memory (admin.go:127-132). A fixture id such as `ami-0al2023x8664` then fails amiIDPattern with 400, as any non-hex id does.
4. GetParameter for the public al2023 parameter (ssm.go:150-157) reads rows instead of answering AL2023AMIID, so it answers ParameterNotFound until a row is loaded.

A failed flag read seeds nothing at the lazy site and fails NewApplication at boot. An unloaded database behaves as today. Reset clears the flag along with the row.

fakeaws-data-sources' AZ table must reuse this loaded reader, not add a second flag.

**Order:** this is an ec2.go story in the cross-epic chain, and it comes after other-services, which also edits ssm.go.

**Done when:**
- TestContract_ami_fixtures_not_seeded_when_loaded. A fresh file database is marked loaded through repository.New before the first NewApplication. Then:
  - us-east-1 holds no fixture AMI;
  - DescribeImages with ImageId.1=ami-0al2023x8664 answers 404 ResourceNotFoundException, today's code for a missing explicit id (ec2.go:2723-2727);
  - RunInstances with that id answers InvalidAMIID.NotFound;
  - GetParameter for the al2023 public name answers ParameterNotFound;
  - POST /mock/images with ami-0al2023x8664 answers 400;
  - POST /mock/images with ami-0123456789abcdef0 writes a row, and RunInstances with it then succeeds.
- A second case boots unloaded, sets the flag, and reopens: the earlier fixtures remain. The test documents that removing them is the loader's job.
- POST /mock/reset clears the flag, and the next DescribeImages in a region seeds the fixtures again. Restore and reopen keep the flag.
- With the flag read failing (identity table dropped), DescribeImages in an unseeded region writes no fixture row, and NewApplication returns an error.
- On unloaded databases, existing AMI, SSM and admin tests pass with no assertion line changed.
- The MUST docstring is in ec2.go or admin.go, AGENTS.md's contracts line lists the id, and the contract audits pass.
- Each new test is shown failing by a cp-backup break of each of the four sites, recorded in the PR.
