---
kind: code
repo: fakeaws
status: ready
epic: fakeaws-loader
depends_on: [fakeaws-load-instances]
touches: [handlers/load_ec2_images.go, handlers/load_ec2_images_test.go, handlers/ec2.go, handlers/ec2_test.go, repository/ec2_compute.go, repository/load_ec2.go, testdata/ec2/DescribeImages.xml]
---

# Public images load with their numeric owner and alias, and Owner.N and architecture filters match them

A loader for DescribeImages in handlers/load_ec2_images.go. These are fakeaws-data-sources'
carried items (the epic's constraints).

- ec2_amis rows (repository/ec2_compute.go:78-88) keep owner_id numeric, as recorded, and gain,
  as struct fields in data JSON, imageOwnerAlias, architecture, creationDate, blockDeviceMapping,
  platformDetails, usageOperation, enaSupport, bootMode, imdsSupport, description, public and the
  rest the provider reads.
- ec2AMIToXML (handlers/ec2.go:3409-3420) answers them, and stops deriving the alias from the
  literal owner `amazon` for a loaded row. `Owner.N=amazon` matches a stored alias as well as the
  fixtures' literal.
- Filters architecture, owner-alias, is-public, root-device-type and platform-details join
  imageFilters (:3128-3170).
- Loaded images live under the loaded account and are visible to it; the fixtures stay
  suppressed. A loaded image resolves the AMI reference of a loaded instance and of a loaded al2023
  parameter (fakeaws-load-ssm-parameters), so neither is answered under unresolved when the read
  holds its DescribeImages (prod-read-seam-reader-parameter-image).

Images have no mutating action in fakeaws, so there is no mutate-once check.

**Done when:**
- A loaded image with owner 137112412989 and alias amazon answers `Owner.1=amazon` with a name
  filter, and `Owner.1=137112412989`; its body equals the fixture by load_diff.go in both
  directions.
- Filter `architecture=arm64` excludes the x86_64 image. On an unloaded db the fixtures answer as
  before (TestRegressionSweepDescribeImagesOwnerSelf unchanged).
- A read holding the al2023 GetParameter and a DescribeImages for its value loads with nothing
  under unresolved for that AMI.
- examples/working/ec2_data_sources still passes provider-smoke.
