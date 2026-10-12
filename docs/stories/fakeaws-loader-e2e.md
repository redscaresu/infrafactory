---
kind: code
repo: infrafactory
status: blocked
blocked_by: [fakeaws-loader-pin]
epic: fakeaws-loader
depends_on: [fakeaws-loader-pin]
touches: [internal/e2e/aws_load_fakeaws_test.go, internal/e2e/aws_reader_iam_fakeaws_test.go, .github/fakeaws-e2e-tests, docs/hld/2026-09-30-import-production-state-into-the-mocks.md, docs/epics/fakeaws-loader.md, docs/stories/fakeaws-loader-e2e.md]
---

# The reader's reads load into fakeaws in CI, the IAM e2e proves inline policies, and the epic closes

The infrafactory e2e work the pin does not force, kept out of fakeaws-loader-pin so that story
changes only what the bump breaks. It runs at the FAKEAWS_SHA the pin set.

- New internal/e2e/aws_load_fakeaws_test.go, listed in .github/fakeaws-e2e-tests. Each test starts
  its fakeaws with `-load-max-bytes` and an admin token.
  - TestAWSReaderTableLoadableByFakeaws POSTs one NotFound exchange per reader-table action except
    STS to /mock/load, and must load with nothing refused, so an action the reader reads and the
    loader lacks fails CI by name.
  - TestAWSReadLoadsIntoFakeaws loads the read TestAWSReadStep0AgainstFakeaws's setup records into
    a second fakeaws, with no refusal.
- TestAWSReadIAMAgainstFakeaws asserts that ListRolePolicies names `inline` and that GetRolePolicy
  answers the seeded document, replacing the `0 == 0` comment (:82-86).
- Close the epic: the HLD's Epics line (:485) becomes `fakeaws-loader — done (#…)` with no link,
  and this PR deletes docs/epics/fakeaws-loader.md and this story file.

**Done when:**
- CI 'AWS against fakeaws' is green, with a PASS line for each name in .github/fakeaws-e2e-tests.
- TestAWSReaderTableLoadableByFakeaws fails, naming the action, against a scratch fakeaws copy with
  one loader's registration removed (recorded in the PR).
- `grep -n 'no-op' internal/e2e/aws_reader_iam_fakeaws_test.go` prints nothing, and the test
  asserts one inline policy.
- docs/epics/fakeaws-loader.md no longer exists, `grep -rn 'epics/fakeaws-loader' docs STATUS.md`
  prints nothing, and `make doc-hygiene` passes.
