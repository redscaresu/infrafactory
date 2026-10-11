---
kind: lead
status: blocked
blocked_by: [prod-read-seam-reader-policy]
epic: prod-read-seam
depends_on: [prod-read-seam-reader-policy]
touches: []
risk: high
---

# Create the read-only reader user and its key file

The existing Layer 3 user cannot serve, because it holds reap's write actions (iam-policy.json). The reader user and its key stay between rehearsals; only the local key file comes and goes (ADR-0023 amendment, prod-read-seam-reader-policy). No assumable role: the session-token path is tested against fakes only.

**You:** in the Layer 3 member account, from OrganizationAccountAccessRole, create a separate IAM user for reading.

**You:** attach docs/layer3/aws/reader-policy.json, from the merged SHA, with REGION replaced by the Layer 3 member account's region and ACCOUNT_ID by its 12-digit id, and attach nothing else.

**You:** create an access key and write it to a file outside the repo (for example `~/.config/infrafactory/aws-reader.env`) as two KEY=VALUE lines, `AWS_ACCESS_KEY_ID=…` and `AWS_SECRET_ACCESS_KEY=…`, then `chmod 600` it.

**You:** give the lead the path; that is how the lead learns this step is done.

**Lead:** checks only the file's mode and its key names (stat, and each line cut before `=`), never a value.

**Done when:**
- The user confirms the reader user exists with only reader-policy.json, at the merged SHA, attached, and gives the file's path.
- The lead's check shows mode 0600 and exactly the two key names, AWS_ACCESS_KEY_ID and AWS_SECRET_ACCESS_KEY.
- No key value appears in any transcript, file or PR.
