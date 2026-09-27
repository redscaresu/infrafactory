---
status: blocked
blocked_by: operator decision on local permission rules
---

# Nested claude hangs in self_review to the 5-minute timeout

`plan_architecture` and `generate_hcl` complete; `self_review`, the only phase that reads
files, is killed at 300s. Its stderr warns about three allow rules in the local
`.claude/settings.local.json` that match nothing. Suspected, not proven, to be the cause.
Correcting them grants access they do not grant today, so the operator decides.

**Done when:** a nested generation completes `self_review`, or the real cause is found.
