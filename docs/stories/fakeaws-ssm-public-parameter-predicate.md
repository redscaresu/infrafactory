---
kind: code
repo: fakeaws
status: ready
epic: fakeaws-loader
depends_on: []
touches: [handlers/ssm.go, handlers/ssm_test.go, repository/ssm.go, repository/ssm_test.go]
---

# One isPublicSSMParameter predicate decides every SSM site, the repository's ARN match included

The al2023 literal is compared at five sites in handlers/ssm.go (:30, 160, 181, 227, 260,
306-342, 351). Tag ops go straight to the repository, whose ssmRefMatch builds an account-scoped
ARN (repository/ssm.go:111, 128, 173), so a public parameter's account-less ARN never matches and
the account form wrongly would (fakeaws#60 left this to the loader).

One predicate, name prefix `/aws/service/`, replaces the literal everywhere:
- GetParameter by name, and by the account-less ARN `arn:aws:ssm:<region>::parameter/aws/service/…`.
- DescribeParameters never lists a public parameter, and /mock/state never gathers one.
- DeleteParameter, PutParameter, AddTagsToResource, RemoveTagsFromResource and
  ListTagsForResource on one are refused. Each refusal's code cites the SSM API reference, with an
  `UNVERIFIED(aws-code)` comment (no recording).

The ARN stays derived here; fakeaws-load-ssm-parameters stores the recorded one. The loader comes
later, so tests insert a public row through the repository.

**Done when:**
- `git grep -n 'al2023-ami-kernel-default' handlers/ssm.go repository/ssm.go` prints at most the
  fixture's own definition.
- A second public row (`/aws/service/ami-amazon-linux-latest/al2023-ami-kernel-default-arm64`)
  answers GetParameter by name and by `arn:aws:ssm:<region>::parameter/…`, is absent from
  DescribeParameters and /mock/state, and Delete, Put and the three tag ops on it are refused.
- The account-scoped form of a public ARN matches nothing; a private parameter's tag ops by its
  ARN work as before.
- A test sends every action handleSSM dispatches with a public name, and fails if any answers as
  it would for a private parameter.
