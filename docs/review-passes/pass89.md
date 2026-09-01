# Codex review pass 89 — S160b

**Clean.** No correctness issues in the changed code. It specifically confirmed
the removal is consistent across all three layers — API request type, CLI
starter, UI normalisation — and that the start-time decision is re-applied during
the per-run config reload.

That last point is the one worth having checked by someone other than the author,
because it is the half of this slice with a live failure mode rather than a
theoretical one: the per-run config is re-read from disk on every run, so a file
carrying `sandbox_deploy.enabled: true` would otherwise have re-enabled spending
on a server nobody authorised.

## Nothing declined this pass.
