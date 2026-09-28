# Review — an AWS S3 SSE configuration satisfies only the bucket it names

Named for the story rather than numbered: pass numbers are a shared counter, and
parallel agents picking the next one collide.

Two findings declined; the accepted ones are in the PR body. Both declines rest on
what `tofu show -json` actually emits, checked against an OpenTofu 1.12.6 plan with
hashicorp/aws 5.100.0, and on `EvaluatePlanPoliciesWithParams` (`internal/harness/opa.go`)
handing that JSON to OPA unmodified.

## Declined: [P2] "Accept fully qualified bucket ID references" (pass 1)

The claim: for `bucket = aws_s3_bucket.assets.id` the configuration may record only
`aws_s3_bucket.assets.id`, not the bare `aws_s3_bucket.assets`, so the reference rule
would deny a correctly bound bucket.

The real plan records both, `["aws_s3_bucket.a.id", "aws_s3_bucket.a"]`, and for
`aws_s3_bucket.x[0].id` it records `["aws_s3_bucket.x[0].id", "aws_s3_bucket.x[0]",
"aws_s3_bucket.x"]`. The policy matches the bucket's own address, which is always
in that list.

## Declined: [P1]/[P2] "Exclude unknown bucket names from literal matching" (passes 2 and 3)

The claim: a known-after-apply bucket name and a reference-valued SSE `bucket` are
both `null` in `planned_values`, so `cfg.values.bucket == bucket.values.bucket`
matches any SSE config to any bucket with a generated name.

They are not `null`, they are absent: in the plan, `has("bucket")` is false for an
`aws_s3_bucket` using `bucket_prefix` and for an SSE config whose `bucket` is
`aws_s3_bucket.a.id`. The equality on two undefined values is undefined, so the rule
does not match. `test_indexed_sse_config_binds_only_its_instance` exercises exactly
this: its buckets have no known name, its SSE configs no known `bucket`, and it
denies `x[1]` and `y[0]`. A known `null` cannot arise either: the SSE `bucket` is
required, and an unset `aws_s3_bucket.bucket` is computed, so unknown.
