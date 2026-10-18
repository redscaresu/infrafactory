---
kind: chore
status: ready
depends_on: []
touches: [scripts/m88_sweep.sh, scripts/m93_resweep.sh, scripts/m94_aws_proof.sh, scripts/m95_multipass.sh]
---

# Delete the old milestone sweep scripts

Found in review of #617. scripts/m88_sweep.sh, m93_resweep.sh, m94_aws_proof.sh and
m95_multipass.sh still time scenarios out with a perl alarm and check rc 124. #617 showed the
alarm exits 142 and cannot stop a Go binary. Nothing live calls them: they appear only in
CHANGELOG.md and BACKLOG_ARCHIVE.md. Delete them rather than fix them. scenario_change_gate.sh
is the maintained runner.

**Done when:**
- The four scripts are gone, and a grep in the PR body shows no reference outside CHANGELOG.md, BACKLOG_ARCHIVE.md and docs/archive/.
- `make doc-hygiene` and CI pass.
