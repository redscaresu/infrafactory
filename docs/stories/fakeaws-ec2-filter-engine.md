---
kind: code
repo: fakeaws
status: ready
epic: fakeaws-data-sources
depends_on: []
touches: [handlers/ec2_filters.go, handlers/ec2_filters_test.go]
risk: high
---

# One EC2 filter evaluator in ec2_filters.go: AND across Filter.N, OR within, tags, wildcards, one sorted refusal

New `handlers/ec2_filters.go` only. It does not edit ec2.go, so it is built now, beside the other
epics' ec2.go chain. No handler calls it yet and nothing marks the request log;
fakeaws-ec2-filters-never-ignored wires both, and adds the MUST ids below, because a contract is a
wire invariant and these become one only when a handler calls them (and because fakeaws
AGENTS.md:155 must list every id, TestAgentsMDListsEveryContract, and AGENTS.md is being edited by
fakeaws-request-log-gap-markers in parallel). Write each test so never-ignored can tag it
`MUST[ec2-filter-and-across-or-within]`, `MUST[ec2-filter-wildcard-crosses-slash]` and
`MUST[ec2-filter-refusal-names-every-name-sorted]` without changing what it asserts.

**Parse.** `Filter.N.Name` / `Filter.N.Value.M` into an ordered list. Each Filter.N is its own AND
condition; the values inside one Filter.N are ORed. Today's ec2Filters merges repeated names into
one OR (handlers/ec2.go:2362-2369); the new parser must not. Walk N and M the way queryListItems
does (ec2.go:1479-1500), never with queryListValues (ec2.go:2122-2128), which stops at the first
missing index: a gap in N or in M drops nothing. `Filter.1.Value.1=a` plus `Filter.1.Value.3=b` ORs
`a` and `b`.

**Match.** A generic table maps each supported name to a row's values, so it fits both the row
tables (eniFilterValues :2354, tagFilterValues :3157) and the XML table (rtFilterValues :878). Where
a table opts in, `tag:<Key>`, `tag-key` and `tag-value` match the row's tags from tagSets
(:3049-3060). Values use AWS wildcards: `*` matches any run including `/`, `?` matches one
character, a backslash escapes. Not path.Match, whose `*` stops at `/`.

**Refuse.** Refused, all in one list: every name the table lacks (listed by name); a Filter.N with
no Name, or with no Value (listed as `Filter.<N>`, since there may be no name to print); and any
non-Filter param the caller hands the writer as unread (listed as `<Param>.N` or the scalar name).
The list is deduped and sorted bytewise. The writer answers 501, code `NotImplemented`, in the EC2
XML error shape (awsproto.WriteServiceError ShapeEC2Query, awsproto/error.go:83-107). Its message is
`<Action>: not modelled by fakeaws: <name>, <name>, ...`. The phrase `not modelled by fakeaws` is
fixed: infrafactory-filter-refusal-routes-to-mock-gaps matches it to route a refusal to
docs/mock-gaps.md instead of learning a pitfall. NotImplemented is fakeaws's own refusal, not a
claim about AWS: AWS answers an unknown filter name with 400 InvalidParameterValue, which would
blame valid HCL.

**Done when:**
- ec2_filters_test.go: two Filter.N with the same name are ANDed (a row holding only one of the
  values fails); two values in one Filter.N are ORed; Filter.1 and Filter.3 with no Filter.2 both
  apply; Filter.1.Value.1=a plus Filter.1.Value.3=b matches a row holding only `b`.
- Wildcards: `ubuntu/images/*` matches `ubuntu/images/hvm-ssd/ubuntu-jammy-22.04`;
  `amzn2-ami-hvm-2.?` matches `amzn2-ami-hvm-2.0` and not `amzn2-ami-hvm-2.10`; `a\*` matches only
  the literal `a*`.
- `tag:Name=web` matches only a row tagged Name=web; tag-key and tag-value match on any of the row's
  tags; a table without tags refuses `tag:Name` and names it.
- Refusal: names `zz`, `aa`, `zz`, a Filter.4 with no Value and a Filter.5 with Value.1 and no Name
  answer 501 NotImplemented with message `<Action>: not modelled by fakeaws: Filter.4, Filter.5, aa, zz`,
  byte-identical across 20 calls.
- `go vet ./...` and `go test ./handlers/` pass, and `git diff --stat main` lists only
  ec2_filters.go and ec2_filters_test.go.
- Each new test is shown failing by a cp-backup break of ec2_filters.go, recorded in the PR:
  repeated names merged into one OR; a Value.M walk that stops at the first gap; a nameless
  Filter.N dropped; a matcher that stops at `/`; an unsorted refusal.
