---
kind: code
repo: fakeaws
status: ready
epic: fakeaws-loader
depends_on: [fakeaws-load-tree-diff]
touches: [handlers/load.go, handlers/load_test.go, handlers/export_test.go, handlers/admin.go, handlers/handlers.go, handlers/regression_audit_test.go, repository/load.go, repository/load_test.go, cmd/fakeaws/main.go, cmd/fakeaws/main_test.go, README.md, SECURITY.md]
risk: high
---

# POST /mock/load, off by default, replaces every row in one transaction through a pinned Loader interface

The route, the replace transaction and the Loader interface. What each exchange means (status,
pages, repeats, references, the rendering diff) is fakeaws-load-exchange-rules, built on the
interface fixed here.

**Flag.** New `-load-max-bytes` (an Options field, cmd/fakeaws/main.go:52-62), default 0, which
leaves the route unregistered. The startup check lives in optionsFromFlags: non-zero with neither
`-admin-token` nor `FAKEAWS_ADMIN_TOKEN` (main.go:76-78) exits non-zero, naming `-load-max-bytes`
and `-admin-token/FAKEAWS_ADMIN_TOKEN`. (-record-requests' refusal in NewApplication,
handlers.go:85-96, names no flag, so it is not the model.)

**Route.** A new mr.Post in registerAdminRoutes (handlers/admin.go:55-71), behind guardAdmin,
reading through http.MaxBytesReader at that limit, not decodeAdminBody's 64 KiB (:155, :276-286).
A strict decoder of the ADR-0041 envelope `{account, region, timestamp, duration_ms, calls,
exchanges}`: an unknown key is refused naming it; account is 12 digits; region is set.

**Transaction.** identityMu is held exclusively for the whole load (handlers.go:248-255). One
transaction carries every read and write; the db has one connection (repository.go:104-107).
Inside it:
1. Delete every resetTables row, children first, with FKs on. Never call Repository.Reset, which
   drops the snapshot file (:279-325).
2. Write identity `{account, region, loaded=1}`.
3. Run the registered loaders in registry order, not exchange order.
4. Commit. Any refusal rolls back.
Region-scoped rows get the envelope's region.

**The Loader interface** (repository/load.go and handlers/load.go), fixed here for the three
parallel type lanes:
- repository/load.go holds a tx-scoped handle, LoadTx. A loader reads and writes only through it.
  It never touches r.db or calls an existing repository method, and never calls
  app.AccountID() (admin.go:290-293): with one connection, either blocks forever inside the tx.
  The account and region come from the envelope.
- A loader registers from init() in its own handlers/load_<service>_*.go file, keyed
  `{service, action}`, declaring: its NotFound codes; its request token param and response token
  field; Load(tx, envelope, exchange), which writes rows and returns, for each item, its
  `(type, id)`, its recorded subtree's path, and its references `(type, id)`, flagging those the
  reader never reads; and Render(tx, ids), which reads the stored rows back through the tx and
  marshals them with the same encoder the Describe/Get handler answers with. Never a rendering
  built from the loader's own decoded struct: that would compare the body with itself.
- An exchange whose `{service, action}` has no loader is refused, naming service and action.
- Answer: 200 `{loaded, absent, unresolved}`, each per type, or 400 listing every refusal.

Only a test-only loader (export_test.go) is registered here.

**Audit.** handlers/regression_audit_test.go's knownNonServiceFiles gains `load.go`, and the
prefix check skips any `load_*.go` (the per-type loader files the next stories add), so the
`load` prefix never needs a LandedServices entry.

**Docs.** README.md documents the route and the flag. SECURITY.md (:31-36) says a load writes a
real account's read into the -db file and its `.snapshot`, both 0600, and that `:memory:` is
preferred for a loaded read.

**Done when:**
- Without `-load-max-bytes`, POST /mock/load answers 404 like any unknown /mock path. With it,
  and with getenv returning nothing for FAKEAWS_ADMIN_TOKEN and no `-admin-token`, fakeaws exits
  non-zero naming both.
- With both: no token answers 401, a non-loopback Origin 403, a body one byte over the limit 413;
  none changes anything.
- A load with no exchanges, on a db holding a VPC, a role and an SSM parameter, empties every
  resetTables table but identity, which holds the envelope's account and region with loaded=1. The
  `.snapshot` file survives, and /mock/restore brings the old rows back.
- An ec2 DescribeFoo exchange answers 400 naming `ec2 DescribeFoo`; an envelope with an unknown
  key answers 400 naming it; after each, /mock/state and identity are byte-identical to before.
- A second load replaces the first: no row of the first remains.
- A DescribeVpcs sent while a load is held mid-transaction by a test hook answers only after
  commit, with the loaded rows.
- LoadTx exposes no `*sql.DB`, and `git grep -n -e 'AccountID()' -e 'r\.db' handlers/load*.go
  repository/load*.go` prints nothing but LoadTx's own constructor.
- TestRegressionSeedAuditManifestMatchesHandlers passes with handlers/load.go and a scratch
  handlers/load_x.go present.
- SECURITY.md names /mock/load, the 0600 db and snapshot, and `:memory:`.
