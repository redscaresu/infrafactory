# Review — policies/aws/no_instance_profile.rego

Named for the story rather than numbered: pass numbers are a shared counter, and
parallel agents picking the next one collide.

## Why the configuration is walked at all

The story asked for a deny on `after_unknown.iam_instance_profile == true`. The captures
showed that rule cannot be written: on `aws_instance` the argument is Optional+Computed, so
the web-step-one plan, which sets no profile, plans `after_unknown.iam_instance_profile =
true` too (`internal/harness/testdata/instance-profile/aws/web_step_one.json`). An unknown
profile is therefore judged from the plan's `configuration`: the argument set at all is
denied.

## Declined: [P2] "Allow null instance-profile expressions"

The claim: `iam_instance_profile = null`, or a variable or conditional that evaluates to
null, attaches no profile, yet the configuration rule denies it. Suggested fix: ignore
known-null expressions, or deny from the configuration only where the planned value is
unknown.

The rule is that the argument is absent, and the deny text says so: remove the argument.
An argument that is null by design has no job in a stack whose instance runs with no
profile, so the cost of the false positive is one repair iteration with a precise fix.

Tying the configuration rule to the planned value means joining each configuration
resource to its `resource_changes` entries by module path and `count`/`for_each` index.
A join that misses drops the deny, so that design fails open on exactly the unknown
references this rule exists to catch. A false positive with a clear fix is the cheaper
error.
