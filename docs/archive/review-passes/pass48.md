# Codex review pass 48 — S155a

One finding, accepted. **A regression pass 47 introduced.**

## [P2] The response drain lost its bound

Reading the body head meant editing the `defer` beside it, and in doing so
`io.Copy(io.Discard, io.LimitReader(resp.Body, maxProbeBodyBytes))` became
`io.Copy(io.Discard, resp.Body)`.

The drain exists so the connection can be reused. Unbounded, a streaming or very
large response spends the probe's entire timeout budget being thrown away — and
`live observe` probes every live deployment in turn, so one slow body delays all
the ones behind it. The client timeout caps it, which is why it would have shown
up as slowness rather than a hang: the worst kind to find later.

Restored, with the reason written down so the next edit to that block does not
quietly drop it again.

## The lesson, which is not about draining

Pass 47 fixed two real findings and broke a third thing on the way, in a line it
was not there to change. **An incidental edit inside a fix is still an edit** —
and under a one-clean-pass rule it is the incidental ones that will survive,
because attention is on the finding.
