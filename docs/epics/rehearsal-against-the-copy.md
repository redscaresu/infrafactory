---
status: active
hld: 2026-09-30-import-production-state-into-the-mocks
depends_on: [rehearsal-passthrough, rehearsal-state-pull, fakeaws-loader, fakeaws-data-sources]
---

# The wedge: with the web stack's real state pulled, the base plans against a checked copy of prod and its drift is listed; the head's change is planned against it, applied to the copy with the real ids, converges, fails where AWS would, and reaches no cloud (Rollout 4; goals 3, 4, 6, 12).

**Done when:**
- `--base <ref>` (default `main`) and `--head <ref>` (default the working tree) are exported without a checkout of the user's tree: a committed ref with `git archive`, the working tree by copying what `git ls-files --cached --others --exclude-standard` lists, skipping listed paths deleted on disk, with tests that an uncommitted edit reaches the head's work directory and an unstaged deletion does not and each is gated and walled as its own work directory.
- Goal 6 runs at prepare time from the parsed configuration of both refs (root plus modules.json): a type either creates, reads or imports that fakeaws does not model refuses before discovery, naming every type, S3 by name. The modelled set is read from fakeaws's coverage_matrix.yaml at the pin plus a named data-source map, tied by a test. This epic owns the modelled-type source and the gap classifier.
- `rehearse` owns the loop: the private fakeaws already carries the user's account and region; the pulled state's ids seed the read set; plan base and head against the empty mock; the read set is the union of both request logs; one read step for the delta (prod-read-seam's command, its own process, the credential file read only there); full-replace load of the cumulative read; re-plan; until a pass adds no request and both plans succeed, capped at five. Then the names every planned create takes are read in one more step.
- The check runs every time: the readers against the loaded mock equal the recorded read field by field over SDK-parsed values; then, with the request log cleared, `plan -refresh-only -detailed-exitcode -out` of the pruned state, `show -json` resource_drift listed by address and attribute, the saved plan applied. Exit 1 is a gap; any request classified not-modelled, unimplemented, filter-refused or empty-collection (the unmodelled Describes only) refuses the rehearsal naming it. The classifier also runs after every later plan and apply.
- The copy is snapshotted with the drift-absorbed state; `rehearse reset` is `POST /mock/restore` plus that state, never `/mock/reset`; a rehearsal after reset plans identically.
- Base: planned, never applied, never refusing; every change listed by address and attribute as drift between base and prod. Head: planned against the same copy and state; the report splits its own changes, base drift it keeps, base drift it changes, and head changes on addresses the base also changes (flagged). Then the head's saved plan is applied and the head re-planned, which must be empty (a non-converging change is named).
- The report names account, region, both refs, the read's timestamp and age, what was read (types, VPCs, names), passes, call count and duration, and the known-limits pointer (deltas.md).
- Nothing writes pitfalls/, docs/mock-gaps.md or the run store; each gap is filed by hand as a fakeaws story.
- The e2e wedge runs with a second fakeaws standing in as prod, loaded with the scrubbed fixture plus an unrelated planted VPC, the scrubbed pre-destroy state, and a provider with `allowed_account_ids`. The real discover → read → load path runs through the doer seam. Asserts: the recorded read holds the tagged VPC, its surroundings and the state's ids, and not the unrelated VPC; an unchanged fixture gives an empty base plan; a planted tag edit in "prod" is listed as drift and the base plan lists it; the head (rename with `moved`, add a subnet, change a tag) shows exactly those as its own changes, applies, converges; an overlapping second subnet fails as AWS would; a planted real-looking credential in the shell never reaches any host.
- ADR-0009 amended (snapshot/restore for a baseline never applied by a run); ADR-0032 amended (refresh and check classified by what the mock answered); ADR-0042 amended; golden files; docs/operations.md.

**Out of scope:** new code with no state (rehearsal-without-state); a PR/CI mode (later, user 2026-10-11); new fakeaws types; the UI.

**Constraints:** only a read step sees the credential file (goal 8; once per pass); the modelled-type list derives from one source with a test; interrupt rules from #436/#438; ADR-0032 exit codes stay distinct; never `git checkout` in the user's tree.

**Your part:** none for you. Lead: run the wedge against the real step-0 recording and the real pre-destroy state on this machine (no cloud) and record the result in the PR; neither enters git.

**Areas:** `internal/cli/ (rehearse_refs.go, rehearse_discover.go, rehearse_load.go, rehearse_check.go, rehearse_compare.go, rehearse_reset.go, rehearse_report.go new; fakeaws client with token)`; `internal/harness/ (read-back comparer, modelled-type list)`; `internal/e2e/ (wedge)`; `docs/decisions/0009, 0032, 0042`; `docs/operations.md`; `docs/layer3/real-vs-mock-deltas.md`
