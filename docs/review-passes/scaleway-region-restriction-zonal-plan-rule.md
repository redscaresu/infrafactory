# Review — Scaleway region_restriction reads a resource's declared zone and region at plan time

Named for the story rather than numbered: pass numbers are a shared counter, and
parallel agents picking the next one collide.

One finding declined; the other was accepted and is in the PR body.

## Declined: [P2] "Only deny provider defaults when resources inherit them"

The claim: when the Scaleway provider's default zone or region is outside
`params.region` but every resource sets its own allowed placement, the provider rule
still denies, so a plan that places nothing outside the region fails Layer 1.

That is intended, and the rule's comment now says so. `params.region` says where the
whole stack lives, and a provider default elsewhere is a placement waiting for its
first resource: the next one generated without its own `zone` lands in it. The repair
is one line in the provider block, and the denial text states only what is true, that
the provider defaults outside the region.

Tying the rule to the resources that inherit the default would also need each
resource type's schema. The plan does not say whether a resource has a `zone` at all:
provider 2.83.0 marks it unknown and 2.76.0 leaves it null, so the rule would depend
on a provider quirk to avoid denying a regional resource for a zone it does not have.
