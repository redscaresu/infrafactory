# Codex review pass 91 — S156d

Two findings, both accepted. The first is the more serious, because **the test
was written in a way that hid it.**

## [P2] `AddressResource` is the probe target, not the fault site

A run-loop failure names a resource address. A real `live observe` failure says
`health path http://51.15.0.1/healthz returned HTTP 503` and names nothing at all.

I passed the deployment's `AddressResource` to `ExtractFixPitfall` as the
attribution hint. It is where the probe **pointed** — a load balancer IP — not
where the fault was, which is typically a backend block. So the diff was narrowed
to the wrong resource type and every upgrade worth learning from was skipped as
"no attributable change".

**My test passed only because it seeded a synthetic detail containing
`scaleway_lb_backend.app`.** A real deployment would have learned nothing, and the
suite would have gone on reporting that it worked. The test now uses the detail a
real `ServiceProbe` produces.

Attribution comes from the diff instead, under the rule the extractor already
applies to ambiguity: use the change only when **exactly one** resource differs.
Several is an ordinary upgrade that carries no attributable remedy — the diff
cannot say which change cleared the failure.

## [P2] A failure that had already recovered was credited to the upgrade

`lastAdverse` searched the entire pre-upgrade history, so a service that broke,
recovered on its own, and was later upgraded produced a "repair" — attaching a
remedy to a failure the new configuration never addressed.

The rule is now the mirror of the one on the other side: `after` must be entirely
healthy, so `before` must **end** unhealthy. Anything looser lets an unrelated
diff inherit somebody else's failure.

## Reverted in the same pass, deliberately

While chasing the first finding I had taught `ExtractFixPitfall` to accept a bare
resource type as a hint, fixing a real silent no-op — the argument had been
falling through `splitAddress` and returning nothing.

With attribution moved to the diff it was unused, and it changes the behaviour of
the **run loop** (`run_command.go` passes `cleared.Resource`) as a side effect of
a live-learning slice. No evidence says any caller passes a bare type today, so
it is speculative as well as out of scope. Reverted. If it is worth fixing it is
worth its own slice with its own evidence.
