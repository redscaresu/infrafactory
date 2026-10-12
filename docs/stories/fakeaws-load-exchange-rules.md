---
kind: code
repo: fakeaws
status: ready
epic: fakeaws-loader
depends_on: [fakeaws-load-route]
touches: [handlers/load.go, handlers/load_test.go, handlers/export_test.go, repository/load.go, repository/load_test.go]
risk: high
---

# /mock/load decides what each exchange means: NotFound, other errors, page chains, repeats, references, and the rendering diff

Built on fakeaws-load-route's Loader interface, with its test-only loaders; no type loader yet.
Every rule below is the route's, so the type lanes inherit it.

**Status.** A non-2xx whose code is in the loader's NotFound list loads nothing and is counted
under absent. Any other non-2xx is refused, naming action and status.

**Page chains.** Pages are separate exchanges, linked only by the token param. Each loader
declares its token fields: EC2 answers `nextToken` and takes `NextToken`; IAM answers
`IsTruncated` and `Marker` and takes `Marker`; SSM JSON answers and takes `NextToken`. Exchanges
of one service and action whose params are equal apart from the request token are one entry. A
chain starts at the page with no request token, and each next page carries the token the previous
one answered.
- A page whose answered token no later page of its entry carries is refused as truncated (IAM:
  `IsTruncated` true).
- A page carrying a token no earlier page of its entry answered is refused as an orphan page.
- Every page of a complete chain loads.

**Repeats.** An id repeated within one chain is refused, naming it. Across different entries, the
same `(type, id)` loads once when its recorded subtrees are equal by load_diff.go. Otherwise it is
refused, naming the id and both actions with their exchange indexes. The step-0 read relies on
this: its tagged SG comes back from the read set's DescribeSecurityGroups tag-key entry and again
from the follow-up DescribeSecurityGroups vpc-id (infrafactory
internal/e2e/testdata/aws-step0/read-set.json:14-18, internal/harness/aws_reader_ec2.go:131).

**References.** After every loader has run, each reference a loader returned must name a loaded
row. An unresolved one is refused, naming row and reference, unless the loader flagged it as one
the reader never reads; those are answered under unresolved, never silently.

**Rendering diff.** For each loaded item, `Diff({service, action}, recorded subtree,
Render(tx, ids))`, inside the transaction. A recorded-only path is refused, naming type and path:
the model cannot hold that field. A rendered-only path is refused too, naming type and path:
fakeaws would answer a field the body never held, which the epic calls minting. Ignore entries
apply through Diff only.

**Done when:**
- Render runs under the same recover as load (handlers/load.go run.exchange, fakeaws#78): a render that panics is a value-free *loadAbort and its panic value never reaches the log; a test render panics with a marker value and the captured log lacks it.
- With test-only loaders: a NotFound exchange is counted absent; a 500 exchange is refused naming
  the action and 500.
- A NextToken page with no next page is refused as truncated; an IAM page with IsTruncated true
  and no next page likewise; a page carrying a token nothing answered is refused as an orphan; a
  complete three-page chain loads all three.
- An id in two pages of one chain is refused naming it.
- The same SG item in a DescribeSecurityGroups tag-key exchange and a vpc-id exchange loads once
  and is counted once. With one field changed in the second, the load is refused naming the id,
  both actions and both exchange indexes.
- An unflagged dangling reference is refused naming row and reference; a flagged one is answered
  under unresolved.
- A test-only loader that drops one recorded field on write is refused naming its type and that
  path. One whose Render adds a field the recording lacks is refused naming that path.
- After every refusal above, /mock/state and identity are byte-identical to before.
