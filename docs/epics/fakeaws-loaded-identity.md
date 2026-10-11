---
status: active
hld: 2026-09-30-import-production-state-into-the-mocks
depends_on: []
---

# fakeaws answers with an account id and region set at runtime instead of the compile-time 000000000000, so an empty mock can already be the user's account before discovery and a loaded one keeps it across restore and restart.

**Done when:**
- An identity table holds account id and region; `-account`/`-region` flags and `POST /mock/identity` (behind the admin token when set) set it; every ARN builder, STS GetCallerIdentity, EC2 dispatch and `/mock/state` answer with it (today `awsproto.FakeAccountID`, ~98 non-test uses).
- The table is in `resetTables` (anti-pattern 6): `/mock/reset` returns to 000000000000; restore and restart keep the stored identity.
- The fixture-AMI seed is suppressed at both sites (boot and lazy) when a flag in the database says it was loaded; an unloaded database seeds as today.
- A provider block with `allowed_account_ids = ["<id>"]` configures against a fakeaws started with `-account <id>`, as a regression test and an example.
- Contract docstrings, `TestContract_*`, AGENTS.md (STS no longer "000000000000 only"); FAKEAWS_SHA bumped in a final story.

**Out of scope:** loading rows (fakeaws-loader); multi-account in one instance.

**Constraints:** its `handlers/ec2.go` stories run one after another with fakeaws-data-sources' (same file); default behaviour byte-identical when no flag is given; fakeaws AGENTS.md audits.

**Your part:** none.

**Areas:** `fakeaws:handlers/awsproto/awsproto.go, arn.go`; `fakeaws:handlers/ec2.go, sts.go, iam.go, ssm.go, admin.go, handlers.go`; `fakeaws:repository/ (identity table)`; `fakeaws:cmd/fakeaws/main.go`; `fakeaws:AGENTS.md`; `.github/workflows/ci.yml (FAKEAWS_SHA)`
