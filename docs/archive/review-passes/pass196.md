# Review pass 196 — S189 private-NIC workaround removal

`codex exec review --base main`, 2026-09-20.

> The changes compile and the relevant Go test suite passes. I did not find an actionable
> correctness regression in the provider pin bump or private-NIC workaround removal.

Clean on the first pass, which is unusual here and worth a note: this slice **deletes**
rather than adds, and the deletion was licensed by three real-cloud canaries rather than
by reasoning. There was less new logic for a reviewer to find fault in because there is
almost no new logic.

The one judgement not visible in the diff: the Layer 2 mock detach went too. Keeping it
would have left the mock exercising a teardown path the provider no longer takes, which
is the mock leading its consumer instead of following it.
