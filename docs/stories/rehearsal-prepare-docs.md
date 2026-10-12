---
kind: docs
status: ready
epic: rehearsal-prepare-gate
depends_on: [rehearsal-prepare-second-init]
touches: [docs/operations.md, docs/epics/rehearsal-prepare-gate.md, docs/hld/2026-09-30-import-production-state-into-the-mocks.md]
---

# Runbook for `rehearse prepare`; the epic closes

Add a `## Rehearsal` section to docs/operations.md (or, if prod-read-seam-rehearse-read-cli created it first, extend it), with `### Prepare` covering: the command and every flag; the layout and modes (0700 executables included); and what is refused and why, in user terms. That includes TF_ENCRYPTION, `.tofu`, `.tf.json` and the other refused suffixes; every non-local module, registry modules included, and why (the download runs git against github.com, where the system git config can carry a keychain credential helper); `terraform.d`; a provider "aws" block without a matching region; the missing lock; the registry.terraform.io warning; and the non-5.100.0 report line.

Delete docs/epics/rehearsal-prepare-gate.md. Change the HLD's `## Epics` line to `rehearsal-prepare-gate — done (#PRs)` and remove links to the deleted file.

**Done when:**
- Every flag that `infrafactory rehearse prepare --help` prints appears in the section; a loop in the PR checks this.
- `grep -rn 'epics/rehearsal-prepare-gate' docs --include='*.md' | grep -v archive` is empty, and `make doc-hygiene` passes.
