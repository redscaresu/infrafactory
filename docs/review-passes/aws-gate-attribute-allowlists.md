# Review — the AWS gate's attribute allowlists, cost bounds and multiplicity bounds

Named for the story rather than numbered: pass numbers are a shared counter, and
parallel agents picking the next one collide.

One finding in one pass, declined.

## Declined: [P1] "Wire AWS attribute checks into preflight"

The claim: nothing in production calls `awsResourceProblems` or
`awsMultiplicityProblems`, and `layer3PreflightHCLForCloud` has no AWS branch, so the
allowlists are enforced only in tests.

That is this story's scope, by design. The AWS gate lands as four stories, and the
wiring is the fourth: `aws-gate-assembled` builds `validateAWSLayer3HCLShape` from
this leg, the provider leg and the user-data and AMI leg. It is blocked on all three.
Wiring this leg on its own would put a partial gate in front of a real account: no
provider, `user_data` or AMI checks.

Leaving it unwired fails closed. `layer3PreflightHCLForCloud` refuses every cloud but
Scaleway ("cloud aws has no Layer 3 HCL gate yet"), so no AWS stack reaches a real
apply until the assembled gate replaces that refusal. The finding itself says so: the
stack "continues to be refused as having no HCL gate".
