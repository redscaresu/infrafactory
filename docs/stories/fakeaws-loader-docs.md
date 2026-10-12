---
kind: docs
repo: fakeaws
status: blocked
blocked_by: [fakeaws-load-read-back, fakeaws-load-round-trip]
epic: fakeaws-loader
depends_on: [fakeaws-load-read-back, fakeaws-load-round-trip]
touches: [handlers/load.go, handlers/ec2.go, handlers/iam.go, handlers/ssm.go, handlers/admin.go, handlers/recorder.go, handlers/load_test.go, handlers/ec2_test.go, handlers/iam_test.go, handlers/ssm_test.go, handlers/admin_test.go, AGENTS.md, concepts.md, CHANGELOG.md]
---

# Contracts, AGENTS.md, concepts.md and CHANGELOG for the loader

One story for every doc, because AGENTS.md's contract list is one line enforced by
TestAgentsMDListsEveryContract, so a contract or CHANGELOG entry per story would serialise the
parallel IAM, SSM and schema lanes.

- The epic's invariants become contracts, as CRITICAL/MUST docstrings in handlers/load.go, ec2.go,
  iam.go, ssm.go, admin.go and recorder.go, with `TestContract_*` made from the stories' existing
  tests by rename, adding no behaviour. The ids: mock-load-off-by-default,
  mock-load-replaces-every-row-atomically, mock-load-keeps-snapshot, mock-load-refuses-unheld-field,
  mock-load-never-mints, mock-load-refuses-dangling-reference, mock-load-not-found-loads-nothing,
  restore-refuses-other-schema-version, ssm-public-parameter-one-predicate,
  iam-role-inline-policies-stored, ec2-sg-rule-ids-stable, ec2-volume-removed-on-terminate.
- AGENTS.md lists them, documents /mock/load and -load-max-bytes, and rewords the
  empty-collection line (:77).
- concepts.md: the one exception to "server-stamped fields are never trusted from the client"
  (:417-478), confined to handlers/load*.go and repository/load*.go; item 7 (:64) without the
  PutRolePolicy no-op and with DescribeVolumes' two classes; and, beside "No partial CRUD"
  (:479-486), the exemption for read-and-terminate-only volumes.
- CHANGELOG [Unreleased] gets one section for the epic, one entry per merged story naming its PR.

**Done when:**
- TestAllContractsHaveTests and TestAgentsMDListsEveryContract pass, and `git grep -c` finds each
  of the twelve ids in handlers/*.go.
- `grep -n PutRolePolicy concepts.md` shows no no-op claim, and AGENTS.md and concepts.md name
  DescribeVolumes as empty-collection only on an unloaded db.
- For each PR number the PR body lists as this epic's merged fakeaws PRs, `grep -c '#<n>'
  CHANGELOG.md` is at least 1.
- `git diff main` changes no test's assertions: test edits are renames and docstring tags only.
