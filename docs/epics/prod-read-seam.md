---
status: active
hld: 2026-09-30-import-production-state-into-the-mocks
depends_on: []
---

# infrafactory reads a named AWS account, read-only, through one walled seam, into a recorded read under .infrafactory/: `infrafactory rehearse read` (Rollout 1, plus Rollout 0's real recording).

**Done when:**
- ADR-0041 "account-read contract, recorded-read and read-set formats" is accepted first: a read-set entry is `{service, protocol, action, params}`, the request half of an exchange; the recorded read is a list of exchanges `{service, protocol, action, params, status, body(raw)}` plus call count, duration and timestamp. fakeaws-rehearsal-instance's request log and rehearsal-against-the-copy's discovery both use it.
- `infrafactory rehearse read --account <id> [--principal <arn>] --region <r> --credentials <file> --read-set <file> --out <dir>` writes the recorded read. `--out` must resolve under `.infrafactory/` (dir 0700, file 0600), with a refusal test. Each invocation is one read step: it reads the credential file itself and holds the key in that process only.
- A read-set entry whose action is outside the reader table fails the read, naming every such entry.
- The doer refuses any request outside the reader table before it leaves the process. It admits a mutating EC2 action only with `DryRun=true`, from the preflight's own table. Table-driven test, one row per protocol: EC2 CreateVpc, IAM CreateAccessKey (form body), STS AssumeRole, SSM PutParameter (`X-Amz-Target`), a request with neither Action nor `X-Amz-Target` (fails closed), and the DryRun action sent without DryRun, all refused with nothing on the wire. Every reader-client constructor accepts only the walled doer.
- `sts:GetCallerIdentity` runs first and a wrong account refuses everything after it; the principal is checked only when given. The EC2 DryRun preflight refuses the read when any action answers anything but `UnauthorizedOperation`; the PR states that IAM and SSM writes rest on the policy test, since DryRun is EC2-only.
- A second credential-file reader admits exactly AWS_ACCESS_KEY_ID, AWS_SECRET_ACCESS_KEY and optional AWS_SESSION_TOKEN, mode 0600, never echoing a value; the Layer 3 reader and its "exactly two keys" contract are untouched; docs/operations.md updated in the same PR.
- Readers for the web stack: every modelled EC2 collection in a touched VPC (subnets, SGs + rules, route tables, gateways, interfaces, instances, EIPs, key pairs), DescribeAvailabilityZones, follow-ups (VPC attributes, instance attributes incl. raw user data, SG rules), IAM role + its policies through the raw replay (no SDK client, which would deserialize away the raw bytes ADR-0041 records), SSM GetParameter with values, images and public SSM parameters when the read set names them; name lookups for IAM role, key pair, parameter, SG-within-VPC. Secrets Manager and KMS are not in the table.
- Every list pages to the end and fails closed when truncated (ADR-0040 rule 10); tests with a Throttling-then-OK fake and a bounded give-up; the result reports call count and duration.
- Unit tests with a recording fake doer assert replayed params (Filter.N, GroupName.N, Owner.N) reach the wire verbatim.
- `docs/layer3/aws/reader-policy.json` ships, tied to the reader table by a test; the Layer 3 policy and its tests are unchanged.
- ADR-0023 amended (a credential that reaches prod is read-only, named to its account, policy in the runbook, present only for its read step); ADR-0040 amended (the seam reads beyond the Layer 3 scope, reading only); runbook names the policy.
- Proved against fakeaws in internal/e2e (the filtered and SG-name rows after fakeaws-data-sources' pin bump), then once against the Layer 3 account. The real proof covers EC2, SSM and STS only; IAM readers are proved against fakeaws and recorded as unproved against AWS (user, 2026-10-11: the SCP stays as is).
- The step-0 read set is a committed file of the wedge's expected requests (VPC by tag, AZs, that VPC's surroundings, the SG to import); before release, the lead checks the recording holds an exchange for each.
- The scrub is code: allowlist output (account id and ARNs mapped to one synthetic 12-digit id, user data and parameter values replaced), the same id map applied to the kept pre-destroy state, and a leak test (no 12-digit number but the synthetic id and a public image's owner, no non-placeholder UserData or Value) run before commit.

**Out of scope:** discovery (what goes in the read set), loading, the tfstate pull, any tofu; types beyond the web stack's; ELBv2/ASG/CloudWatch readers.

**Constraints:** ADR-0023 rules 1-2; ADR-0025 proof-first; ADR-0040 rule 10 and its sweep table untouched; the reader table is one Go source from which the policy, the wall and the docs derive; account id and region from flags, never infrafactory.yaml's `aws:` block; nothing imports aws-sdk-go-v2/config. Story order: ADR-0041; seam, wall, identity and credentials; policy and ADRs; EC2 readers; IAM and SSM readers (off the wedge's critical path); real recording and scrub last. The `rehearse read` CLI story waits for rehearsal-prepare-command-group (merged as #480); the real recording waits for aws-hold-for-read.

**Your part:**
- In the Layer 3 member account, from the org account-access role: create a separate read-only IAM user, attach `docs/layer3/aws/reader-policy.json`, write its key to a mode-0600 file and give the lead the path. (The existing Layer 3 user cannot serve: it holds reap's write actions.) Optionally an assumable role, if the session-token path should be proved against real STS.
- Lead, under the Layer 3 claim and the €5 cap: hold aws-web-live (aws-hold-for-read), run `rehearse read` with the committed step-0 read set, check the recording, keep the pre-destroy state, release; run the scrub and commit only the scrubbed fixtures.

**Areas:** `internal/harness/ (aws_reader*.go new, aws_identity.go, aws_client.go, policy test sibling)`; `internal/cli/ (rehearse_read*.go, golden files)`; `internal/e2e/`; `docs/layer3/aws/reader-policy.json (new)`; `docs/decisions/0041 (new), 0023, 0040, README.md`; `docs/operations.md`
