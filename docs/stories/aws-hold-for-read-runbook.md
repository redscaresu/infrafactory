---
kind: docs
status: ready
epic: aws-hold-for-read
depends_on: [aws-hold-for-read-hold]
touches: [docs/operations.md, internal/cli/aws_scope_doc_test.go]
risk: high
---

# The runbook names `test --hold`, the release, the copy and the hold's cost bound

Split from the hold story. First of this epic's two docs/operations.md edits (then real-proof).
prod-read-seam-reader-policy, prod-read-seam-rehearse-read-cli, fakeaws-rehearsal-instance-pin-and-docs,
fakeaws-data-sources-pin and rehearsal-prepare-docs edit other sections of the same file; rebase if second.

New `### Holding aws-web-live for a read` under `## Layer 3 (AWS)`, after `### Running aws-web-live`.
It names no IAM action, because the section above is pinned to exactly one (aws_scope_doc_test.go:54-80).
It says:
- generate aws-web-live's HCL first, without the Layer 3 config, and see it green against fakeaws;
- `infrafactory test scenarios/training/aws-web-live.yaml --config <cfg> --hold`: it holds only a
  passing, uninterrupted run whose copy is written; anything else destroys, sweeps and releases;
- the copy: `.infrafactory/holds/aws-web-live/terraform-live.tfstate`, dir 0700, file 0600, never
  committed, carries the account id;
- while the copy exists, generate, run and test of aws-web-live are refused;
- `infrafactory reap scenarios/training/aws-web-live.yaml --config <cfg> --take-over <holder> --release-hold`:
  copy, destroy, sweep and reap, release;
- if the release refuses: plain `reap ... --take-over <holder>` destroys without the copy;
- the cost bound: one small instance and its root volume, per hour held; no NAT gateway or load
  balancer; nothing expires a hold, so release it the same day;
- delete the copy after the read.

**Unwritten rules it touches (S156d):** the existing section's one-IAM-action pin; advice names only
flags that exist (both merged by now); no account id in a committed file.

**Done when:**
- A new doc test in internal/cli/aws_scope_doc_test.go fails if the section drops `--hold`,
  `--release-hold`, `.infrafactory/holds/`, 0700, 0600, plain `--take-over <holder>` as the exit,
  or the cost bound's "per hour held".
- The new test is shown failing by a cp-backup break (a line removed from a cp copy of
  docs/operations.md, restored with cp), recorded in the PR.
- TestAWSRunChecklistNamesTheStagesAndTheReapCommand passes unchanged; `make doc-hygiene` passes.
