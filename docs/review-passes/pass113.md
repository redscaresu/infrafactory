# Codex review pass 113 — S162a

Two findings, accepted. The second is the important one, and not because of the
bug.

## [P2] Scaleway shape names on other clouds

`EstimateCost` defaulted compute to `DEV1-S instance` and the balancer to
`LB-S load balancer` regardless of cloud, marking them unpriced for GCP and AWS
but still **naming** them.

Unpriced was the right call and the name was not: telling a GCP user their deploy
will create a "DEV1-S instance" names a resource that will not exist, which is
worse than being vague when the endpoint's entire job is saying what deploy would
do. Non-Scaleway scenarios now get the shape — "compute", "load balancer" — and
still say what will be created.

## [P2] The year-one expiry, again

An undeployable preview never sets `ExpiresAt`, and `omitempty` does not omit a
zero `time.Time`. So every greyed-out scenario would have reported expiring on
`0001-01-01`.

**This is the class S159a closed, four slices ago, with a test written
specifically to stop it recurring.** It recurred anyway, and the reason is worth
stating precisely: that test asserts over the **deployments** payload. This is a
different payload, in a different handler, added later. A class test that names
one instance of the class is still a snapshot — it just looks less like one.

The general rule this project keeps rediscovering is that **an optional field in
a view needs a type that can be absent**, and Go's `omitempty` does not provide
one for structs. Every response type added to this API inherits that, and the
only real defence is checking it when the type is written rather than when the
review finds it.

`ExpiresAt` is a `*time.Time` now, and the preview has its own year-one assertion
covering the undeployable cases — where it bites, because those are the ones that
never set it.
