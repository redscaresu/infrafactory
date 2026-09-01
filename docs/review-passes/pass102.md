# Codex review pass 102 — S161

One finding, accepted.

## [P2] The address link dropped the port

`live observe` probes `address:port`, using the port snapshotted on the record at
deploy time (S154). The estate page linked to `http://<address>` and discarded it.

A deployment served on 8080 would therefore show an address, and clicking it
would open somewhere the system never checked — most likely nothing at all. The
page would look like it was reporting a broken service when nothing was wrong
with it, which is the same shape of falsehood as everything else this slice is
about: **the UI showing something other than what the system actually measured.**

Port 80 is still omitted, because a URL carrying it is the same URL. Zero — a
record predating the field, or a scenario declaring no port — falls back to the
bare address rather than inventing one.

## Worth noting about the slice

The interesting thing is that this is a **fidelity** bug, not a rendering one.
Every text and visual test passed: the page said the right words about health,
version and observation, and linked to the wrong place. The page's stated job is
to show what the system knows, and a link is a claim about that too.
