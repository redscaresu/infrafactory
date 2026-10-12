---
status: active
hld: 2026-09-30-import-production-state-into-the-mocks
depends_on: [fakeaws-loaded-identity]
---

# fakeaws loads a recorded read as rows that answer exactly as prod did, with every field it cannot hold refused by name: `POST /mock/load`, models widened to the real response shape, the instance root volume (Rollout 2).

**Done when:**
- `POST /mock/load` (ADR-0041 format, behind the admin token) replaces every row in one transaction, FK order resolved, and sets the stored identity; loading again is a defined full replace. ids, ARNs, owner ids, default SGs, main route tables and private DNS names are what the body held, never minted; `created_at` only for types whose response carries one.
- Field refusal is a generic XML/JSON tree diff of each body against what the model can hold, never the handlers' decoders (encoding/xml drops unknown elements silently). Tests plant an unknown nested element (XML) and an unknown key (JSON 1.1) and see the load refused naming type and field; a type with no table refuses naming the type; the response lists what loaded per type.
- Per-type widening of models and XML to the real shape for the web-stack types (VPC, subnet, SG and rules, route table, IGW, ENI with nullable instance, EIP, key pair, instance with its stored DescribeInstanceAttribute values, IAM role with inline and attached policies and instance profile, SSM parameter, AZ). Each is driven by a real-shaped testdata fixture from the AWS API reference or the scrubbed step-0 recording, never by fakeaws's own output.
- An explicit, test-pinned ignore list only for fields the 5.100.0 provider never reads, each with a provider source cite.
- IAM role inline policies are stored: PutRolePolicy, GetRolePolicy and ListRolePolicies answer from a new table (today a no-op and an empty list).
- Public AMIs and public SSM parameters load with their real owner and are visible to the loaded account; user data and SSM values load byte-exact and read back; tests for both.
- D7: an instance loaded with its DescribeVolumes pair has a root volume that DescribeVolumes returns and terminate removes; this epic owns the volume table, and updates fakeaws-rehearsal-instance's `empty-collection` classification for DescribeVolumes. A standalone interface loads without an instance.
- The AZ table is filled from a recorded DescribeAvailabilityZones.
- Round trip limited to the reader table's types: apply each such example, record, load into a fresh db, `tofu plan` exits 0. It proves load, not real shape. coverage_matrix.yaml gains a `loadable` column enforced by TestFullCoverageAudit.
- The scrubbed step-0 fixture loads and reads back equal; body ceiling is a flag; contract docstrings, `TestContract_*`, AGENTS.md, concepts.md, infrafactory `docs/operations.md` and deltas.md D7 updated; FAKEAWS_SHA bumped in a final story.

