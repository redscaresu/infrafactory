# Review: a run-id tag key assembled from parts

`codex exec review --base main` on the AWS gate's provider leg, 2026-09-28. This file records the one
finding that was declined.

## [P2] Evaluate composed tag keys before admitting them (declined)

`awsRunIDTagProblem` looks for `infrafactory-run-id` in each name, identifier and literal string of
the stack, so a key built from constant parts, such as `"${"infrafactory"}-run-id"`, passes, and a
resource could carry a run id other than its own.

The finding is accurate, and the story asked for exactly this check: "refused, as refuseAwsRunIDTag
refuses it", which is mention-based on the generation path too. Closing it is not one more case.
After the template form comes `join("-", [...])`, `format()`, `replace()`, a local, a for
expression, `zipmap()`: the only complete answer is evaluating every tags expression with every
allowlisted function, locals and variable defaults, which is a second tofu.

It also buys no safety. The HLD (2026-09-27, § Containment) says tags report and never gate: the sweep
and reap enumerate the whole claimed scope by collection, whatever a resource's tags say, so a
mistagged resource is misreported, never left behind. The code comment on `awsRunIDTagProblem` now
states this limit, so the check is not read as a guarantee.
