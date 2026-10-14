---
kind: code
status: blocked
blocked_by: [rehearsal-walls-prepare-wiring]
epic: rehearsal-walls
depends_on: [rehearsal-walls-prepare-wiring]
touches: [internal/cli/rehearse_walls_regate.go, internal/cli/rehearse_walls_regate_test.go]
risk: high
---

# rehearsalOpen and rehearsalRegate: find a rehearsal safely, then the full gate over each prepared root, admitting infrafactory's two overrides by exact bytes

New file `internal/cli/rehearse_walls_regate.go`.

`rehearsalOpen(repo, id string)` is the one way a later step (end, passthrough, against-the-copy) finds a rehearsal. The id must be a single path element, and not `.` or `..`. `<repo>/.infrafactory/rehearsals/<id>` is checked with Lstat and must be a real directory, not a symlink. It never calls rehearsalsDir and never creates anything: a missing `.infrafactory/rehearsals` or `<id>` is "no such rehearsal". It then reads the record with rehearse_walls.go's reader, and dials rehearsalDeadProxy again, refusing if anything accepts (prepare dial-checks it only once, rehearse_prepare_init.go:197-200). It returns distinct errors (errors.Is-able) for a bad id, a missing rehearsal, an unreadable or unparseable record, and a dead proxy that accepts, so `end` can act on each.

`rehearsalOurOverrides(rehearsal, root) (map[string][]byte, error)` rebuilds both overrides' bytes for one root from the record: localBackendOverride("") and rehearsalProviderOverride(fakeaws URL from the port, region, that root's endpoint_keys and aws_aliases). It never reads the files back.

`rehearsalRegate(rehearsal, root) error` rebuilds the gate input for that root from <id>: the root and its work dir (the boundary) from the record's roots entry, region, vars.tfvars when present, the aws version from the copy's lock, and modules from rehearsalModulesGate over <id>/data. It then runs rehearsalGate with ours. A root not in the record is refused. Passthrough calls rehearsalOpen and then rehearsalRegate before every tofu step; walls adds no caller but `end`'s use of rehearsalOpen.

**Done when:**
- After a fake-runner prepare, rehearsalOpen returns its record and rehearsalRegate passes for its one root.
- Flipping one byte in infrafactory_aws_override.tf, deleting it, or editing one endpoints URL to a public host is each refused, naming the file. So are a planted zz_override.tf and a planted `.tofu` file.
- Changing the record's port without changing the file is refused. This proves the bytes come from the record, not the file. Making rehearsalOurOverrides read the file instead turns it red (cp-backup).
- A record with two roots: a planted break in the second root is refused naming that root, while the first passes, and a root name not in the record is refused.
- rehearsalOpen refuses `../x`, `a/b`, `.`, `..` and a symlinked `<id>` as a bad id. On a repo with no `.infrafactory` it reports no such rehearsal, and afterwards `.infrafactory` still does not exist. An unparseable record is refused as such. With rehearsalDeadProxy moved to a listener the test holds, it refuses naming the address; removing the re-dial turns that red.
- Checks before every stage in docs/epics/rehearsal-walls.md hold.

**From review (binding; overrides the scope above where they differ):**
- rehearsalOpen(repo, id) lives here: validate the id as a single path element via Lstat with no symlink, never mkdir, read the record, re-dial the dead proxy and refuse if it accepts. `end` and regate both call it, so passthrough copies no path-safety code (critic).
- The record and regate are per root: rehearsalRegate(rehearsal, root), and the override bytes per root. Today there is one root; the list shape is what later epics extend (critic).
