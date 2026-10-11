---
status: active
hld: 2026-09-30-import-production-state-into-the-mocks
depends_on: [rehearsal-passthrough, prod-read-seam, fakeaws-loader, fakeaws-data-sources]
---

# The wedge: new code that finds the real VPC by tag, adds a subnet and imports a real security group discovers what it touches, that is read from prod, loaded, checked, and the code plans and applies against the copy with the real ids, fails where AWS would, and reaches no cloud (Rollout 4; goals 3, 4, 6).

**Done when:**
- Goal 6 runs at prepare time from the parsed configuration (root plus modules.json): a type the code creates, reads or imports that fakeaws does not model refuses before discovery, naming every type, S3 by name. The modelled set is read from fakeaws's coverage_matrix.yaml at the pin plus a named data-source map, tied by a test. This epic owns the modelled-type source and the gap classifier; rehearsal-takeover reuses both.
- `rehearse` owns the loop: the private fakeaws already carries the user's account and region; plan against the empty mock; read the request log into a read set; one read step for the delta (prod-read-seam's command, its own process, the credential file read only there); full-replace load of the cumulative read; re-plan; until a pass adds no request and the plan succeeds, capped at five. Then the names every planned create would take (`tofu show -json`) are read in one more step. The read set and each pass are shown.
- The check runs every time: the readers against the loaded mock equal the recorded read field by field over SDK-parsed values; any request classified not-modelled, unimplemented, filter-refused or empty-collection (the unmodelled Describes only) refuses the rehearsal naming it. The classifier also runs after every passthrough plan/apply, so a gap first hit by `rehearse apply` is named as a gap.
- The mock is snapshotted with the starting state beside it; `rehearse reset` is `POST /mock/restore` plus the state copy, never `/mock/reset`; a rehearsal after reset plans identically.
- The report names account, region, the read's timestamp and age, what was read (types, VPCs, names), passes, call count and duration, and the known-limits pointer (deltas.md).
- Nothing writes pitfalls/, docs/mock-gaps.md or the run store; each gap is filed by hand as a fakeaws story.
- The e2e wedge runs with a second fakeaws standing in as prod, loaded with the scrubbed fixture plus an unrelated planted VPC, and a provider with `allowed_account_ids`. The real discover → read → load path runs through the doer seam; asserts: the recorded read holds the tagged VPC, its surroundings and the imported SG, and not the unrelated VPC; subnet added, SG imported with its real id, plan and apply exit 0; an overlapping second subnet fails as AWS would; a planted real-looking credential in the shell never reaches any host.
- ADR-0009 amended (snapshot/restore for a baseline never applied by a run); ADR-0032 amended (refresh and check classified by what the mock answered); ADR-0042 amended; golden files; docs/operations.md.

**Out of scope:** the tfstate pull, pruning and drift (rehearsal-takeover); new fakeaws types; the UI.

**Constraints:** only a read step sees the credential file (goal 8; user, 2026-10-11: once per pass); the modelled-type list derives from one source with a test; interrupt rules from #436/#438; ADR-0032 exit codes stay distinct.

**Your part:** none for you. Lead: run the wedge against the real step-0 recording on this machine (no cloud) and record the result in the PR; the recording never enters git.

**Areas:** `internal/cli/ (rehearse_discover.go, rehearse_load.go, rehearse_check.go, rehearse_reset.go, rehearse_report.go new; fakeaws client with token)`; `internal/harness/ (read-back comparer, modelled-type list)`; `internal/e2e/ (wedge)`; `docs/decisions/0009, 0032, 0042`; `docs/operations.md`; `docs/layer3/real-vs-mock-deltas.md`
