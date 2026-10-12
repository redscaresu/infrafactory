---
kind: docs
status: ready
epic: aws-hold-for-read
depends_on: []
touches: [docs/decisions/0040-aws-whole-scope-ownership.md, internal/harness/aws_scope_adr_test.go]
risk: high
---

# ADR-0040 amended: a deliberate hold keeps the claim inside rule 6, and its release is a reap under rules 4 and 5

First of the two ADR-0040 edits: this one, then prod-read-seam-adr-0040-amendment (which depends on
this slug). Every code story of this epic depends on it, because each tip commit cites it
(`ADR: none — implements ADR-0040's hold amendment (aws-hold-for-read-adr-0040)`) and
scripts/check_doc_hygiene.sh:182-192 checks that trailer by regex only.

Append one dated section at the file's end, added lines only. It records:
- **The hold** is `test scenarios/training/aws-web-live.yaml --hold`, aws-web-live only. It holds only an
  iteration with no failures, not interrupted (read from the run's own context, never the fresh
  teardown context), and with its state copy written and verified; any other end destroys, sweeps and
  releases as today. It is decided in the one teardown and the process exits, so it is **within
  rule 6**, not an exception to it. What is new is that the release happens in another process.
- **The release** is `reap <scenario> --take-over <holder> --release-hold`, a reap under rules 4
  (take-over only by name, a live local holder refused) and 5 (released only in
  `awsReleaseAfterCleanSweep`, after a clean sweep). Order: copy, destroy, sweep and reap, release.
  It refuses, before any AWS request, when the copy cannot be written or verified, and names plain
  `reap <scenario> --take-over <holder>` as the exit that destroys without the copy.
- **The copy** is `.infrafactory/holds/<scenario>/terraform-live.tfstate`, dir 0700, file 0600, a
  fixed path read from no environment variable (INFRAFACTORY_RUNSTORE_ROOT does not move it), never
  under `.infrafactory/runs/` or `.infrafactory/live/` (the UI serves both), gitignored, never committed.
  The copy is the authority: when the output dir's state is missing, release restores it from the
  copy, never the other way round.
- **While the copy exists**, generate, run and test of that scenario are refused, because generation
  removes the output dir that holds the only live state.
- **Nothing expires a hold**: `service.ttl` has no AWS consumer (run_keep.go:31-51).
- What the hold's real proof adds (a held stack, released from another process with its copy taken),
  without restating the Consequences' unproven list; most of it is proved (STATUS.md:8-13).

The sweep rows, the precedence pairs and rules 1-17 are not edited.

**Unwritten rules this story touches (S156d):** the claim is released in one place
(`awsReleaseAfterCleanSweep`); a failed or interrupted run reaches destroy, sweep and release
(#436/#438); an absent input is not an empty input; no account id in a committed file.

**Done when:**
- `git diff main -- docs/decisions/0040-aws-whole-scope-ownership.md` shows only added lines, in one dated section.
- A new TestADR0040RecordsTheHold in internal/harness/aws_scope_adr_test.go requires the amendment to
  name `--hold`, `--release-hold`, `.infrafactory/holds/`, 0700, 0600 and `awsReleaseAfterCleanSweep`,
  and these rules as phrases: the hold holds only a passing, uninterrupted run with its copy written;
  a failed or interrupted hold destroys and releases; the copy is written before the destroy and the
  release refuses without it, naming `reap <scenario> --take-over <holder>`; generate, run and test
  are refused while the copy exists; nothing expires a hold. It does not contain "exception to rule 6".
- The new test is shown failing by a cp-backup break (the amendment section removed from a cp copy
  of the ADR, then restored with cp), recorded in the PR.
- TestADR0040NamesEverySweepRowAndPrecedencePair passes unchanged; `make adr-index` leaves no diff;
  `make doc-hygiene` passes.
