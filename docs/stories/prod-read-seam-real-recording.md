---
kind: lead
status: blocked
blocked_by: [prod-read-seam-reader-user, prod-read-seam-rehearse-read-cli, prod-read-seam-scrub, prod-read-seam-fakeaws-filtered-e2e, prod-read-seam-iam-readers, prod-read-seam-adr-0040-amendment, aws-hold-for-read-real-proof, prod-read-seam-reader-describe-volumes, prod-read-seam-reader-parameter-image]
epic: prod-read-seam
depends_on: [prod-read-seam-reader-user, prod-read-seam-rehearse-read-cli, prod-read-seam-scrub, prod-read-seam-fakeaws-filtered-e2e, prod-read-seam-iam-readers, prod-read-seam-adr-0040-amendment, aws-hold-for-read-real-proof, prod-read-seam-reader-describe-volumes, prod-read-seam-reader-parameter-image, fakeaws-load-read-back]
touches: [internal/e2e/testdata/aws-step0/, internal/e2e/aws_step0_fixture_test.go, docs/epics/prod-read-seam.md, docs/hld/2026-09-30-import-production-state-into-the-mocks.md, STATUS.md, docs/stories/fakeaws-loader-real-read-back.md]
risk: high
costs: money
---

# Record aws-web-live once through `rehearse read`, scrub it, commit only the scrubbed fixtures, close the epic

Waits on aws-hold-for-read-real-proof, so the recording never runs on a hold not yet proved on real AWS. Waits on prod-read-seam-reader-describe-volumes and prod-read-seam-reader-parameter-image, so the recording holds each instance's volumes and the image the al2023 parameter names. Waits on fakeaws-load-read-back, so step 5b can load the raw recording into fakeaws before step 6 deletes it (fakeaws-loader-real-read-back); this delays the recording until the loader is built, which costs less than a second paid read.

**You:** say go before the paid run; the lead waits for it in the conversation.

**You:** give the lead the reader key file's path. After each read the lead deletes that file, so for every later read write it again (mode 0600, the same two KEY=VALUE lines) from where you keep the key. The lead checks only the file's mode and key names.

**Values.** `--account` is the Layer 3 member account's 12-digit id and `--region` is its region, the same values filled into reader-policy.json. The lead passes them as flags on the command line, and never writes them to a committed file.

**Lead,** under the Layer 3 claim and the €5 cap:
1. Hold aws-web-live: `infrafactory test scenarios/training/aws-web-live.yaml --config <cfg> --hold` (operations.md ### Holding aws-web-live for a read).
2. Run `infrafactory rehearse read --account <id> --region <r> --credentials <reader file> --read-set internal/e2e/testdata/aws-step0/read-set.json --out .infrafactory/reads/step0`.
3. Check the recording, including a DescribeVolumes exchange for the held instance and a DescribeImages for the al2023 parameter's value.
4. Release: `infrafactory reap scenarios/training/aws-web-live.yaml --config <cfg> --take-over <holder> --release-hold`, which keeps the pre-destroy tfstate at `.infrafactory/holds/aws-web-live/terraform-live.tfstate`, then destroys, sweeps empty, reaps and releases the claim.
5. Run cmd/aws-read-scrub over the recording and that state copy, writing into internal/e2e/testdata/aws-step0/.
5b. Run fakeaws-loader-real-read-back on the raw recording: `FAKEAWS_LOAD_READ=<raw recording> go test ./handlers/ -run TestLoadReadBack -count=1` in a fakeaws checkout at the main holding fakeaws-load-read-back (in process, `:memory:`, no cloud). Record in this PR only PASS, or the failing actions and paths, and the per-type loaded, absent and unresolved counts; file a fakeaws story for each refusal or difference.
6. End the read step, only after step 5b has run: delete the local key file, the raw recording and the pre-destroy state copy (which also lifts the guard on generate, run and test of aws-web-live), and check the key file is absent. The IAM user and its key stay.
7. Commit only the scrubbed files, with an ungated test that runs harness.CheckScrubbedRead over them.

This is the epic's last PR: it deletes docs/epics/prod-read-seam.md and docs/stories/fakeaws-loader-real-read-back.md, marks the epic done in the HLD's ## Epics and updates STATUS.md.

**Done when:**
- The identity check passed, and every preflight DryRun action answered UnauthorizedOperation against the reader key; otherwise the run stops and the lead files a story.
- Before release, the lead checked that each read-set entry was answered with 200 or its named NotFound, that the VPC lookup by tag returned exactly one VPC, and that the absent-name SG lookup answered 200 with no groups.
- Release left the sweep empty and the claim released; the PR records the cost, under €5.
- Step 5b ran before step 6: the PR records TestLoadReadBack's PASS, or each refused action and path with its fakeaws story, and the per-type counts, with no account id, ARN or value.
- After the scrub, the key file, `.infrafactory/reads/step0` and the pre-destroy state no longer exist (ls shows each absent), and the PR says so.
- Only scrubbed files are committed. The committed-fixture leak test passes, and fails on a cp-backup copy with the real account id planted back. The account id is in no committed file and no PR body.
- The PR states that EC2, SSM and STS are proved against AWS, and IAM against fakeaws only (SCP; user, 2026-10-11).
