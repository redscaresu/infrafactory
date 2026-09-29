# Codex review pass 117 — S162a

One finding, accepted, and it is the **dangerous** direction of pass 112's
finding.

## [P2] A compute-only scenario was told it was not internet-facing

The estimator bills a `public IPv4 address` for every compute instance. The
`internet_facing` flag checked only the load balancer. So a scenario with an
instance and no balancer was **charged for a public address and told it was not
reachable from the internet.**

Pass 112 was the same disagreement in the safe direction — over-warning on private
load balancers. This is the same disagreement understating exposure, at the
confirmation step, immediately before someone agrees.

## Two answers to one question

Pass 112's fix was `LoadBalancer.Public()`, used by both the estimator and the
flag "because the two callers must agree". That reasoning was right and the fix
was too narrow: it made them agree **about load balancers**, and left the compute
path with its own hand-written condition.

`internet_facing` is now derived from the component list —
`CostEstimate.InternetFacing()` is true when the estimate contains a public
address. The bill and the warning cannot disagree because there is only one of
them, and anything that adds a public address later raises the warning without
anyone remembering to.

## Two tests failed, and they were wrong

`TestPreviewSaysWhetherItWillBeReachableFromTheInternet` and
`TestPreviewDoesNotCallAPrivateLoadBalancerInternetFacing` both removed the
networking block or made the balancer private, **left `Compute` in place**, and
asserted `false`. They encoded the defect: an instance gets its own public
address, so `false` was never the right answer for those scenarios.

Third time this session a red test has been the test's fault rather than the
code's. The reflex is to change the code, and the check is cheap: ask what the
system should do before asking what the assertion says.
