---
kind: code
repo: fakeaws
status: ready
epic: fakeaws-loader
depends_on: []
touches: [handlers/load_diff.go, handlers/load_diff_test.go, handlers/regression_audit_test.go]
risk: high
---

# A generic XML and JSON 1.1 tree diff names, in both directions, every path where a recorded body and fakeaws's rendering differ

New handlers/load_diff.go; nothing calls it yet. fakeaws-load-exchange-rules diffs each loaded
item with it, and fakeaws-load-read-back each re-sent exchange, both using this story's API
unchanged.

**Parse.** XML through encoding/xml tokens, never struct decoders, which drop unknown elements
silently. JSON 1.1 through a generic walk decoded with UseNumber. Both produce one exported tree
type, with two helpers: Subtree(path, index), which returns one item's subtree, and
MergePages(trees...), which returns the union of a paged list's items, so the loader diffs a
single item and read-back compares the union of pages.

**Compare.** `Diff(key, recorded, rendered)` returns two sorted path lists: paths only in the
recording (or with another leaf value), and paths only in the rendering. "Equal" means both are
empty. key is `{service, action}`, the key fakeaws-load-route registers loaders by, and Diff is
the only place ignore entries are applied.
- Every repeated `item` (EC2) or `member` (IAM query) child, and every JSON array element, is a
  multiset member, whatever its parent is called (vpcSet, ipPermissions, ipRanges,
  availabilityZoneInfo, blockDeviceMapping, productCodes). Order is never a difference.
- An unmatched item is paired with the unmatched item on the other side that differs on the
  fewest paths (ties by index), and the pair's differing paths are reported. An item with no pair
  is reported whole. A path names the item's index in its own tree: `vpcSet/item[2]/isDefault`.
- Whitespace-only text between elements is ignored. Leaf text is compared byte-exact, so a
  trailing newline is a difference.
- An element with no children and no text is treated as absent: a recorded `<groupSet/>` equals a
  rendering that omits it (the SDK decodes both to an empty list).
- JSON numbers are compared by value: `1.7E9` equals `1700000000`.
- Always ignored: XML comments, requestId/RequestId, ResponseMetadata, and the paging fields
  nextToken, NextToken, IsTruncated and Marker.

**Ignore list.** An entry is `{key, path, cite}`. It suppresses only its own key's path, in both
directions. cite names a terraform-provider-aws v5.100.0 source file in which the field is never
read. Each loader file registers its own entries; one test here walks every registered entry, so
each loader story's entries are checked when it lands. A path never carries a value, since a body
may hold a real account's data.

**Audit.** handlers/regression_audit_test.go groups handler files by their prefix before `_`
(:240-257), and `load` is not a service, so knownNonServiceFiles (:232-238) gains
`load_diff.go`. fakeaws-load-route adds the rule for load.go and the per-type `load_*.go` files.

**Done when:**
- An unknown nested `<vpcSet><item><fooBar><x>1</x></fooBar></item></vpcSet>` is reported as
  recorded-only `vpcSet/item[0]/fooBar/x`; an unknown JSON key under Parameter as `Parameter/Foo`.
- A rendering that adds `<dhcpOptionsId>` the recording lacks is reported rendered-only.
- Swapped vpcSet items, swapped ipPermissions/item/ipRanges/item and swapped
  availabilityZoneInfo/item report nothing. isDefault true against false reports
  `vpcSet/item[1]/isDefault` when it is the second item. A set one item short reports that item by
  its index, and no path of the matching items.
- An indented recording against a compact rendering reports nothing; a leaf differing only by a
  trailing newline is reported; a recorded empty `<tagSet/>` against an absent one reports
  nothing; JSON `1.7E9` against `1700000000` reports nothing.
- nextToken, IsTruncated and Marker present on one side only report nothing.
- An ignore entry suppresses only its own key's path. An entry whose cite is empty, or not a
  terraform-provider-aws v5.100.0 path, fails the ignore-list test.
- Two bodies differing only in a value `SECRET-MARKER`: no returned string contains SECRET-MARKER.
- TestRegressionSeedAuditManifestMatchesHandlers passes, and `git diff --stat main` lists only the
  three touched files.
