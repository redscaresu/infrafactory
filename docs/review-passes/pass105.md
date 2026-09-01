# Codex review pass 105 — S161

One finding, accepted.

## [P2] IPv6 addresses built an invalid URL

`http://2001:db8::1:8080` is not a URL — the colons are ambiguous and a browser
will not open it.

The reviewer checked the reachability rather than assuming it: the probe path
uses Go's `net.JoinHostPort`, which brackets IPv6, and `pickHost` accepts any
address `net.ParseIP` understands. So an IPv6 deployment can be recorded, probed
successfully, and then rendered here as a link that goes nowhere.

Same shape as pass 102's dropped port, and the reason both matter is the same:
**the link is a claim about what the system measured.** `live observe` probed
`[2001:db8::1]:8080`; a page linking anywhere else is showing something other
than what was checked.

An address carrying a single colon is left alone — that is a host that already
has a port, and guessing about it would be worse than not touching it.

## Five findings, one family

Every finding in this slice has been the page telling a small untruth about what
the system knows: a blank cell for an unchecked version, a year-one date for a
moment that never happened, "nothing is deployed" during a failed read, "no live
deployments" beside unreadable records, and now a link to somewhere nothing was
probed.

That is the right family for this page to be wrong in — it is the page's entire
subject — and it is worth noticing that the interesting failures were never in
the parts that render *problems*. They were all in how it renders **the absence
of information**.
