---
kind: lead
status: blocked
blocked_by: [prod-read-seam-reader-user, prod-read-seam-rehearse-read-cli, prod-read-seam-scrub, prod-read-seam-fakeaws-filtered-e2e, prod-read-seam-iam-readers, prod-read-seam-adr-0040-amendment, aws-hold-for-read]
epic: prod-read-seam
depends_on: [prod-read-seam-reader-user, prod-read-seam-rehearse-read-cli, prod-read-seam-scrub, prod-read-seam-fakeaws-filtered-e2e, prod-read-seam-iam-readers, prod-read-seam-adr-0040-amendment, aws-hold-for-read]
touches: [internal/e2e/testdata/aws-step0/, internal/e2e/aws_step0_fixture_test.go, docs/epics/prod-read-seam.md, docs/hld/2026-09-30-import-production-state-into-the-mocks.md, STATUS.md]
risk: high
---

# Record aws-web-live once through `rehearse read`, scrub it, commit only the scrubbed fixtures, close the epic

`aws-hold-for-read` is that epic's slug, because it is not yet planned; once it is, re-point blocked_by and depends_on to its hold-command story's slug.

**You:** say go before the paid run; the lead waits for it in the conversation.

**You:** give the lead the reader key file's path. After each read the lead deletes that file, so for every later read write it again (mode 0600, the same two KEY=VALUE lines) from where you keep the key.

**Values.** `--account` is the Layer 3 member account's 12-digit id and `--region` is its region, the same values filled into reader-policy.json. The lead passes them as flags on the command line, and never writes them to a committed file.

**Lead,** under the Layer 3 claim and the €5 cap:
1. Hold aws-web-live with aws-hold-for-read's command.
2. Run `infrafactory rehearse read --account <id> --region <r> --credentials <reader file> --read-set internal/e2e/testdata/aws-step0/read-set.json --out .infrafactory/reads/step0`.
3. Check the recording, and keep the pre-destroy tfstate.
4. Release: destroy, sweep empty, reap, claim released.
5. Run cmd/aws-read-scrub over the recording and the state, writing into internal/e2e/testdata/aws-step0/.
6. End the read step: delete the local key file, the raw recording and the pre-destroy state. The IAM user and its key stay.
7. Commit only the scrubbed files, with an ungated test that runs harness.CheckScrubbedRead over them.

This is the epic's last PR: it deletes docs/epics/prod-read-seam.md, marks the epic done in the HLD's ## Epics and updates STATUS.md.

**Done when:**
- The identity check passed, and every preflight DryRun action answered UnauthorizedOperation against the reader key; otherwise the run stops and the lead files a story.
- Before release, the lead checked that each read-set entry was answered with 200 or its named NotFound, that the VPC lookup by tag returned exactly one VPC, and that the absent-name SG lookup answered 200 with no groups.
- Release left the sweep empty and the claim released; the PR records the cost, under €5.
- After the scrub, the key file, `.infrafactory/reads/step0` and the pre-destroy state no longer exist (ls shows each absent), and the PR says so.
- Only scrubbed files are committed. The committed-fixture leak test passes, and fails on a cp-backup copy with the real account id planted back. The account id is in no committed file and no PR body.
- The PR states that EC2, SSM and STS are proved against AWS, and IAM against fakeaws only (SCP; user, 2026-10-11).
