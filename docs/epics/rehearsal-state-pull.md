---
status: active
hld: 2026-09-30-import-production-state-into-the-mocks
depends_on: [rehearsal-walls, prod-read-seam]
---

# The base's tfstate is pulled in a throwaway gated directory, version-checked, pruned with no backup, and turned into read-set entries, so the base-then-head rehearsal has its starting state (goals 1, 9, 12; Rollout 4, state path first).

**Done when:**
- `rehearse prepare … --backend-credentials <file> [--state-config <dir>] [--backend-config k=v] [--workspace w]` (the state config defaults to the base ref's export) gates that configuration's `terraform {}` block: encryption and cloud refused; `backend "s3"` arguments allowlisted (assume_role/role_arn, profile, shared_*_files and endpoints refused); any other backend but local refused by name; a missing workspace refused, never created. It runs `tofu init -input=false` and `tofu state pull` under the backend credential file only, then deletes the throwaway copy right after the pull.
- An assumption test proves the S3 backend's init only reads: run against a SeaweedFS identity allowed only Get/List, so any write fails it. A state bucket using SSE-KMS is refused by name (user, 2026-10-11).
- `docs/layer3/aws/backend-reader-policy.json` (S3 Get/List on the named bucket and key) ships, tied to the admitted-backend table by a test.
- A state whose `terraform_version` is newer than the rehearsal's tofu is refused; older is accepted.
- A state-to-read-set table (resource type → reader action + id parameter, composite ids parsed for e.g. aws_security_group_rule, aws_route_table_association, aws_iam_role_policy_attachment) derives from the reader table, with a test that every modelled state type maps.
- Unmodelled types are pruned with `tofu state rm -backup=-` and listed. Only the pruned copy is kept, 0600, outside any config copy; the raw pull never persists; a test asserts a planted `secret_string` is absent from `.infrafactory/` after prune and after `end`.
- Fail-closed test: after the pull, a bare `tofu apply` in either work directory dies at a dead address.
- The pre-destroy state from the step-0 hold is scrubbed with the read's id map and committed as the CI fixture.
- ADR-0042 amended; golden files; docs/operations.md.

**Out of scope:** the refresh-only check, drift, base and head plans (rehearsal-against-the-copy); backends other than S3 and local; a real S3 backend in the Layer 3 account; SSE-KMS state; the registry lock warning (rehearsal-prepare-gate's).

**Constraints:** the backend credential file uses prod-read-seam's reader; the state copy lives outside the configuration copies; goal 6's modelled-type source for pruning is the one rehearsal-against-the-copy uses (a story here may build it first; the other reuses it).

**Your part:** none for you (a real S3-backend proof would need the SCP widened; default no).

**Areas:** `internal/cli/ (rehearse_state*.go new: pull, prune, version check)`; `internal/harness/ (state-to-read-set table)`; `internal/e2e/`; `docs/layer3/aws/backend-reader-policy.json (new)`; `docs/decisions/0042`; `docs/operations.md`
