---
kind: lead
status: blocked
blocked_by: [prod-read-seam-reader-user, prod-read-seam-rehearse-read-cli, prod-read-seam-scrub, prod-read-seam-fakeaws-filtered-e2e, prod-read-seam-iam-readers, prod-read-seam-adr-0040-amendment, aws-hold-for-read-real-proof]
epic: prod-read-seam
depends_on: [prod-read-seam-reader-user, prod-read-seam-rehearse-read-cli, prod-read-seam-scrub, prod-read-seam-fakeaws-filtered-e2e, prod-read-seam-iam-readers, prod-read-seam-adr-0040-amendment, aws-hold-for-read-real-proof]
touches: [internal/e2e/testdata/aws-step0/, internal/e2e/aws_step0_fixture_test.go, docs/epics/prod-read-seam.md, docs/hld/2026-09-30-import-production-state-into-the-mocks.md, STATUS.md]
risk: high
---

# Record aws-web-live once through `rehearse read`, scrub it, commit only the scrubbed fixtures, close the epic

Waits on aws-hold-for-read-real-proof, so the recording never runs on a hold not yet proved on real AWS.

**You:** say go before the paid run; the lead waits for it in the conversation.

**You:** before each read, create a fresh key with the runbook's create block (docs/operations.md § The reader policy) and give the lead the key file's path; the first read uses the key prod-read-seam-reader-user made, while its file is still there. The lead checks only the file's mode and key names.

**Values.** `--account` is the Layer 3 member account's 12-digit id and `--region` is its region, the same values filled into reader-policy.json. The lead passes them as flags on the command line, and never writes them to a committed file.

**Lead,** under the Layer 3 claim and the €5 cap:
1. Hold aws-web-live: `infrafactory test scenarios/training/aws-web-live.yaml --config <cfg> --hold` (operations.md ### Holding aws-web-live for a read).
2. Run `infrafactory rehearse read --account <id> --region <r> --credentials <reader file> --read-set internal/e2e/testdata/aws-step0/read-set.json --out .infrafactory/reads/step0`.
3. Check the recording.
4. End the read step. **You:** run the runbook's cleanup block (delete-access-key, remove the key file, list-access-keys shows 0). The lead checks the key file is absent. A read run again starts from a fresh key.
5. Release: `infrafactory reap scenarios/training/aws-web-live.yaml --config <cfg> --take-over <holder> --release-hold`, which keeps the pre-destroy tfstate at `.infrafactory/holds/aws-web-live/terraform-live.tfstate`, then destroys, sweeps empty, reaps and releases the claim.
6. Run cmd/aws-read-scrub over the recording and that state copy, writing into internal/e2e/testdata/aws-step0/.
7. Delete the raw recording and the pre-destroy state copy (which also lifts the guard on generate, run and test of aws-web-live).
8. Commit only the scrubbed files, with an ungated test that runs harness.CheckScrubbedRead over them.

This is the epic's last PR: it deletes docs/epics/prod-read-seam.md, marks the epic done in the HLD's ## Epics and updates STATUS.md.

**Done when:**
- The identity check passed, and every preflight DryRun action answered UnauthorizedOperation against the reader key; otherwise the run stops and the lead files a story.
- Before release, the lead checked that each read-set entry was answered with 200 or its named NotFound, that the VPC lookup by tag returned exactly one VPC, and that the absent-name SG lookup answered 200 with no groups.
- Release left the sweep empty and the claim released; the PR records the cost, under €5.
- The user reported list-access-keys showing 0 keys once the read ended. After the scrub, the key file, `.infrafactory/reads/step0` and the pre-destroy state no longer exist (ls shows each absent), and the PR says so.
- Only scrubbed files are committed. The committed-fixture leak test passes, and fails on a cp-backup copy with the real account id planted back. The account id is in no committed file and no PR body.
- The PR states that EC2, SSM and STS are proved against AWS, and IAM against fakeaws only (SCP; user, 2026-10-11).
