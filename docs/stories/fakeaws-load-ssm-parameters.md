---
kind: code
repo: fakeaws
status: ready
epic: fakeaws-loader
depends_on: [fakeaws-load-exchange-rules, fakeaws-ssm-public-parameter-predicate, fakeaws-schema-version-refuses-older]
touches: [handlers/load_ssm.go, handlers/load_ssm_test.go, handlers/ssm.go, handlers/ssm_test.go, repository/ssm.go, repository/ssm_test.go, repository/repository.go, repository/testdata/schema.golden, testdata/ssm/]
---

# Public SSM parameters load byte-exact with their recorded owner and ARN; a non-public one is refused naming what its recording cannot hold

A loader for GetParameter (JSON 1.1) in handlers/load_ssm.go.

**Owner and ARN.** A public parameter (isPublicSSMParameter) is AWS's, not the loaded account's:
its ARN has no account (`arn:aws:ssm:<region>::parameter/aws/service/…`). ssm_parameters has no
owner or ARN column (repository/ssm.go:18-32, PK account_id, region, name), so it gains a nullable
`arn` column holding the recorded ARN, byte-exact. A row with an ARN answers it, and the
repository's ARN match (ssmRefMatch) uses it; a row without one (PutParameter) derives it as today.
The row lives under the loaded account's key, so the loaded account sees it, while its ARN keeps
the real owner. It is an existing table, so dbSchemaVersion is bumped and the golden regenerated.

**Fields.** Type, DataType, Version, LastModifiedDate and Value load byte-exact. A non-public
parameter is refused, naming it and description, tier, allowed_pattern, key_id and tags:
aws_ssm_parameter's read also calls DescribeParameters and lists tags (terraform-provider-aws
v5.100.0 internal/service/ssm/parameter.go:331, @Tags :35), which the reader never records.
ParameterNotFound loads nothing. A public parameter whose Value is an AMI id returns that image as
a reference the reader never reads: answered under unresolved, unless the read holds the
DescribeImages for it (prod-read-seam-reader-parameter-image, loaded by fakeaws-load-images).

**Mutation.** A public parameter cannot be mutated (the predicate story refuses Put, Delete and
the tag ops).

Fixture: testdata/ssm/GetParameter.json from the SSM API reference, with its `.source` file.

**Done when:**
- The al2023 fixture loads into account 111122223333. GetParameter by name and by its account-less
  ARN answers a body load_diff.go finds equal to the fixture in both directions; the answer's ARN
  holds no account id.
- The ARN is stored, not derived: a fixture copy whose ARN differs from the derived form in the
  region field answers that recorded ARN, and GetParameter by it finds the row.
- The account-scoped form `arn:aws:ssm:<region>:111122223333:parameter/aws/service/…` matches
  nothing, and ListTagsForResource on it answers as for an absent parameter.
- A value with a trailing newline and a non-ASCII byte reads back byte-exact.
- On the loaded db the parameter is absent from DescribeParameters and /mock/state, and no other
  al2023 row exists.
- PutParameter on the loaded parameter is refused, and GetParameter still equals the fixture.
- Its AMI is answered under unresolved, naming the image id.
- Planting Parameter.Foo refuses the load naming GetParameter and `Parameter/Foo`.
- A `/myapp/db-host` GetParameter exchange refuses the load naming it and the five fields.
- A file at the previous schema version is refused.
