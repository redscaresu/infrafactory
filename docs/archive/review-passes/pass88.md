# Codex review pass 88 — S160a

**Clean.** No correctness regression found in the diff.

Worth recording what it checked rather than only that it passed: it ran
`net.ParseIP` against `::ffff:127.0.0.1` and `0:0:0:0:0:ffff:7f00:1` to see
whether an IPv4-mapped IPv6 origin slips past `IsLoopback()`. Both report
`loop=true`, which is the correct answer — those *are* loopback — so the guard
admits them deliberately rather than by accident.

The slice converged in one pass, and the reason is worth naming: **the design
error was caught by the existing test suite before review ever saw it.** The
first implementation compared `Origin` against the request's own `Host`, which
looks stricter and is defeated by DNS rebinding. Three websocket tests failed
immediately, because the Vite dev server serves the UI on `:5173` and calls the
API on `:4173` — cross-origin and entirely loopback. Chasing that failure
surfaced the rebinding hole in the same rule.

A codex loop is not the only review in this project, and here it was the second
one.
