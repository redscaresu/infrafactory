---
kind: code
status: blocked
blocked_by: [rehearsal-walls-adr]
epic: rehearsal-walls
depends_on: [rehearsal-walls-adr]
touches: [internal/cli/rehearse_gate.go, internal/cli/rehearse_gate_aws.go, internal/cli/rehearse_gate_test.go]
risk: high
---

# The gate admits infrafactory's own override files only byte-identical, and reports the root's aws aliases

rehearsalGateInput gains `ours map[string][]byte` (lowercased name → exact bytes). In the entry walk (rehearsalEntryProblem refuses override.tf* and *_override.tf* by name today, rehearse_gate.go:220-240), an override-pattern file whose name is in `ours` is admitted only when it is in the root dir and its bytes equal ours[name]. A difference is refused, naming the file. With `ours` nil, today's behaviour is byte-identical: every override is refused. A file in ours that is missing from the root is refused too, since an override that is not there is a missing wall. The admitted file is never parsed into the gate's checks.

The gate also returns the root's `provider "aws"` aliases (unaliased = ""), read from what rehearse_gate_aws.go already walks for goal 11. This lets prepare write one override block per alias. Prepare does not pass `ours` (it writes both files after the gates). The gate input is already per root, so rehearsalRegate(rehearsal, root) builds one `ours` per root.

**Done when:**
- Table test: ours={infrafactory_override.tf: B}. A root file with bytes B passes. B plus one trailing byte is refused, naming the file. The same file in a child module dir is refused. A missing file is refused. zz_override.tf is still refused. Removing the bytes comparison turns the B+1 case red (cp-backup).
- With ours nil, `infrafactory_override.tf` is refused as today, and every existing rehearse_gate test passes unchanged.
- A fixture with `provider "aws" {}`, `provider "aws" { alias = "west" }` and `provider "aws" { alias = "dr" }` returns aliases ["", "dr", "west"].
- Checks before every stage in docs/epics/rehearsal-walls.md hold.

**From review (binding; overrides the scope above where they differ):**
- None of the reviews changed this story.
