---
kind: docs
status: ready
depends_on: []
touches: [docs/operations.md]
risk: high
---

# The Layer 3 runbook's key-names check prints only allowed names

Found in #473's review. docs/operations.md's Layer 3 section checks `layer3-aws.env` with
`cut -d= -f1`, which prints any line that has no `=` in full: a wrapped or pasted secret on its own
line is echoed to the terminal. #473 fixed the same check for the reader key file with an allowlist;
the Layer 3 section still has the old form.

Replace it with the reader section's allowlist form: print only lines whose name is exactly
AWS_ACCESS_KEY_ID or AWS_SECRET_ACCESS_KEY, and count the rest without printing them.

**Done when:**
- docs/operations.md has no `cut -d= -f1` (or similar) applied to a credential file.
- The Layer 3 check prints only the two allowed key names and a count of other lines, never a line's text.
- Run against a scratch file holding a line with no `=` (a fake secret), the command's output does not contain that line, shown in the PR.
