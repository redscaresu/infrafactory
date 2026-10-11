---
kind: docs
status: ready
epic: prod-read-seam
depends_on: []
touches: [docs/decisions/0041-account-read-contract-and-recorded-read-format.md, docs/decisions/README.md, docs/stories/fakeaws-request-log-read-set.md]
---

# ADR-0041: the account-read contract, the read-set and recorded-read formats

First and alone. Three fakeaws items wait on it: fakeaws-request-log-read-set, the fakeaws-loader epic (line 23) and the fakeaws-rehearsal-instance epic (line 12).

**Read-set entry.** `{service, protocol, action, params}`, with protocol one of `query`, `json` or `rest`.
- **query:** action is the form body's Action. params is a JSON object of every other form field except Version, keys exactly as on the wire (Filter.1.Name, GroupName.1, Owner.1), string values. A repeated key is refused.
- **json:** action is the operation after the dot in X-Amz-Target. params is the request body's JSON object, verbatim.
- **rest:** action is `"<METHOD> <route pattern>"`, and params is `{path: the raw path, query: the raw query}`. So two S3 subresources on one bucket are two entries. Rest is outside the reader table, so the reader refuses every rest entry by name.

**Recorded read.** `{account, region, timestamp, duration_ms, calls, exchanges}`. Each exchange is an entry plus `status` and `body`, where body is the raw response bytes as a JSON string. calls counts every request sent, retries, identity and preflight included. The account and region are what fakeaws /mock/load stores (fakeaws-loader epic, line 10).

**Where params live.** Params and bodies never reach fakeaws /mock/state, whose audit stays `{service, protocol, action, status, class}`, nor its db or snapshot.

**Files.** Under `.infrafactory/`, directories 0700 and files 0600. The ADR also records the `rehearse read` flags (epic line 11), so the CLI PR carries `ADR: none`.

**Board.** In docs/stories/fakeaws-request-log-read-set.md, replace `prod-read-seam` with `prod-read-seam-adr-0041` in blocked_by and depends_on, and rewrite its body line that says to re-point once this epic is planned so it names this story.

**Done when:**
- docs/decisions/0041-*.md exists with status accepted and is indexed in docs/decisions/README.md by `make adr-index`; `make doc-hygiene` passes.
- It holds one fenced JSON example per protocol: EC2 DescribeVpcs with Filter.1.Name=tag-key, SSM GetParameter, and S3 GetBucketTagging and GetBucketPolicy on one bucket as two entries that differ only in params.query. It also holds one fenced recorded-read example. prod-read-seam-wall's test parses each strictly, and fails when it finds no block.
- Each of these has its own sentence: rest entries are refused by name; params and bodies never reach fakeaws /mock/state, its db or its snapshot; the recorded read carries the account and region that /mock/load stores.
- fakeaws-request-log-read-set.md names `prod-read-seam-adr-0041` in blocked_by, depends_on and its body, and no reference to the bare epic slug `prod-read-seam` remains in it.
- The diff touches only docs/decisions/ and that one story file.
