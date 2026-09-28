---
kind: code
status: blocked
blocked_by: [policy-loader-skips-test-files, rego-test-harness]
epic: policy-correctness
depends_on: [policy-loader-skips-test-files, rego-test-harness]
touches: ["policies/aws/encryption.rego", "policies/aws/encryption_test.rego", "internal/harness/opa_m98_test.go"]
risk: high
---

# AWS encryption: an S3 SSE configuration satisfies only the bucket it names

The comment says 'referring to this bucket' (encryption.rego:19-22), but :35-40 and :44-49 accept any SSE config for every bucket. Bind it three ways: (a) configuration expressions.bucket.references naming aws_s3_bucket.<addr>, with the index stripped as in scaleway vpc_required.rego:97-98; (b) SSE values.bucket equal to the bucket's values.bucket; (c) the inline block. The after_unknown body goes, so opa_m98_test.go:61-66 swaps its required 'after_unknown' token for 'references': the M98 unknown-reference case is now read from configuration. Outcome at Layer 1, on every AWS plan: aws-s3 and aws-full-stack newly deny a bucket without its own SSE config. The aws_full_stack e2e stub binds by literal name (aws_full_stack_test.go:266-274) and still passes.

**Done when:**
- Rego test: buckets a and b with one SSE config whose bucket = aws_s3_bucket.a.id (unknown at plan) denies b (fails today)
- Rego test: an SSE config naming the bucket by literal name does not deny
- Rego test: aws_s3_bucket.x[0] with an SSE config referencing aws_s3_bucket.x does not deny
- opa_m98_test.go passes with the aws/encryption.rego tokens changed as scoped
- The harness mutation check kills all three deny bodies
