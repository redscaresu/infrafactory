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

**Out of scope:** filters and overlap (fakeaws-data-sources); token, bind, request log; S3, ELBv2, ASG, CloudWatch, NAT gateways, launch templates, snapshots (larger-estate's queue); any infrafactory command.

**Constraints:** starts once prod-read-seam's ADR-0041 story merges; the real-fixture story is last. `/mock/reset` still deletes the snapshot, so the rehearsal's reset is restore. No handler-path inserts. Existing admin tests keep passing. docs/mockway-contract.md is not the fakeaws contract. Carried from fakeaws-data-sources planning (2026-10-12): image owner widening (the numeric Amazon owner id, architecture filters) and DescribeSecurityGroupRules are loader-owned; prod-read-seam-fakeaws-filtered-e2e leaves DescribeSecurityGroupRules out until this epic models it. From fakeaws#60 (2026-10-12): public SSM parameters (/aws/service/...) need one isPublicSSMParameter predicate used at every SSM site (tag ops, GetParameter by ARN, listing, state) before the loader writes them.

**Your part:** none for you. Lead: load the real step-0 recording into a private fakeaws on this machine and run read-back (no cloud).

**Areas:** `fakeaws:handlers/load.go (new)`; `fakeaws:handlers/ec2.go, iam.go, ssm.go`; `fakeaws:repository/ (load.go new, ec2.go, ec2_compute.go, iam.go)`; `fakeaws:coverage_matrix.yaml, internal/audit/`; `fakeaws:testdata/`; `fakeaws:AGENTS.md, concepts.md, CHANGELOG.md`; `.github/workflows/ci.yml (FAKEAWS_SHA)`; `docs/layer3/real-vs-mock-deltas.md (D7)`; `docs/operations.md`
