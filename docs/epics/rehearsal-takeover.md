---
status: active
hld: 2026-09-30-import-production-state-into-the-mocks
depends_on: [rehearsal-against-the-copy]
---

# New code that continues or takes over an existing configuration rehearses from its pulled tfstate: the pull in a throwaway gated directory, pruning with no backup, the version check, the refresh-only check with drift absorbed (Rollout 4, last clause; goals 1, 4, 9).

**Done when:**
- `rehearse prepare --state-config <dir> --backend-credentials <file> [--backend-config k=v] [--workspace w]` gates that configuration's `terraform {}` block: encryption and cloud refused; `backend "s3"` arguments allowlisted (assume_role/role_arn, profile, shared_*_files and endpoints refused); any other backend refused by name; a missing workspace refused, never created. It runs `tofu init -input=false` and `tofu state pull` under the backend credential file only, then deletes the state-config copy right after the pull.
- An assumption test proves the S3 backend's init only reads: run against a SeaweedFS identity allowed only Get/List, so any write fails it. A state bucket using SSE-KMS is refused by name (user, 2026-10-11).
- `docs/layer3/aws/backend-reader-policy.json` (S3 Get/List on the named bucket and key) ships, tied to the admitted-backend table by a test.
- A state whose `terraform_version` is newer than the rehearsal's tofu is refused; older is accepted.
- A state-to-read-set table (resource type → reader action + id parameter, composite ids parsed for e.g. aws_security_group_rule, aws_route_table_association, aws_iam_role_policy_attachment) derives from the reader table, with a test that every modelled state type maps. The pull runs before discovery and its ids feed the first read step.
- Unmodelled types are pruned with `tofu state rm -backup=-` and listed (goal 6 refuses through the shared classifier if the new code declares one). Only the pruned copy is kept, 0600, outside any config copy; the raw pull never persists; a test asserts a planted `secret_string` is absent from `.infrafactory/` after prune and after `end`.
- Refresh-only runs in the new-code work directory under its override, environment and local backend: clear the request log, `plan -refresh-only -detailed-exitcode -out`, list `show -json` resource_drift by address and attribute, apply that saved plan. Exit 1 is a gap; any request not answered from rows refuses. The mock is re-snapshotted after drift is absorbed, and `reset` restores the drift-absorbed state, with a test.
- Fail-closed test: after the pull, a bare `tofu apply` in the work directory dies at a dead address.
- CI: the pre-destroy state scrubbed with the read's id map. An unchanged fixture gives zero drift; a planted tag edit and a removed resource are listed exactly; the takeover plan (continue the web stack, rename one resource with `moved`, add a subnet) has no create, replace or destroy on taken-over addresses.
- ADR-0032 extended for the refresh; ADR-0042 amended; golden files; docs/operations.md.

**Out of scope:** backends other than S3 and local; a real S3 backend in the Layer 3 account; encrypted or SSE-KMS state; the registry lock warning (rehearsal-prepare-gate's).

**Constraints:** reuses rehearsal-against-the-copy's classifier and modelled-type source; the backend credential file uses prod-read-seam's reader; ADR-0032 exit-code discipline; the state copy lives outside the configuration copy.

**Your part:** none for you (a real S3-backend proof would need the SCP widened; default no). Lead: run the takeover against the real pre-destroy state kept from the step-0 hold, locally with the local backend, no cloud.

**Areas:** `internal/cli/ (rehearse_state*.go new: pull, prune, version check, refresh-only, drift)`; `internal/harness/ (state-to-read-set table)`; `internal/e2e/`; `docs/layer3/aws/backend-reader-policy.json (new)`; `docs/decisions/0032, 0042`; `docs/operations.md`
