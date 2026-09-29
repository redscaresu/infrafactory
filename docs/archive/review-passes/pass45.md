# Codex review pass 45 — S154 `live observe`

`codex exec review --base main` on `s154-live-observe` at `c65abac`.

One finding, accepted. **It is a defect pass 44's neighbouring change
introduced** — the workflow fix shipped in this same PR.

## [P1] The marker was read as proof a project still exists

`.github/workflows/layer3-gate.yml`. The reap step checked `-f "$marker"`
**before anything else** and reaped when it found one.

Nothing removes the marker after a successful delete, so a green run leaves one
behind too. Verified on disk rather than argued: after the successful
`block-paris` canary the workdir held both

```
.infrafactory-run-project      <- still there
terraform-live.tfstate         <- present, "resources": []
```

So every green gate run would have paid for a redundant real-API reap, and gone
red if that reap hit a transient error. A cleanup step that fails after a
successful run is worse than the gap it was closing.

## The distinction that fixes it

**The marker proves a project was created. It never proves one survives.**
Whether it survives is the API's question, and `reap` asks it — a project
already gone answers 404, which reap treats as success.

So the check moved *inside* the no-state branch, where it adds the information
that branch was missing and nothing else. A green run leaves an **empty** state
file, not a missing one, so it never reaches the marker at all.

## Re-reading the fix against its own class

The class is *a signal that proves X being read as proving Y*. Every other
marker consumer was checked:

| site | reads the marker as | correct |
|---|---|---|
| `CaptureSweepTarget` | which project is ours (identity) | yes |
| `reap` | which project is ours (identity) | yes |
| `AssertRunProjectDeletable` | identity, paired with API provenance for existence | yes — existence is explicitly the API's half |
| `reclaimable()` | whether teardown owns the record | yes — a released record returns false before the marker is consulted |

The workflow was the only place reading it as existence, and it is the only
place with no type system to object.

## Nothing declined this pass.