**Checks before every stage** (every story's PR meets these; the stories do not repeat them):
- fakeaws: `go vet ./...`, `go test ./...` and provider-smoke pass. Existing admin tests pass unchanged, except the two GetSecurityGroupRules break cases (handlers/admin_test.go:142, :146), which fakeaws-sg-rule-model replaces with equivalents on the new rule table.
- Each new test is shown failing by a cp-backup break of the code it covers, recorded in the PR.
- Loader writes go only through repository/load*.go: no handler-path inserts. `git grep -n -e ec2RandID -e randHex -e time.Now handlers/load*.go repository/load*.go` prints nothing, and a type whose response carries no timestamp stores `created_at` empty.
- A widened field is a struct field, never a data-JSON-only key: the Update, Modify and Associate paths marshal the struct back into `data` (repository/ec2.go:398-410, :941-944; ec2_compute.go:359-363, :531-539), so a key the struct lacks is lost on the first mutation.
- A change to an existing table's, index's or trigger's SQL bumps the db schema version and regenerates the schema golden (fakeaws-schema-version-refuses-older); a new table, index or trigger is appended to the golden.
- Every new table survives load, snapshot, mutate, restore.
- Every fixture names its source (a first-line XML comment, or a sibling `.source` file for JSON): the AWS API reference or the scrubbed step-0 at an infrafactory commit, never fakeaws output.
- No new CRITICAL/MUST id, and no CHANGELOG.md, AGENTS.md or concepts.md edit, outside fakeaws-loader-docs.

**Out of scope:** filters and overlap (fakeaws-data-sources); token, bind, request log; S3, ELBv2, ASG, CloudWatch, NAT gateways, launch templates, snapshots (larger-estate's queue); any infrafactory command. Also these deviations the plan takes (2026-10-12):
- Non-public SSM parameters are refused at load, naming their unrecorded fields: aws_ssm_parameter's read also calls DescribeParameters and lists tags, which the reader never records. Lifting that is a reader follow-up (larger-estate). Public parameters load with their recorded account-less ARN.
- No data-preserving schema migration: a db or snapshot at another schema version is refused, never migrated (a mock db is disposable).
- Volumes are read-and-terminate only: no CreateVolume, and RunInstances still makes no root volume, so D7 stays open for an applied instance.
- CreateVpc still makes no main route table (fakeaws-loader-pin records it in D7). A created VPC keeps today's DNS attribute answers (true, true), though AWS defaults enableDnsHostnames to false: fakeaws-vpc-dns-hostnames-default (later).
- ModifySecurityGroupRules stays refused (501). Deleting a group that another group's rule references stays allowed, as today (AWS answers DependencyViolation).
- References the reader never reads load as unresolved, named in the answer and never minted: instance imageId, an iamInstanceProfile with no IAM exchange, VPC dhcpOptionsId, prefix-list ids, customer-managed policy ARNs, a volume's snapshotId and KMS key, and the AMI a loaded al2023 parameter names when the read holds no DescribeImages for it (prod-read-seam-reader-parameter-image adds that read).
- DescribeInstanceCreditSpecifications (aws_instance's read, burstable types) is not read; fakeaws-loader-pin records the gap in deltas.md.
- IAM, images and EIPs are proved on API-reference fixtures only: the step-0 read holds no IAM (SCP), no non-public SSM, and an image only through the al2023 follow-up.

**Constraints:** starts once prod-read-seam's ADR-0041 story merges; the real-fixture story is the last fakeaws story. `/mock/reset` still deletes the snapshot, so the rehearsal's reset is restore. No handler-path inserts. Existing admin tests keep passing (the one exception is in the checks above). docs/mockway-contract.md is not the fakeaws contract. Carried from fakeaws-data-sources planning (2026-10-12): image owner widening (the numeric Amazon owner id, architecture filters) and DescribeSecurityGroupRules are loader-owned; prod-read-seam-fakeaws-filtered-e2e leaves DescribeSecurityGroupRules out until this epic models it. From fakeaws#60 (2026-10-12): public SSM parameters (/aws/service/...) need one isPublicSSMParameter predicate used at every SSM site (tag ops, GetParameter by ARN, listing, state) before the loader writes them. Cross-epic order: prod-read-seam-reader-describe-volumes merges before the user creates the reader user, and prod-read-seam-real-recording runs fakeaws-loader-real-read-back as its step 5b, before it deletes the raw recording.

**Your part:** none beyond prod-read-seam's: the reader user is created after prod-read-seam-reader-describe-volumes merges, so its policy holds ec2:DescribeVolumes. Lead: load the raw step-0 recording into a private in-process fakeaws and read it back, at prod-read-seam-real-recording's step 5b (no cloud).

**Areas:** `fakeaws:handlers/load*.go (new)`; `fakeaws:handlers/ec2.go, ec2_filters.go, iam.go, ssm.go, admin.go, handlers.go, recorder.go, export_test.go, regression_audit_test.go`; `fakeaws:cmd/fakeaws/`; `fakeaws:repository/ (load*.go new, ec2_volumes.go new, repository.go, ec2.go, ec2_compute.go, ec2_tags.go, iam.go, ssm.go, testdata/ new)`; `fakeaws:coverage_matrix.yaml, internal/audit/`; `fakeaws:examples/ (load_round_trip_test.go new, known_red_test.go, working/iam_role_inline_policy new, README.md)`; `fakeaws:testdata/`; `fakeaws:AGENTS.md, concepts.md, CHANGELOG.md, README.md, SECURITY.md, .github/workflows/ci.yml`; `.github/workflows/ci.yml (FAKEAWS_SHA)`; `internal/e2e/aws_reader_ec2_fakeaws_test.go, aws_reader_step0_fakeaws_test.go` (only the expectations the pin bump breaks, in fakeaws-loader-pin: CI runs them at the PR's FAKEAWS_SHA, so they cannot land apart from the bump); `internal/e2e/aws_load_fakeaws_test.go (new), aws_reader_iam_fakeaws_test.go, .github/fakeaws-e2e-tests` (fakeaws-loader-e2e); `docs/layer3/real-vs-mock-deltas.md (D7)`; `docs/operations.md`; `docs/stories/fakeaws-known-red-examples.md`; `docs/hld/2026-09-30-import-production-state-into-the-mocks.md (Epics line)`
